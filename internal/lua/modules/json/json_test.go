package json

import (
	"testing"
	lua "github.com/yuin/gopher-lua"
)

func TestJsonModule(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	Register(L)

	// Test json.encode
	err := L.DoString(`
		local json = require("json")
		local tbl = {name = "alice", age = 30, tags = {"a", "b"}}
		local encoded, err = json.encode(tbl)
		if err ~= nil then
			error("encode error: " .. err)
		end
		if type(encoded) ~= "string" then
			error("expected string, got " .. type(encoded))
		end
	`)
	if err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}

	// Test json.decode
	err = L.DoString(`
		local json = require("json")
		local encoded = '{"name":"bob","score":95.5}'
		local decoded, err = json.decode(encoded)
		if err ~= nil then
			error("decode error: " .. err)
		end
		if type(decoded) ~= "table" then
			error("expected table, got " .. type(decoded))
		end
		if decoded.name ~= "bob" then
			error("expected name=bob, got " .. tostring(decoded.name))
		end
	`)
	if err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}
}

func TestJsonEncodeDecodeRoundtrip(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	Register(L)

	script := `
		local json = require("json")
		local original = {
			name = "test",
			numbers = {1, 2, 3},
			nested = {foo = "bar"}
		}
		local encoded, err = json.encode(original)
		if err ~= nil then
			error("encode error: " .. err)
		end
		local decoded, err = json.decode(encoded)
		if err ~= nil then
			error("decode error: " .. err)
		end
		-- Basic check
		if decoded.name ~= original.name then
			error("name mismatch")
		end
		if #decoded.numbers ~= #original.numbers then
			error("numbers length mismatch")
		end
		if decoded.nested.foo ~= original.nested.foo then
			error("nested mismatch")
		end
	`
	err := L.DoString(script)
	if err != nil {
		t.Fatalf("roundtrip failed: %v", err)
	}
}

func TestJsonFromYAML(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	Register(L)

	script := `
		local json = require("json")
		local yaml = "name: alice\nage: 30"
		local jsonStr, err = json.fromYAML(yaml)
		if err ~= nil then
			error("fromYAML error: " .. err)
		end
		if type(jsonStr) ~= "string" then
			error("expected string, got " .. type(jsonStr))
		end
		-- decode to verify structure
		local decoded, err = json.decode(jsonStr)
		if err ~= nil then
			error("decode error: " .. err)
		end
		if decoded.name ~= "alice" then
			error("expected name=alice, got " .. tostring(decoded.name))
		end
		if decoded.age ~= 30 then
			error("expected age=30, got " .. tostring(decoded.age))
		end
	`
	err := L.DoString(script)
	if err != nil {
		t.Fatalf("fromYAML test failed: %v", err)
	}
}
