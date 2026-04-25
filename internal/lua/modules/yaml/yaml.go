// Package yaml provides Lua bindings for YAML serialization.
package yaml

import (
	lua "github.com/yuin/gopher-lua"
	"github.com/bubblegutz/nklhd/internal/exports/yaml"
	"github.com/bubblegutz/nklhd/internal/lua/modules"
)

// Register adds the yaml module to the given Lua state.
// The module provides three functions:
//   yaml.encode(value) -> string, error
//   yaml.decode(string) -> value, error
//   yaml.fromJSON(string) -> string, error
func Register(L *lua.LState) {
	mod := L.RegisterModule("yaml", map[string]lua.LGFunction{
		"encode": encode,
		"decode": decode,
		"fromJSON": fromJSON,
	}).(*lua.LTable)
	L.SetGlobal("yaml", mod)
}

func encode(L *lua.LState) int {
	v := L.CheckAny(1)
	goVal, err := modules.LuaValueToGo(v)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("convert error: " + err.Error()))
		return 2
	}
	result, err := yaml.Encode(goVal)
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
	data, err := yaml.Decode(s)
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

func fromJSON(L *lua.LState) int {
	s := L.CheckString(1)
	result, err := yaml.FromJSON(s)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("fromJSON error: " + err.Error()))
		return 2
	}
	L.Push(lua.LString(result))
	L.Push(lua.LNil)
	return 2
}


