// Package lua provides route registration and matching using a robust radix tree router.
package lua

import (
	"fmt"
	"os"
	"strings"

	"github.com/bubblegutz/nklhd/internal/exports/app"
	"github.com/bubblegutz/nklhd/internal/router"
	"github.com/bubblegutz/nklhd/internal/script"
	lua "github.com/yuin/gopher-lua"
)

// RouteHandler represents a registered Lua handler for a specific operation.
type RouteHandler struct {
	// L is the Lua state where the handler function is defined.
	L *lua.LState
	// Func is the Lua function to call.
	Func lua.LValue
}

// Serve implements router.ServeHandler.
func (h *RouteHandler) Serve(req *router.Request) (*router.Response, error) {
	debugf("RouteHandler.Serve: op=%s path=%s\n", req.Operation, req.Path)
	// Build a Lua table representing the request
	reqTable := h.L.NewTable()
	for k, v := range req.Params {
		h.L.SetField(reqTable, k, lua.LString(v))
	}
	h.L.SetField(reqTable, "path", lua.LString(req.Path))
	h.L.SetField(reqTable, "operation", lua.LString(req.Operation))
	if req.Data != nil {
		h.L.SetField(reqTable, "data", lua.LString(string(req.Data)))
	}
	if req.Offset >= 0 {
		h.L.SetField(reqTable, "offset", lua.LNumber(req.Offset))
	}

	// Call the Lua function with the request table
	if err := h.L.CallByParam(lua.P{
		Fn:      h.Func,
		NRet:    1,
		Protect: true,
	}, reqTable); err != nil {
		return nil, fmt.Errorf("lua handler error: %v", err)
	}

	ret := h.L.Get(-1)
	h.L.Pop(1)
	debugf("RouteHandler.Serve: ret type=%v value=%v\n", ret.Type(), ret)

	if ret.Type() == lua.LTString {
		return &router.Response{Data: ret.String()}, nil
	}
	if ret.Type() == lua.LTNil {
		return &router.Response{Data: ""}, nil
	}
	if ret.Type() == lua.LTTable && (req.Operation == "readdir" || req.Operation == "getattr") {
		tbl := ret.(*lua.LTable)
		if req.Operation == "readdir" {
			var entries []string
			tbl.ForEach(func(key lua.LValue, value lua.LValue) {
				if value.Type() == lua.LTString {
					entries = append(entries, value.String())
				}
			})
			return &router.Response{Data: strings.Join(entries, "\n")}, nil
		} else { // stat
			var lines []string
			tbl.ForEach(func(key lua.LValue, value lua.LValue) {
				if key.Type() == lua.LTString {
					lines = append(lines, key.String()+"="+value.String())
				}
			})
			return &router.Response{Data: strings.Join(lines, "\n")}, nil
		}
	}
	return nil, fmt.Errorf("handler must return a string or nil, got %s", ret.Type())
}

// LuaApp exposes route registration functions to Lua scripts.
type LuaApp struct {
	registry *script.Registry
	L        *lua.LState
}

// NewLuaApp creates a new LuaApp instance for a Lua state.
func NewLuaApp(registry *script.Registry, L *lua.LState) *LuaApp {
	return &LuaApp{registry: registry, L: L}
}

// RegisterRead registers a handler for read operations (FUSE read).
func (app *LuaApp) RegisterRead(pathPattern string, fn lua.LValue) {
	app.registry.Register("read", pathPattern, &RouteHandler{L: app.L, Func: fn})
}

// RegisterWrite registers a handler for write operations (FUSE write).
func (app *LuaApp) RegisterWrite(pathPattern string, fn lua.LValue) {
	app.registry.Register("write", pathPattern, &RouteHandler{L: app.L, Func: fn})
}

// RegisterCreate registers a handler for create operations (FUSE create).
func (app *LuaApp) RegisterCreate(pathPattern string, fn lua.LValue) {
	app.registry.Register("create", pathPattern, &RouteHandler{L: app.L, Func: fn})
}

// RegisterRemove registers a handler for remove operations (FUSE remove/unlink).
func (app *LuaApp) RegisterRemove(pathPattern string, fn lua.LValue) {
	app.registry.Register("unlink", pathPattern, &RouteHandler{L: app.L, Func: fn})
}

