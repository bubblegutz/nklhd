// Package jq provides Lua bindings for jQuery-like query operations.
package jq

import (
	lua "github.com/yuin/gopher-lua"
	exportsjq "github.com/bubblegutz/nklhd/internal/exports/jq"
	"github.com/bubblegutz/nklhd/internal/lua/modules"
)

// Register adds the jq module to the given Lua state.
// The module provides one function:
//   jq.wrap(value) -> jq_query userdata
func Register(L *lua.LState) {
	mod := L.RegisterModule("jq", map[string]lua.LGFunction{
		"wrap": wrap,
	}).(*lua.LTable)
	L.SetGlobal("jq", mod)
	
	// Register jq_query metatable
	mt := L.NewTypeMetatable("jq_query")
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"find":   find,
		"filter": filter,
		"map":    mapFn,
		"pluck":  pluck,
		"where":  where,
		"has":    has,
		"first":  first,
		"last":   last,
		"get":    get,
		"each":   each,
		"count":  count,
	}))
}

// luaValueToGoItems converts a Lua value to a slice of Go interfaces suitable for a Query.
// If v is a Lua table that is an array (sequential integer keys starting at 1), each element becomes an item.
// Otherwise, v becomes the single item in the slice.
func luaValueToGoItems(L *lua.LState, v lua.LValue) ([]interface{}, error) {
	if v.Type() != lua.LTTable {
		// Single non‑table value
		goVal, err := modules.LuaValueToGo(v)
		if err != nil {
			return nil, err
		}
		return []interface{}{goVal}, nil
	}
	// Table: convert to Go using the generic converter
	goVal, err := modules.LuaValueToGo(v)
	if err != nil {
		return nil, err
	}
	// If the converter returned a slice, treat each element as a separate item.
	if slice, ok := goVal.([]interface{}); ok {
		return slice, nil
	}
	// Otherwise (map or something else) wrap as single item.
	return []interface{}{goVal}, nil
}

// wrap implements jq.wrap(value) -> jq_query userdata
func wrap(L *lua.LState) int {
	v := L.CheckAny(1)
	items, err := luaValueToGoItems(L, v)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("wrap error: " + err.Error()))
		return 2
	}
	q := exportsjq.NewQuery(items)
	ud := L.NewUserData()
	ud.Value = q
	L.SetMetatable(ud, L.GetTypeMetatable("jq_query"))
	L.Push(ud)
	return 1
}

func checkQuery(L *lua.LState) *exportsjq.Query {
	ud := L.CheckUserData(1)
	if ud == nil {
		L.ArgError(1, "jq query expected")
		return nil
	}
	q, ok := ud.Value.(*exportsjq.Query)
	if !ok {
		L.ArgError(1, "jq query expected")
		return nil
	}
	return q
}

func find(L *lua.LState) int {
	q := checkQuery(L)
	key := L.CheckString(2)
	newQ := q.Find(key)
	ud := L.NewUserData()
	ud.Value = newQ
	L.SetMetatable(ud, L.GetTypeMetatable("jq_query"))
	L.Push(ud)
	return 1
}

func pluck(L *lua.LState) int {
	q := checkQuery(L)
	key := L.CheckString(2)
	newQ := q.Pluck(key)
	ud := L.NewUserData()
	ud.Value = newQ
	L.SetMetatable(ud, L.GetTypeMetatable("jq_query"))
	L.Push(ud)
	return 1
}

func where(L *lua.LState) int {
	q := checkQuery(L)
	key := L.CheckString(2)
	lval := L.CheckAny(3)
	gval, err := modules.LuaValueToGo(lval)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("where error: " + err.Error()))
		return 2
	}
	newQ := q.Where(key, gval)
	ud := L.NewUserData()
	ud.Value = newQ
	L.SetMetatable(ud, L.GetTypeMetatable("jq_query"))
	L.Push(ud)
	return 1
}

func has(L *lua.LState) int {
	q := checkQuery(L)
	key := L.CheckString(2)
	newQ := q.Has(key)
	ud := L.NewUserData()
	ud.Value = newQ
	L.SetMetatable(ud, L.GetTypeMetatable("jq_query"))
	L.Push(ud)
	return 1
}

