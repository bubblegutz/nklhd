package modules

import (
	"testing"
	lua "github.com/yuin/gopher-lua"
)

func TestLuaValueToGo(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	tests := []struct {
		name     string
		setup    func() lua.LValue
		expected interface{}
	}{
		{"nil", func() lua.LValue { return lua.LNil }, nil},
		{"bool true", func() lua.LValue { return lua.LBool(true) }, true},
		{"bool false", func() lua.LValue { return lua.LBool(false) }, false},
		{"number int", func() lua.LValue { return lua.LNumber(42) }, float64(42)},
		{"number float", func() lua.LValue { return lua.LNumber(3.14) }, 3.14},
		{"string", func() lua.LValue { return lua.LString("hello") }, "hello"},
		{"array", func() lua.LValue {
			tbl := L.NewTable()
			tbl.Append(lua.LString("a"))
			tbl.Append(lua.LString("b"))
			tbl.Append(lua.LString("c"))
			return tbl
		}, []interface{}{"a", "b", "c"}},
		{"object", func() lua.LValue {
			tbl := L.NewTable()
			tbl.RawSetString("key", lua.LString("value"))
			tbl.RawSetString("num", lua.LNumber(123))
			return tbl
		}, map[string]interface{}{"key": "value", "num": float64(123)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lv := tt.setup()
			got, err := LuaValueToGo(lv)
			if err != nil {
				t.Fatalf("LuaValueToGo failed: %v", err)
			}
			// Simple equality checks (not deep)
			switch g := got.(type) {
			case nil:
				if tt.expected != nil {
					t.Errorf("got nil, expected %v", tt.expected)
				}
			case bool:
				if g != tt.expected.(bool) {
					t.Errorf("got %v, expected %v", g, tt.expected)
				}
			case float64:
				if g != tt.expected.(float64) {
					t.Errorf("got %v, expected %v", g, tt.expected)
				}
			case string:
				if g != tt.expected.(string) {
					t.Errorf("got %v, expected %v", g, tt.expected)
				}
			case []interface{}:
				exp := tt.expected.([]interface{})
				if len(g) != len(exp) {
					t.Errorf("array length mismatch: got %d, expected %d", len(g), len(exp))
				}
			case map[string]interface{}:
				exp := tt.expected.(map[string]interface{})
				if len(g) != len(exp) {
					t.Errorf("map length mismatch: got %d, expected %d", len(g), len(exp))
				}
			}
		})
	}
}

func TestGoToLuaValue(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	tests := []struct {
		name     string
		input    interface{}
		check    func(lv lua.LValue) bool
	}{
		{"nil", nil, func(lv lua.LValue) bool { return lv.Type() == lua.LTNil }},
		{"bool true", true, func(lv lua.LValue) bool { return lv == lua.LBool(true) }},
		{"bool false", false, func(lv lua.LValue) bool { return lv == lua.LBool(false) }},
		{"int", 42, func(lv lua.LValue) bool { return lv == lua.LNumber(42) }},
		{"float64", 3.14, func(lv lua.LValue) bool { return lv == lua.LNumber(3.14) }},
		{"string", "test", func(lv lua.LValue) bool { return lv == lua.LString("test") }},
		{"array", []interface{}{1, "a", true}, func(lv lua.LValue) bool {
			tbl, ok := lv.(*lua.LTable)
			if !ok {
				return false
			}
			return tbl.Len() == 3
		}},
		{"map", map[string]interface{}{"k": "v"}, func(lv lua.LValue) bool {
			tbl, ok := lv.(*lua.LTable)
			if !ok {
				return false
			}
			v := tbl.RawGetString("k")
			return v == lua.LString("v")
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lv, err := GoToLuaValue(L, tt.input)
			if err != nil {
				t.Fatalf("GoToLuaValue failed: %v", err)
			}
			if !tt.check(lv) {
				t.Errorf("conversion mismatch: got %v", lv)
			}
		})
	}
}
