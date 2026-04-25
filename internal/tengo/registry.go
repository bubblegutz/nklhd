// Package tengo provides Tengo scripting support for nklhd route registration.
package tengo

import (
	"fmt"
	"os"
	"strings"

	"github.com/bubblegutz/nklhd/internal/exports/app"
	"github.com/bubblegutz/nklhd/internal/router"
	"github.com/bubblegutz/nklhd/internal/script"
	"github.com/d5/tengo/v2"
	"github.com/d5/tengo/v2/parser"
)

// globalIndices for the wrapper main function.
// These are placed at the end of the globals array so they don't
// conflict with the original script's globals (which handlers reference).
const (
	wrapperHandlerIdx = iota
	wrapperReqIdx
	wrapperResultIdx
	wrapperGlobalsExtra
)

// debugLogging controls whether debug messages are printed.
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

// RouteHandler implements router.ServeHandler by manually constructing
// bytecode that calls the handler function with the request argument.
// The wrapper stores handler+request at indices past the original globals
// so that handler bytecode (referencing globals from the compiled script)
// still resolves correctly.
type RouteHandler struct {
	Fn            *tengo.CompiledFunction
	origConstants []tengo.Object
	origGlobals   []tengo.Object
	mainFn        *tengo.CompiledFunction
	moduleObjects map[string]tengo.Object
	fileSet       *parser.SourceFileSet
}

// NewRouteHandler creates a new RouteHandler with a manually constructed
// main function that calls fn(request) and stores the result.
// origGlobals must contain the original script's global variables so that
// handler functions can resolve OpGetGlobal references.
// The wrapper stores handler, request, and result at indices past the
// original globals to avoid conflicting with the handler's global references.
func NewRouteHandler(fn *tengo.CompiledFunction, origConstants, origGlobals []tengo.Object, moduleObjects map[string]tengo.Object, fileSet *parser.SourceFileSet) *RouteHandler {
	baseIdx := len(origGlobals)

	// Manual wrapper instructions (no constant references):
	//   OpGetGlobal baseIdx+0 -- push handler
	//   OpGetGlobal baseIdx+1 -- push request
	//   OpCall 1 0            -- call handler(request)
	//   OpSetGlobal baseIdx+2 -- store result
	//   OpSuspend             -- clean exit
	insts := make([]byte, 0, 15)
	insts = append(insts, tengo.MakeInstruction(parser.OpGetGlobal, baseIdx+wrapperHandlerIdx)...)
	insts = append(insts, tengo.MakeInstruction(parser.OpGetGlobal, baseIdx+wrapperReqIdx)...)
	insts = append(insts, tengo.MakeInstruction(parser.OpCall, 1, 0)...)
	insts = append(insts, tengo.MakeInstruction(parser.OpSetGlobal, baseIdx+wrapperResultIdx)...)
	insts = append(insts, parser.OpSuspend)

	mainFn := &tengo.CompiledFunction{
		Instructions:  insts,
		NumLocals:     0,
		NumParameters: 0,
	}

	return &RouteHandler{
		Fn:            fn,
		origConstants: origConstants,
		origGlobals:   origGlobals,
		mainFn:        mainFn,
		moduleObjects: moduleObjects,
		fileSet:       fileSet,
	}
}

