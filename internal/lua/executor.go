package lua

import (
	"fmt"
	"os"
	"strings"

	"github.com/bubblegutz/nklhd/internal/lua/modules/fs"
	"github.com/bubblegutz/nklhd/internal/lua/modules/http"
	"github.com/bubblegutz/nklhd/internal/lua/modules/jq"
	"github.com/bubblegutz/nklhd/internal/lua/modules/json"
	"github.com/bubblegutz/nklhd/internal/lua/modules/oauth"
	"github.com/bubblegutz/nklhd/internal/lua/modules/yaml"
	"github.com/bubblegutz/nklhd/internal/router"
	"github.com/bubblegutz/nklhd/internal/script"
	lua "github.com/yuin/gopher-lua"
)

var debugLogging bool = true

// SetDebug enables or disables debug logging.
func SetDebug(debug bool) {
	debugLogging = debug
}

func debugf(format string, args ...any) {
	if debugLogging {
		fmt.Fprintf(os.Stderr, format, args...)
	}
}

// Executor manages Lua script execution and route registration.
type Executor struct {
	registry *script.Registry
	// scripts maps script file path to its Lua state (for caching)
	scripts map[string]*lua.LState
}

// NewExecutor creates a new Executor with empty route registry.
func NewExecutor() *Executor {
	return &Executor{
		registry: script.NewRegistry(),
		scripts:  make(map[string]*lua.LState),
	}
}

// Close cleans up all Lua states held by the executor.
func (e *Executor) Close() {
	for _, L := range e.scripts {
		L.Close()
	}
	e.scripts = make(map[string]*lua.LState)
}

// LoadScript loads a Lua script file, executes it, and registers any routes
// defined via the global 'app' object.
func (e *Executor) LoadScript(scriptFile string) error {
	// Check if already loaded
	if _, exists := e.scripts[scriptFile]; exists {
		return nil
	}

	L := lua.NewState()

	// Inject global 'app' object with route registration methods
	e.injectApp(L)

	// Load and execute the script
	debugf("LoadScript: executing %s\n", scriptFile)
	if err := L.DoFile(scriptFile); err != nil {
		L.Close()
		return fmt.Errorf("failed to load script %s: %v", scriptFile, err)
	}

	// Store the Lua state for later use
	e.scripts[scriptFile] = L
	return nil
}

// injectApp injects the global 'app' object into the Lua state.
func (e *Executor) injectApp(L *lua.LState) {
	app := NewLuaApp(e.registry, L)

	// Create app table
	appTable := L.NewTable()

	// Set methods
	L.SetField(appTable, "read", L.NewFunction(func(L *lua.LState) int {
		path := L.CheckString(1)
		fn := L.CheckFunction(2)
		debugf("app.read: registering %s\n", path)
		app.RegisterRead(path, fn)
		return 0
	}))

	L.SetField(appTable, "write", L.NewFunction(func(L *lua.LState) int {
		path := L.CheckString(1)
		fn := L.CheckFunction(2)
		debugf("app.write: registering %s\n", path)
		app.RegisterWrite(path, fn)
		return 0
	}))

	L.SetField(appTable, "create", L.NewFunction(func(L *lua.LState) int {
		path := L.CheckString(1)
		fn := L.CheckFunction(2)
		debugf("app.create: registering %s\n", path)
		app.RegisterCreate(path, fn)
		return 0
	}))

	L.SetField(appTable, "unlink", L.NewFunction(func(L *lua.LState) int {
		path := L.CheckString(1)
		fn := L.CheckFunction(2)
		debugf("app.unlink: registering %s\n", path)
		app.RegisterRemove(path, fn)
		return 0
	}))

	L.SetField(appTable, "getattr", L.NewFunction(func(L *lua.LState) int {
		path := L.CheckString(1)
		fn := L.CheckFunction(2)
		app.RegisterStat(path, fn)
		return 0
	}))

	L.SetField(appTable, "lookup", L.NewFunction(func(L *lua.LState) int {
		path := L.CheckString(1)
		fn := L.CheckFunction(2)
		app.RegisterWalk(path, fn)
		return 0
	}))

	L.SetField(appTable, "open", L.NewFunction(func(L *lua.LState) int {
		path := L.CheckString(1)
		fn := L.CheckFunction(2)
		app.RegisterOpen(path, fn)
		return 0
	}))

	L.SetField(appTable, "readdir", L.NewFunction(func(L *lua.LState) int {
		path := L.CheckString(1)
		fn := L.CheckFunction(2)
		debugf("app.readdir: registering %s\n", path)
		app.RegisterList(path, fn)
		return 0
	}))

	// Environment variables
	envTable := L.NewTable()
	for _, env := range os.Environ() {
		if key, val, ok := strings.Cut(env, "="); ok {
			L.SetField(envTable, key, lua.LString(val))
		}
	}
	L.SetField(appTable, "env", envTable)

	// Logging functions
	L.SetField(appTable, "log", L.NewFunction(func(L *lua.LState) int {
		msg := L.CheckString(1)
		debugf("LUA LOG: %s\n", msg)
		return 0
	}))

	L.SetField(appTable, "debug", L.NewFunction(func(L *lua.LState) int {
		if debugLogging {
			msg := L.CheckString(1)
			debugf("LUA DEBUG: %s\n", msg)
		}
		return 0
	}))

	// Set global 'app'
	L.SetGlobal("app", appTable)

	// Register utility modules
	http.Register(L)
	json.Register(L)
	oauth.Register(L)
	yaml.Register(L)
	jq.Register(L)
	fs.Register(L)
}

