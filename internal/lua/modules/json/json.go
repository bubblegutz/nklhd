// Package json provides Lua bindings for JSON serialization.
package json

import (
	lua "github.com/yuin/gopher-lua"
	"github.com/bubblegutz/nklhd/internal/exports/json"
	"github.com/bubblegutz/nklhd/internal/lua/modules"
)

// Register adds the json module to the given Lua state.
// The module provides three functions:
//   json.encode(value) -> string, error
//   json.decode(string) -> value, error
//   json.fromYAML(string) -> string, error
func Register(L *lua.LState) {
	mod := L.RegisterModule("json", map[string]lua.LGFunction{
		"encode": encode,
		"decode": decode,
		"fromYAML": fromYAML,
	}).(*lua.LTable)
	L.SetGlobal("json", mod)
}

func encode(L *lua.LState) int {
	v := L.CheckAny(1)
	goVal, err := modules.LuaValueToGo(v)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("convert error: " + err.Error()))
		return 2
	}
	result, err := json.Encode(goVal)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("encode error: " + err.Error()))
		return 2
	}
	L.Push(lua.LString(result))
	L.Push(lua.LNil)
	return 2
}

func decode(L *lua.LState) int {
	s := L.CheckString(1)
	data, err := json.Decode(s)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("decode error: " + err.Error()))
		return 2
	}
	lv, err := modules.GoToLuaValue(L, data)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("convert error: " + err.Error()))
		return 2
	}
	L.Push(lv)
	L.Push(lua.LNil)
	return 2
}

func fromYAML(L *lua.LState) int {
	s := L.CheckString(1)
	result, err := json.FromYAML(s)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("fromYAML error: " + err.Error()))
		return 2
	}
	L.Push(lua.LString(result))
	L.Push(lua.LNil)
	return 2
}


