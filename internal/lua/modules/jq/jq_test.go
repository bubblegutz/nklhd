package jq

import (
	"testing"
	lua "github.com/yuin/gopher-lua"
)

func TestJqModule(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	Register(L)

	// Test jq.wrap with a table
	err := L.DoString(`
		local jq = require("jq")
		local tbl = {
			{name = "Alice", age = 30},
			{name = "Bob", age = 25},
			{name = "Charlie", age = 35},
		}
		local q = jq.wrap(tbl)
		if type(q) ~= "userdata" then
			error("expected userdata, got " .. type(q))
		end
		-- test count
		if q:count() ~= 3 then
			error("expected 3 items, got " .. tostring(q:count()))
		end
	`)
	if err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}
}

func TestJqFind(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	Register(L)

	err := L.DoString(`
		local jq = require("jq")
		local tbl = {
			{name = "Alice", age = 30},
			{title = "Engineer", department = "IT"},
		}
		local q = jq.wrap(tbl)
		local found = q:find("name")
		if found:count() ~= 1 then
			error("expected 1 match for 'name', got " .. tostring(found:count()))
		end
		local val = found:get()
		if val ~= "Alice" then
			error("expected 'Alice', got " .. tostring(val))
		end
	`)
	if err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}
}

func TestJqPluck(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	Register(L)

	err := L.DoString(`
		local jq = require("jq")
		local tbl = {
			{id = 1, name = "Alice"},
			{id = 2, name = "Bob"},
			{name = "Charlie"}, -- missing id
		}
		local q = jq.wrap(tbl)
		local ids = q:pluck("id")
		if ids:count() ~= 3 then
			error("expected 3 results, got " .. tostring(ids:count()))
		end
		local items = ids:get()
		-- items should be a table {1, 2, nil}
		if items[1] ~= 1 then
			error("expected ids[1]=1, got " .. tostring(items[1]))
		end
		if items[2] ~= 2 then
			error("expected ids[2]=2, got " .. tostring(items[2]))
		end
		if items[3] ~= nil then
			error("expected ids[3]=nil, got " .. tostring(items[3]))
		end
	`)
	if err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}
}

func TestJqWhere(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	Register(L)

	err := L.DoString(`
		local jq = require("jq")
		local tbl = {
			{role = "admin", name = "Alice"},
			{role = "user", name = "Bob"},
			{role = "admin", name = "Charlie"},
		}
		local q = jq.wrap(tbl)
		local admins = q:where("role", "admin")
		if admins:count() ~= 2 then
			error("expected 2 admins, got " .. tostring(admins:count()))
		end
		local firstAdmin = admins:first()
		if firstAdmin.name ~= "Alice" then
			error("expected first admin name Alice, got " .. tostring(firstAdmin.name))
		end
	`)
	if err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}
}

func TestJqHas(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	Register(L)

	err := L.DoString(`
		local jq = require("jq")
		local tbl = {
			{name = "Alice", age = 30},
			{name = "Bob"},
			{age = 25},
		}
		local q = jq.wrap(tbl)
		local withAge = q:has("age")
		if withAge:count() ~= 2 then
			error("expected 2 items with age, got " .. tostring(withAge:count()))
		end
	`)
	if err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}
}

func TestJqFirstLast(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	Register(L)

	err := L.DoString(`
		local jq = require("jq")
		local tbl = {"a", "b", "c"}
		local q = jq.wrap(tbl)
		if q:first() ~= "a" then
			error("first() expected 'a', got " .. tostring(q:first()))
		end
		if q:last() ~= "c" then
			error("last() expected 'c', got " .. tostring(q:last()))
		end
	`)
	if err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}
}

func TestJqGet(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	Register(L)

	err := L.DoString(`
		local jq = require("jq")
		-- single item
		local q1 = jq.wrap({42})
		local val = q1:get()
		if val ~= 42 then
			error("get() on single item expected 42, got " .. tostring(val))
		end
		-- multiple items
		local q2 = jq.wrap({10, 20, 30})
		local tbl = q2:get()
		if type(tbl) ~= "table" then
			error("get() on multiple items expected table, got " .. type(tbl))
		end
		if #tbl ~= 3 then
			error("expected table length 3, got " .. #tbl)
		end
	`)
	if err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}
}

func TestJqFilter(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	Register(L)

	err := L.DoString(`
		local jq = require("jq")
		local tbl = {5, 12, 8, 130, 44}
		local q = jq.wrap(tbl)
		local filtered = q:filter(function(val)
			return val > 10
		end)
		if filtered:count() ~= 3 then
			error("expected 3 items >10, got " .. tostring(filtered:count()))
		end
		local first = filtered:first()
		if first ~= 12 then
			error("first filtered item expected 12, got " .. tostring(first))
		end
	`)
	if err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}
}

func TestJqMap(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	Register(L)

	err := L.DoString(`
		local jq = require("jq")
		local tbl = {1, 2, 3}
		local q = jq.wrap(tbl)
		local mapped = q:map(function(val)
			return val * 2
		end)
		if mapped:count() ~= 3 then
			error("expected 3 mapped items, got " .. tostring(mapped:count()))
		end
		local items = mapped:get()
		if items[1] ~= 2 or items[2] ~= 4 or items[3] ~= 6 then
			error("mapped values mismatch")
		end
	`)
	if err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}
}

func TestJqEach(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	Register(L)

	err := L.DoString(`
		local jq = require("jq")
		local tbl = {10, 20, 30}
		local q = jq.wrap(tbl)
		local sum = 0
		q:each(function(val)
			sum = sum + val
		end)
		if sum ~= 60 then
			error("expected sum 60, got " .. tostring(sum))
		end
	`)
	if err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}
}