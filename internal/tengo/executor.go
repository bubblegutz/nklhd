package tengo

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"github.com/bubblegutz/nklhd/internal/router"
	"github.com/bubblegutz/nklhd/internal/script"
	"github.com/bubblegutz/nklhd/internal/tengo/modules/fs"
	"github.com/bubblegutz/nklhd/internal/tengo/modules/http"
	"github.com/bubblegutz/nklhd/internal/tengo/modules/jq"
	"github.com/bubblegutz/nklhd/internal/tengo/modules/oauth"
	"github.com/bubblegutz/nklhd/internal/tengo/modules/yaml"
	"github.com/d5/tengo/v2"
	"github.com/d5/tengo/v2/parser"
	"github.com/d5/tengo/v2/stdlib"
)

// Executor manages Tengo script execution and route registration.
type Executor struct {
	registry *script.Registry
}

// NewExecutor creates a new Executor with empty route registry.
func NewExecutor() *Executor {
	return &Executor{
		registry: script.NewRegistry(),
	}
}

// Close cleans up resources held by the executor.
func (e *Executor) Close() {}

// moduleNames is the list of Tengo standard library modules pre-injected as globals.
// moduleNames is the list of Tengo standard library modules pre-injected as globals.
var moduleNames = []string{"json", "os", "times", "text", "fmt", "math", "rand", "enum", "hex", "base64"}

// customModuleNames is the list of custom nklhd modules.
var customModuleNames = []string{"yaml", "http", "oauth", "fs", "jq"}

// customModules returns a map of custom module name to module map.
func customModules() map[string]map[string]tengo.Object {
	return map[string]map[string]tengo.Object{
		"yaml":  yaml.Module,
		"http":  http.Module,
		"oauth": oauth.Module,
		"fs":    fs.Module,
		"jq":    jq.Module,
	}
}

// getModuleMap returns a module map with all standard library and custom modules.
func getModuleMap() *tengo.ModuleMap {
	moduleMap := stdlib.GetModuleMap(moduleNames...)
	for name, mod := range customModules() {
		moduleMap.AddBuiltinModule(name, mod)
	}
	return moduleMap
}

// injectModules registers modules for top-level import() support and enables
// file imports for loading local .tengo files.
func (e *Executor) injectModules(s *tengo.Script, scriptFile string) {
	moduleMap := getModuleMap()
	s.SetImports(moduleMap)

	// Enable file imports so import("./local") loads local .tengo files.
	s.EnableFileImport(true)
	if dir := filepath.Dir(scriptFile); dir != "" {
		s.SetImportDir(dir) //nolint:errcheck
	}
}

// createModuleObjects instantiates module objects for injection
// into handler request maps (req). This is the only way handlers
// can access module features, since handlers run in separate VMs without
// access to the script's global scope.
func createModuleObjects(moduleMap *tengo.ModuleMap) map[string]tengo.Object {
	allNames := append(append([]string{}, moduleNames...), customModuleNames...)
	objects := make(map[string]tengo.Object, len(allNames))
	for _, name := range allNames {
		if mod := moduleMap.Get(name); mod != nil {
			v, err := mod.Import(name)
			if err == nil {
				if obj, ok := v.(tengo.Object); ok {
					objects[name] = obj
				}
			}
		}
	}
	return objects
}