// Serve implements router.ServeHandler.
func (h *RouteHandler) Serve(req *router.Request) (*router.Response, error) {
	debugf("TengoRouteHandler.Serve: op=%s path=%s\n", req.Operation, req.Path)

	// Build a map representing the request
	reqMap := make(map[string]tengo.Object)
	for k, v := range req.Params {
		reqMap[k] = &tengo.String{Value: v}
	}
	reqMap["path"] = &tengo.String{Value: req.Path}
	reqMap["operation"] = &tengo.String{Value: req.Operation}
	if req.Data != nil {
		reqMap["data"] = &tengo.String{Value: string(req.Data)}
	}
	if req.Offset >= 0 {
		reqMap["offset"] = &tengo.Int{Value: req.Offset}
	}

	// Add modules directly to reqMap so handlers access them as req.*
	for k, v := range h.moduleObjects {
		reqMap[k] = v
	}

	// Create globals: copy original globals (preserving handler's global
	// references) and extend with handler, request, result slots.
	baseIdx := len(h.origGlobals)
	globals := make([]tengo.Object, baseIdx+wrapperGlobalsExtra)
	copy(globals, h.origGlobals)
	globals[baseIdx+wrapperHandlerIdx] = h.Fn
	globals[baseIdx+wrapperReqIdx] = &tengo.Map{Value: reqMap}

	// Create bytecode with the original constants so the handler function
	// can resolve its constant references correctly.
	bc := &tengo.Bytecode{
		MainFunction: h.mainFn,
		Constants:    h.origConstants,
		FileSet:      h.fileSet,
	}

	vm := tengo.NewVM(bc, globals, -1)
	if err := vm.Run(); err != nil {
		return nil, fmt.Errorf("tengo handler error: %v", err)
	}

	result := globals[baseIdx+wrapperResultIdx]
	if result == nil || result == tengo.UndefinedValue {
		return &router.Response{Data: ""}, nil
	}

	v := tengo.ToInterface(result)
	if v == nil {
		return &router.Response{Data: ""}, nil
	}

	switch r := v.(type) {
	case string:
		return &router.Response{Data: r}, nil
	case []byte:
		return &router.Response{Data: string(r)}, nil
	default:
		return &router.Response{Data: fmt.Sprint(r)}, nil
	}
}

// TengoApp exposes route registration functions to Tengo scripts.
type TengoApp struct {
	registry      *script.Registry
	constants     []tengo.Object
	origGlobals   []tengo.Object
	moduleObjects map[string]tengo.Object
	fileSet       *parser.SourceFileSet
}

// NewTengoApp creates a new TengoApp instance.
func NewTengoApp(registry *script.Registry, constants, origGlobals []tengo.Object, moduleObjects map[string]tengo.Object) *TengoApp {
	return &TengoApp{
		registry:      registry,
		constants:     constants,
		origGlobals:   origGlobals,
		moduleObjects: moduleObjects,
	}
}

// RegisterRead registers a handler for read operations (FUSE read).
func (app *TengoApp) RegisterRead(pathPattern string, fn *tengo.CompiledFunction) error {
	handler := NewRouteHandler(fn, app.constants, app.origGlobals, app.moduleObjects, app.fileSet)
	app.registry.Register("read", pathPattern, handler)
	return nil
}

// RegisterWrite registers a handler for write operations (FUSE write).
func (app *TengoApp) RegisterWrite(pathPattern string, fn *tengo.CompiledFunction) error {
	handler := NewRouteHandler(fn, app.constants, app.origGlobals, app.moduleObjects, app.fileSet)
	app.registry.Register("write", pathPattern, handler)
	return nil
}

// RegisterCreate registers a handler for create operations (FUSE create).
func (app *TengoApp) RegisterCreate(pathPattern string, fn *tengo.CompiledFunction) error {
	handler := NewRouteHandler(fn, app.constants, app.origGlobals, app.moduleObjects, app.fileSet)
	app.registry.Register("create", pathPattern, handler)
	return nil
}

// RegisterRemove registers a handler for remove operations (FUSE remove/unlink).
func (app *TengoApp) RegisterRemove(pathPattern string, fn *tengo.CompiledFunction) error {
	handler := NewRouteHandler(fn, app.constants, app.origGlobals, app.moduleObjects, app.fileSet)
	app.registry.Register("unlink", pathPattern, handler)
	return nil
}

// RegisterStat registers a handler for stat operations (FUSE getattr).
func (app *TengoApp) RegisterStat(pathPattern string, fn *tengo.CompiledFunction) error {
	handler := NewRouteHandler(fn, app.constants, app.origGlobals, app.moduleObjects, app.fileSet)
	app.registry.Register("getattr", pathPattern, handler)
	return nil
}

// RegisterWalk registers a handler for walk operations (custom traversal).
func (app *TengoApp) RegisterWalk(pathPattern string, fn *tengo.CompiledFunction) error {
	handler := NewRouteHandler(fn, app.constants, app.origGlobals, app.moduleObjects, app.fileSet)
	app.registry.Register("lookup", pathPattern, handler)
	return nil
}