// RegisterStat registers a handler for stat operations (FUSE getattr).
func (app *LuaApp) RegisterStat(pathPattern string, fn lua.LValue) {
	app.registry.Register("getattr", pathPattern, &RouteHandler{L: app.L, Func: fn})
}

// RegisterWalk registers a handler for walk operations (custom traversal).
func (app *LuaApp) RegisterWalk(pathPattern string, fn lua.LValue) {
	app.registry.Register("lookup", pathPattern, &RouteHandler{L: app.L, Func: fn})
}

// RegisterOpen registers a handler for open operations (FUSE open).
func (app *LuaApp) RegisterOpen(pathPattern string, fn lua.LValue) {
	app.registry.Register("open", pathPattern, &RouteHandler{L: app.L, Func: fn})
}

// RegisterList registers a handler for directory listing operations (FUSE readdir).
func (app *LuaApp) RegisterList(pathPattern string, fn lua.LValue) {
	app.registry.Register("readdir", pathPattern, &RouteHandler{L: app.L, Func: fn})
}

// Backend returns a language-agnostic backend interface for this LuaApp.
func (la *LuaApp) Backend() app.Backend {
	return &luaBackend{app: la}
}

// luaBackend implements app.Backend for Lua.
type luaBackend struct {
	app *LuaApp
}

func (b *luaBackend) RegisterRead(pathPattern string, handler app.Handler) error {
	fn, ok := handler.(lua.LValue)
	if !ok {
		return fmt.Errorf("handler must be a Lua function")
	}
	b.app.RegisterRead(pathPattern, fn)
	return nil
}

func (b *luaBackend) RegisterWrite(pathPattern string, handler app.Handler) error {
	fn, ok := handler.(lua.LValue)
	if !ok {
		return fmt.Errorf("handler must be a Lua function")
	}
	b.app.RegisterWrite(pathPattern, fn)
	return nil
}

func (b *luaBackend) RegisterCreate(pathPattern string, handler app.Handler) error {
	fn, ok := handler.(lua.LValue)
	if !ok {
		return fmt.Errorf("handler must be a Lua function")
	}
	b.app.RegisterCreate(pathPattern, fn)
	return nil
}

func (b *luaBackend) RegisterUnlink(pathPattern string, handler app.Handler) error {
	fn, ok := handler.(lua.LValue)
	if !ok {
		return fmt.Errorf("handler must be a Lua function")
	}
	b.app.RegisterRemove(pathPattern, fn)
	return nil
}

func (b *luaBackend) RegisterGetAttr(pathPattern string, handler app.Handler) error {
	fn, ok := handler.(lua.LValue)
	if !ok {
		return fmt.Errorf("handler must be a Lua function")
	}
	b.app.RegisterStat(pathPattern, fn)
	return nil
}

func (b *luaBackend) RegisterLookup(pathPattern string, handler app.Handler) error {
	fn, ok := handler.(lua.LValue)
	if !ok {
		return fmt.Errorf("handler must be a Lua function")
	}
	b.app.RegisterWalk(pathPattern, fn)
	return nil
}

func (b *luaBackend) RegisterOpen(pathPattern string, handler app.Handler) error {
	fn, ok := handler.(lua.LValue)
	if !ok {
		return fmt.Errorf("handler must be a Lua function")
	}
	b.app.RegisterOpen(pathPattern, fn)
	return nil
}

func (b *luaBackend) RegisterReadDir(pathPattern string, handler app.Handler) error {
	fn, ok := handler.(lua.LValue)
	if !ok {
		return fmt.Errorf("handler must be a Lua function")
	}
	b.app.RegisterList(pathPattern, fn)
	return nil
}

func (b *luaBackend) Environment() map[string]string {
	env := make(map[string]string)
	for _, e := range os.Environ() {
		if idx := strings.Index(e, "="); idx != -1 {
			env[e[:idx]] = e[idx+1:]
		}
	}
	return env
}

func (b *luaBackend) Log(msg string) {
	debugf("LUA LOG: %s\n", msg)
}

func (b *luaBackend) Debug(msg string) {
	if debugLogging {
		debugf("LUA DEBUG: %s\n", msg)
	}
}