// LoadScript loads a Tengo script file, executes it, and registers any routes
// defined via the global 'app' object.
func (e *Executor) LoadScript(scriptFile string) error {
	src, err := os.ReadFile(scriptFile)
	if err != nil {
		return fmt.Errorf("failed to read script %s: %v", scriptFile, err)
	}

	s := tengo.NewScript(src)
	e.injectModules(s, scriptFile)
	moduleMap := getModuleMap()
	moduleObjects := createModuleObjects(moduleMap)

	// Create TengoApp with nil constants/globals — they will be populated
	// after compilation, before Run() triggers handler registration.
	tengoApp := NewTengoApp(e.registry, nil, nil, moduleObjects)
	e.injectApp(s, tengoApp)

	debugf("LoadScript: compiling %s\n", scriptFile)
	compiled, err := s.Compile()
	if err != nil {
		return fmt.Errorf("failed to compile script %s: %v", scriptFile, err)
	}

	// Extract constants from the compiled bytecode (includes constants from
	// imported modules, since Tengo's Compiler.addConstant delegates to the
	// parent compiler for forked module compilers).
	bc := extractCompiledBytecode(compiled)
	tengoApp.constants = bc.Constants
	if bc.FileSet != nil {
		tengoApp.fileSet = bc.FileSet
	} else {
		// Create a minimal FileSet so VM.Run() doesn't panic on error
		tengoApp.fileSet = parser.NewFileSet()
		if dir := filepath.Dir(scriptFile); dir != "" {
			tengoApp.fileSet.AddFile(scriptFile, -1, len(src))
		} else {
			tengoApp.fileSet.AddFile("(main)", -1, len(src))
		}
	}

	debugf("LoadScript: extracted %d constants from %s\n", len(bc.Constants), scriptFile)

	// Extract globals from the compiled script BEFORE Run().
	// The globals slice is pre-allocated by the compiler with nil entries
	// for user-defined variables. After Run() the slice is populated.
	// We capture the reference now so RouteHandler's copy at Serve() time
	// will see the populated values (the backing array is modified in-place).
	globals := extractCompiledGlobals(compiled)
	tengoApp.origGlobals = globals

	debugf("LoadScript: executing %s\n", scriptFile)
	if err := compiled.Run(); err != nil {
		return fmt.Errorf("failed to load script %s: %v", scriptFile, err)
	}
	return nil
}

// extractCompiledGlobals accesses the unexported globals field from
// a tengo.Compiled instance using unsafe pointer arithmetic.
func extractCompiledGlobals(compiled *tengo.Compiled) []tengo.Object {
	// The Compiled struct layout (from tengo v2.17.0):
	//   globalIndexes map[string]int
	//   bytecode      *Bytecode
	//   globals       []Object
	// We access the globals field (3rd field) by offset.
	// Using reflect-based unsafe access since the field is unexported.
	v := *(*[]tengo.Object)(unsafe.Pointer(
		uintptr(unsafe.Pointer(compiled)) +
			// Skip globalIndexes (map = 8 bytes) + bytecode (*Bytecode = 8 bytes)
			// On 64-bit: 16 bytes for first two fields
			unsafe.Offsetof(struct {
				a map[string]int
				b *byte
				c []tengo.Object
			}{}.c),
	))
	return v
}

// extractCompiledBytecode accesses the unexported bytecode field from
// a tengo.Compiled instance using unsafe pointer arithmetic.
func extractCompiledBytecode(compiled *tengo.Compiled) *tengo.Bytecode {
	// The Compiled struct layout (from tengo v2.17.0):
	//   globalIndexes map[string]int
	//   bytecode      *Bytecode
	//   globals       []Object
	// The bytecode field is the 2nd field, offset after globalIndexes.
	return *(**tengo.Bytecode)(unsafe.Pointer(
		uintptr(unsafe.Pointer(compiled)) +
			unsafe.Offsetof(struct {
				a map[string]int
				b *tengo.Bytecode
			}{}.b),
	))
}

// extractConstants compiles the script source to extract the constants array
// without executing any user functions. The constants are needed by RouteHandler
// when creating VMs for handler execution.
func extractConstants(src []byte, modules *tengo.ModuleMap) ([]tengo.Object, error) {
	fileSet := parser.NewFileSet()
	srcFile := fileSet.AddFile("(main)", -1, len(src))
	p := parser.NewParser(srcFile, src, nil)
	file, err := p.ParseFile()
	if err != nil {
		return nil, fmt.Errorf("parse error: %v", err)
	}

	symbolTable := tengo.NewSymbolTable()
	symbolTable.Define("app")

	c := tengo.NewCompiler(srcFile, symbolTable, nil, modules, nil)
	if err := c.Compile(file); err != nil {
		return nil, fmt.Errorf("compile error: %v", err)
	}

	bc := c.Bytecode()
	bc.RemoveDuplicates()

	return bc.Constants, nil
}

