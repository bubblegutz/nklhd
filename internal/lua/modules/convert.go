// Package modules provides Lua-specific utilities for module implementations.
package modules

import (
	"errors"
	"fmt"
	lua "github.com/yuin/gopher-lua"
)

// LuaValueToGo converts a Lua value to a Go interface{} suitable for YAML/JSON marshaling.
func LuaValueToGo(lv lua.LValue) (interface{}, error) {
	switch lv.Type() {
	case lua.LTNil:
		return nil, nil
	case lua.LTBool:
		return lua.LVAsBool(lv), nil
	case lua.LTNumber:
		return float64(lua.LVAsNumber(lv)), nil
	case lua.LTString:
		return lua.LVAsString(lv), nil
	case lua.LTTable:
		tbl := lv.(*lua.LTable)
		max := 0
		count := 0
		tbl.ForEach(func(k, v lua.LValue) {
			count++
			if k.Type() == lua.LTNumber {
				n := int(lua.LVAsNumber(k))
				if n > max {
					max = n
				}
			}
		})
		if max > 0 && max == count {
			out := make([]interface{}, 0, max)
			for i := 1; i <= max; i++ {
				v := tbl.RawGetInt(i)
				gv, err := LuaValueToGo(v)
				if err != nil {
					out = append(out, nil)
				} else {
					out = append(out, gv)
				}
			}
			return out, nil
		}
		out := make(map[string]interface{}, count)
		var err error
		tbl.ForEach(func(k, v lua.LValue) {
			if err != nil {
				return
			}
			if k.Type() != lua.LTString {
				err = errors.New("only string keys allowed in object maps")
				return
			}
			key := lua.LVAsString(k)
			gv, gerr := LuaValueToGo(v)
			if gerr != nil {
				err = gerr
				return
			}
			out[key] = gv
		})
		if err != nil {
			return nil, err
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported lua type: %s", lv.Type().String())
	}
}

// GoToLuaValue converts a Go interface{} (as produced by YAML/JSON unmarshaling) to a Lua value.
func GoToLuaValue(L *lua.LState, v interface{}) (lua.LValue, error) {
	if v == nil {
		return lua.LNil, nil
	}
	switch vv := v.(type) {
	case bool:
		return lua.LBool(vv), nil
	case int:
		return lua.LNumber(vv), nil
	case int64:
		return lua.LNumber(vv), nil
	case float64:
		return lua.LNumber(vv), nil
	case string:
		return lua.LString(vv), nil
	case []interface{}:
		tbl := L.NewTable()
		for i, e := range vv {
			lv, err := GoToLuaValue(L, e)
			if err != nil {
				return nil, err
			}
			tbl.RawSetInt(i+1, lv)
		}
		return tbl, nil
	case map[string]interface{}:
		tbl := L.NewTable()
		for k, e := range vv {
			lv, err := GoToLuaValue(L, e)
			if err != nil {
				return nil, err
			}
			tbl.RawSetString(k, lv)
		}
		return tbl, nil
	default:
		return nil, fmt.Errorf("unsupported go type: %T", v)
	}
}