// ExecuteHandler executes the appropriate handler for the given operation and path.
// It passes request parameters to the Lua handler and returns the result.
func (e *Executor) ExecuteHandler(operation, requestPath string, params map[string]string, data []byte, offset int64) (string, error) {
	handler, pathParams := e.registry.FindHandler(operation, requestPath)
	if handler == nil {
		debugf("ExecuteHandler: no handler registered for %s %s\n", operation, requestPath)
		return "", fmt.Errorf("no handler registered for %s %s", operation, requestPath)
	}
	debugf("ExecuteHandler: found handler for %s %s, pathParams %v, params %v\n", operation, requestPath, pathParams, params)

	// Merge params (path parameters) with additional params
	allParams := make(map[string]string)
	for k, v := range pathParams {
		allParams[k] = v
	}
	for k, v := range params {
		allParams[k] = v
	}

	// Cast to Lua RouteHandler
	lh, ok := handler.(*RouteHandler)
	if !ok {
		return "", fmt.Errorf("handler is not a Lua RouteHandler")
	}

	// Prepare arguments for the Lua handler
	L := lh.L

	// Build a Lua table representing the request
	reqTable := L.NewTable()
	for k, v := range allParams {
		L.SetField(reqTable, k, lua.LString(v))
	}
	L.SetField(reqTable, "path", lua.LString(requestPath))
	L.SetField(reqTable, "operation", lua.LString(operation))
	if data != nil {
		L.SetField(reqTable, "data", lua.LString(string(data)))
	}
	if offset >= 0 {
		L.SetField(reqTable, "offset", lua.LNumber(offset))
	}

	// Call the handler with the request table as single argument
	debugf("ExecuteHandler: calling Lua handler for %s %s\n", operation, requestPath)
	if err := L.CallByParam(lua.P{
		Fn:      lh.Func,
		NRet:    1,
		Protect: true,
	}, reqTable); err != nil {
		debugf("ExecuteHandler: Lua handler error: %v\n", err)
		return "", fmt.Errorf("lua handler error: %v", err)
	}

	ret := L.Get(-1)
	L.Pop(1)

	debugf("ExecuteHandler: Lua returned type %s\n", ret.Type())
	if ret.Type() == lua.LTString {
		str := ret.String()
		debugf("ExecuteHandler: returning string length %d\n", len(str))
		return str, nil
	}
	// Allow nil return (no error, no data)
	if ret.Type() == lua.LTNil {
		debugf("ExecuteHandler: returning empty (nil)\n")
		return "", nil
	}
	debugf("ExecuteHandler: invalid return type %s\n", ret.Type())
	return "", fmt.Errorf("handler must return a string or nil, got %s", ret.Type())
}