// injectApp injects the global 'app' object into the Tengo script.
func (e *Executor) injectApp(s *tengo.Script, tengoApp *TengoApp) {
	appMap := make(map[string]tengo.Object)

	// read
	appMap["read"] = &tengo.UserFunction{
		Name: "read",
		Value: func(args ...tengo.Object) (tengo.Object, error) {
			path := args[0].(*tengo.String).Value
			fn := args[1].(*tengo.CompiledFunction)
			debugf("app.read: registering %s\n", path)
			if err := tengoApp.RegisterRead(path, fn); err != nil {
				return nil, err
			}
			return tengo.UndefinedValue, nil
		},
	}

	// write
	appMap["write"] = &tengo.UserFunction{
		Name: "write",
		Value: func(args ...tengo.Object) (tengo.Object, error) {
			path := args[0].(*tengo.String).Value
			fn := args[1].(*tengo.CompiledFunction)
			debugf("app.write: registering %s\n", path)
			if err := tengoApp.RegisterWrite(path, fn); err != nil {
				return nil, err
			}
			return tengo.UndefinedValue, nil
		},
	}

	// create
	appMap["create"] = &tengo.UserFunction{
		Name: "create",
		Value: func(args ...tengo.Object) (tengo.Object, error) {
			path := args[0].(*tengo.String).Value
			fn := args[1].(*tengo.CompiledFunction)
			debugf("app.create: registering %s\n", path)
			if err := tengoApp.RegisterCreate(path, fn); err != nil {
				return nil, err
			}
			return tengo.UndefinedValue, nil
		},
	}

	// unlink
	appMap["unlink"] = &tengo.UserFunction{
		Name: "unlink",
		Value: func(args ...tengo.Object) (tengo.Object, error) {
			path := args[0].(*tengo.String).Value
			fn := args[1].(*tengo.CompiledFunction)
			debugf("app.unlink: registering %s\n", path)
			if err := tengoApp.RegisterRemove(path, fn); err != nil {
				return nil, err
			}
			return tengo.UndefinedValue, nil
		},
	}

	// getattr
	appMap["getattr"] = &tengo.UserFunction{
		Name: "getattr",
		Value: func(args ...tengo.Object) (tengo.Object, error) {
			path := args[0].(*tengo.String).Value
			fn := args[1].(*tengo.CompiledFunction)
			debugf("app.getattr: registering %s\n", path)
			if err := tengoApp.RegisterStat(path, fn); err != nil {
				return nil, err
			}
			return tengo.UndefinedValue, nil
		},
	}

	// lookup
	appMap["lookup"] = &tengo.UserFunction{
		Name: "lookup",
		Value: func(args ...tengo.Object) (tengo.Object, error) {
			path := args[0].(*tengo.String).Value
			fn := args[1].(*tengo.CompiledFunction)
			debugf("app.lookup: registering %s\n", path)
			if err := tengoApp.RegisterWalk(path, fn); err != nil {
				return nil, err
			}
			return tengo.UndefinedValue, nil
		},
	}

	// open
	appMap["open"] = &tengo.UserFunction{
		Name: "open",
		Value: func(args ...tengo.Object) (tengo.Object, error) {
			path := args[0].(*tengo.String).Value
			fn := args[1].(*tengo.CompiledFunction)
			debugf("app.open: registering %s\n", path)
			if err := tengoApp.RegisterOpen(path, fn); err != nil {
				return nil, err
			}
			return tengo.UndefinedValue, nil
		},
	}

	// readdir
	appMap["readdir"] = &tengo.UserFunction{
		Name: "readdir",
		Value: func(args ...tengo.Object) (tengo.Object, error) {
			path := args[0].(*tengo.String).Value
			fn := args[1].(*tengo.CompiledFunction)
			debugf("app.readdir: registering %s\n", path)
			if err := tengoApp.RegisterList(path, fn); err != nil {
				return nil, err
			}
			return tengo.UndefinedValue, nil
		},
	}

	// Environment variables
	envMap := make(map[string]tengo.Object)
	for _, e := range os.Environ() {
		if key, val, ok := strings.Cut(e, "="); ok {
			envMap[key] = &tengo.String{Value: val}
		}
	}
	appMap["env"] = &tengo.ImmutableMap{Value: envMap}

	// Logging functions
	appMap["log"] = &tengo.UserFunction{
		Name: "log",
		Value: func(args ...tengo.Object) (tengo.Object, error) {
			msg := ""
			if len(args) > 0 {
				if s, ok := args[0].(*tengo.String); ok {
					msg = s.Value
				} else {
					msg = args[0].String()
				}
			}
			debugf("TENGO LOG: %s\n", msg)
			return tengo.UndefinedValue, nil
		},
	}

	appMap["debug"] = &tengo.UserFunction{
		Name: "debug",
		Value: func(args ...tengo.Object) (tengo.Object, error) {
			if debugLogging && len(args) > 0 {
				msg := ""
				if s, ok := args[0].(*tengo.String); ok {
					msg = s.Value
				} else {
					msg = args[0].String()
				}
				debugf("TENGO DEBUG: %s\n", msg)
			}
			return tengo.UndefinedValue, nil
		},
	}

	s.Add("app", &tengo.ImmutableMap{Value: appMap})
}