// RegisterOpen registers a handler for open operations (FUSE open).
func (app *TengoApp) RegisterOpen(pathPattern string, fn *tengo.CompiledFunction) error {
	handler := NewRouteHandler(fn, app.constants, app.origGlobals, app.moduleObjects, app.fileSet)
	app.registry.Register("open", pathPattern, handler)
	return nil
}

// RegisterList registers a handler for directory listing operations (FUSE readdir).
func (app *TengoApp) RegisterList(pathPattern string, fn *tengo.CompiledFunction) error {
	handler := NewRouteHandler(fn, app.constants, app.origGlobals, app.moduleObjects, app.fileSet)
	app.registry.Register("readdir", pathPattern, handler)
	return nil
}

// Backend returns a language-agnostic backend interface for this TengoApp.
func (ta *TengoApp) Backend() app.Backend {
	return &tengoBackend{app: ta}
}

// tengoBackend implements app.Backend for Tengo.
type tengoBackend struct {
	app *TengoApp
}

func (b *tengoBackend) RegisterRead(pathPattern string, handler app.Handler) error {
	fn, ok := handler.(*tengo.CompiledFunction)
	if !ok {
		return fmt.Errorf("handler must be a *tengo.CompiledFunction")
	}
	return b.app.RegisterRead(pathPattern, fn)
}

func (b *tengoBackend) RegisterWrite(pathPattern string, handler app.Handler) error {
	fn, ok := handler.(*tengo.CompiledFunction)
	if !ok {
		return fmt.Errorf("handler must be a *tengo.CompiledFunction")
	}
	return b.app.RegisterWrite(pathPattern, fn)
}

func (b *tengoBackend) RegisterCreate(pathPattern string, handler app.Handler) error {
	fn, ok := handler.(*tengo.CompiledFunction)
	if !ok {
		return fmt.Errorf("handler must be a *tengo.CompiledFunction")
	}
	return b.app.RegisterCreate(pathPattern, fn)
}

func (b *tengoBackend) RegisterUnlink(pathPattern string, handler app.Handler) error {
	fn, ok := handler.(*tengo.CompiledFunction)
	if !ok {
		return fmt.Errorf("handler must be a *tengo.CompiledFunction")
	}
	return b.app.RegisterRemove(pathPattern, fn)
}

func (b *tengoBackend) RegisterGetAttr(pathPattern string, handler app.Handler) error {
	fn, ok := handler.(*tengo.CompiledFunction)
	if !ok {
		return fmt.Errorf("handler must be a *tengo.CompiledFunction")
	}
	return b.app.RegisterStat(pathPattern, fn)
}

func (b *tengoBackend) RegisterLookup(pathPattern string, handler app.Handler) error {
	fn, ok := handler.(*tengo.CompiledFunction)
	if !ok {
		return fmt.Errorf("handler must be a *tengo.CompiledFunction")
	}
	return b.app.RegisterWalk(pathPattern, fn)
}

func (b *tengoBackend) RegisterOpen(pathPattern string, handler app.Handler) error {
	fn, ok := handler.(*tengo.CompiledFunction)
	if !ok {
		return fmt.Errorf("handler must be a *tengo.CompiledFunction")
	}
	return b.app.RegisterOpen(pathPattern, fn)
}

func (b *tengoBackend) RegisterReadDir(pathPattern string, handler app.Handler) error {
	fn, ok := handler.(*tengo.CompiledFunction)
	if !ok {
		return fmt.Errorf("handler must be a *tengo.CompiledFunction")
	}
	return b.app.RegisterList(pathPattern, fn)
}

func (b *tengoBackend) Environment() map[string]string {
	env := make(map[string]string)
	for _, e := range os.Environ() {
		if idx := strings.Index(e, "="); idx != -1 {
			env[e[:idx]] = e[idx+1:]
		}
	}
	return env
}

func (b *tengoBackend) Log(msg string) {
	debugf("TENGO LOG: %s\n", msg)
}

func (b *tengoBackend) Debug(msg string) {
	if debugLogging {
		debugf("TENGO DEBUG: %s\n", msg)
	}
}