// ExecuteListHandler executes a list handler for the given request path.
// It returns a slice of entry names (strings) that should appear in the directory.
// Each entry can be a file or directory; directories should have a trailing slash.
func (e *Executor) ExecuteListHandler(requestPath string, params map[string]string) ([]string, error) {
	debugf("ExecuteListHandler looking for list handler for path %s, params %v\n", requestPath, params)
	handler, pathParams := e.registry.FindHandler("readdir", requestPath)
	if handler == nil {
		debugf("  no list handler found\n")
		return nil, fmt.Errorf("no list handler registered for %s", requestPath)
	}
	debugf("  found handler, pathParams %v\n", pathParams)

	// Merge params (path parameters) with additional params
	allParams := make(map[string]string)
	for k, v := range pathParams {
		allParams[k] = v
	}
	for k, v := range params {
		allParams[k] = v
	}

	// Cast to Lua RouteHandler
	lh, ok := handler.(*RouteHandler)
	if !ok {
		return nil, fmt.Errorf("handler is not a Lua RouteHandler")
	}

	// Prepare arguments for the Lua handler
	L := lh.L

	// Build a Lua table representing the request
	reqTable := L.NewTable()
	for k, v := range allParams {
		L.SetField(reqTable, k, lua.LString(v))
	}
	L.SetField(reqTable, "path", lua.LString(requestPath))
	L.SetField(reqTable, "operation", lua.LString("readdir"))

	// Call the handler with the request table as single argument
	if err := L.CallByParam(lua.P{
		Fn:      lh.Func,
		NRet:    1,
		Protect: true,
	}, reqTable); err != nil {
		return nil, fmt.Errorf("lua handler error: %v", err)
	}

	ret := L.Get(-1)
	L.Pop(1)

	// Parse the return value
	switch ret.Type() {
	case lua.LTNil:
		return []string{}, nil
	case lua.LTString:
		// Split by newline, filter empty lines
		str := ret.String()
		entries := strings.FieldsFunc(str, func(r rune) bool { return r == '\n' })
		return entries, nil
	case lua.LTTable:
		tbl := ret.(*lua.LTable)
		var entries []string
		tbl.ForEach(func(key lua.LValue, value lua.LValue) {
			if value.Type() == lua.LTString {
				entries = append(entries, value.String())
			}
		})
		return entries, nil
	default:
		return nil, fmt.Errorf("list handler must return a string, table, or nil, got %s", ret.Type())
	}
}

// GetRoutePaths returns all unique route path patterns registered across all operations.
func (e *Executor) GetRoutePaths() []string {
	return e.registry.GetAllRoutePaths()
}

// GetRouteTree returns a hierarchical tree representation of all registered routes.
func (e *Executor) GetRouteTree() router.TreeNode {
	return e.registry.BuildTree()
}

// GetRouter returns the underlying router instance.
func (e *Executor) GetRouter() *router.Router {
	return e.registry.GetRouter()
}

// ExecuteScript is the legacy API for executing a specific function from a script.
// It maintains backward compatibility with the old config-based routing.
func (e *Executor) ExecuteScript(scriptFile, functionName string, params map[string]string, data []byte) (string, error) {
	// Load the script if not already loaded
	if err := e.LoadScript(scriptFile); err != nil {
		return "", err
	}

	L := e.scripts[scriptFile]
	if L == nil {
		return "", fmt.Errorf("script %s not loaded", scriptFile)
	}

	// Set globals for backward compatibility
	for k, v := range params {
		L.SetGlobal(k, lua.LString(v))
	}
	L.SetGlobal("requestData", lua.LString(string(data)))

	// Call the function
	if err := L.CallByParam(lua.P{
		Fn:      L.GetGlobal(functionName),
		NRet:    1,
		Protect: true,
	}); err != nil {
		return "", fmt.Errorf("lua function error: %v", err)
	}

	ret := L.Get(-1)
	L.Pop(1)

	if ret.Type() == lua.LTString {
		return ret.String(), nil
	}
	return "", fmt.Errorf("script did not return a string")
}