// ExecuteHandler executes the appropriate handler for the given operation and path.
func (e *Executor) ExecuteHandler(operation, requestPath string, params map[string]string, data []byte, offset int64) (string, error) {
	handler, pathParams := e.registry.FindHandler(operation, requestPath)
	if handler == nil {
		debugf("ExecuteHandler: no handler registered for %s %s\n", operation, requestPath)
		return "", fmt.Errorf("no handler registered for %s %s", operation, requestPath)
	}
	debugf("ExecuteHandler: found handler for %s %s, pathParams %v, params %v\n", operation, requestPath, pathParams, params)

	// Merge path params with additional params
	allParams := make(map[string]string)
	for k, v := range pathParams {
		allParams[k] = v
	}
	for k, v := range params {
		allParams[k] = v
	}

	resp, err := handler.Serve(&router.Request{
		Operation: operation,
		Path:      requestPath,
		Params:    allParams,
		Data:      data,
		Offset:    offset,
	})
	if err != nil {
		return "", err
	}
	return resp.Data, nil
}

// ExecuteListHandler executes a list handler for the given request path.
func (e *Executor) ExecuteListHandler(requestPath string, params map[string]string) ([]string, error) {
	debugf("ExecuteListHandler looking for list handler for path %s, params %v\n", requestPath, params)
	handler, pathParams := e.registry.FindHandler("readdir", requestPath)
	if handler == nil {
		debugf("  no list handler found\n")
		return nil, fmt.Errorf("no list handler registered for %s", requestPath)
	}
	debugf("  found handler, pathParams %v\n", pathParams)

	// Merge path params with additional params
	allParams := make(map[string]string)
	for k, v := range pathParams {
		allParams[k] = v
	}
	for k, v := range params {
		allParams[k] = v
	}

	resp, err := handler.Serve(&router.Request{
		Operation: "readdir",
		Path:      requestPath,
		Params:    allParams,
	})
	if err != nil {
		return nil, err
	}

	// Parse the response (newline-separated entries)
	data := strings.TrimSpace(resp.Data)
	if data == "" {
		return []string{}, nil
	}
	return strings.Split(data, "\n"), nil
}

// GetRoutePaths returns all unique route path patterns registered across all operations.
func (e *Executor) GetRoutePaths() []string {
	return e.registry.GetAllRoutePaths()
}

// GetRouter returns the underlying router instance.
func (e *Executor) GetRouter() *router.Router {
	return e.registry.GetRouter()
}