func first(L *lua.LState) int {
	q := checkQuery(L)
	val := q.First()
	lval, err := modules.GoToLuaValue(L, val)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("first error: " + err.Error()))
		return 2
	}
	L.Push(lval)
	return 1
}

func last(L *lua.LState) int {
	q := checkQuery(L)
	val := q.Last()
	lval, err := modules.GoToLuaValue(L, val)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("last error: " + err.Error()))
		return 2
	}
	L.Push(lval)
	return 1
}

func get(L *lua.LState) int {
	q := checkQuery(L)
	items := q.Items()
	// If there is exactly one item, return that item directly.
	// Otherwise, return a Lua array containing all items.
	if len(items) == 1 {
		lval, err := modules.GoToLuaValue(L, items[0])
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("get error: " + err.Error()))
			return 2
		}
		L.Push(lval)
		return 1
	}
	tbl := L.NewTable()
	for i, v := range items {
		lv, err := modules.GoToLuaValue(L, v)
		if err != nil {
			// On error, insert nil
			tbl.RawSetInt(i+1, lua.LNil)
		} else {
			tbl.RawSetInt(i+1, lv)
		}
	}
	L.Push(tbl)
	return 1
}

func count(L *lua.LState) int {
	q := checkQuery(L)
	n := q.Count()
	L.Push(lua.LNumber(n))
	return 1
}

// filter implements query:filter(fn) by iterating over items and calling the Lua function.
func filter(L *lua.LState) int {
	q := checkQuery(L)
	fn := L.CheckFunction(2)
	items := q.Items()
	out := make([]interface{}, 0, len(items))
	for i, v := range items {
		lv, err := modules.GoToLuaValue(L, v)
		if err != nil {
			continue
		}
		if err := L.CallByParam(lua.P{
			Fn:      fn,
			NRet:    1,
			Protect: true,
		}, lv, lua.LNumber(i+1)); err == nil {
			ret := L.Get(-1)
			L.Pop(1)
			if lua.LVAsBool(ret) {
				out = append(out, v)
			}
		} else {
			L.Pop(1)
		}
	}
	newQ := exportsjq.NewQuery(out)
	ud := L.NewUserData()
	ud.Value = newQ
	L.SetMetatable(ud, L.GetTypeMetatable("jq_query"))
	L.Push(ud)
	return 1
}

// mapFn implements query:map(fn) by iterating over items and calling the Lua function.
func mapFn(L *lua.LState) int {
	q := checkQuery(L)
	fn := L.CheckFunction(2)
	items := q.Items()
	out := make([]interface{}, 0, len(items))
	for i, v := range items {
		lv, err := modules.GoToLuaValue(L, v)
		if err != nil {
			out = append(out, nil)
			continue
		}
		if err := L.CallByParam(lua.P{
			Fn:      fn,
			NRet:    1,
			Protect: true,
		}, lv, lua.LNumber(i+1)); err == nil {
			ret := L.Get(-1)
			L.Pop(1)
			// Convert the Lua return value back to Go for storage
			goret, gerr := modules.LuaValueToGo(ret)
			if gerr != nil {
				out = append(out, nil)
			} else {
				out = append(out, goret)
			}
		} else {
			out = append(out, nil)
		}
	}
	newQ := exportsjq.NewQuery(out)
	ud := L.NewUserData()
	ud.Value = newQ
	L.SetMetatable(ud, L.GetTypeMetatable("jq_query"))
	L.Push(ud)
	return 1
}

// each implements query:each(fn) by iterating over items and calling the Lua function.
func each(L *lua.LState) int {
	q := checkQuery(L)
	fn := L.CheckFunction(2)
	items := q.Items()
	for i, v := range items {
		lv, err := modules.GoToLuaValue(L, v)
		if err != nil {
			continue
		}
		_ = L.CallByParam(lua.P{
			Fn:      fn,
			NRet:    0,
			Protect: true,
		}, lv, lua.LNumber(i+1))
	}
	L.Push(lua.LTrue)
	return 1
}