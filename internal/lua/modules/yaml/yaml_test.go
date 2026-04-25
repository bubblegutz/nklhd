package yaml

import (
	"testing"
	lua "github.com/yuin/gopher-lua"
)

func TestYamlModule(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	Register(L)

	// Test yaml.encode
	err := L.DoString(`
		local yaml = require("yaml")
		local tbl = {name = "alice", age = 30, tags = {"a", "b"}}
		local encoded, err = yaml.encode(tbl)
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

	// Test yaml.decode
	err = L.DoString(`
		local yaml = require("yaml")
		local encoded = 'name: bob\nscore: 95.5\n'
		local decoded, err = yaml.decode(encoded)
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

func TestYamlEncodeDecodeRoundtrip(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	Register(L)

	script := `
		local yaml = require("yaml")
		local original = {
			name = "test",
			numbers = {1, 2, 3},
			nested = {foo = "bar"}
		}
		local encoded, err = yaml.encode(original)
		if err ~= nil then
			error("encode error: " .. err)
		end
		local decoded, err = yaml.decode(encoded)
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

func TestYamlFromJSON(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	Register(L)

	script := `
		local yaml = require("yaml")
		local json = '{"name":"bob","score":95.5}'
		local yamlStr, err = yaml.fromJSON(json)
		if err ~= nil then
			error("fromJSON error: " .. err)
		end
		if type(yamlStr) ~= "string" then
			error("expected string, got " .. type(yamlStr))
		end
		-- decode to verify structure
		local decoded, err = yaml.decode(yamlStr)
		if err ~= nil then
			error("decode error: " .. err)
		end
		if decoded.name ~= "bob" then
			error("expected name=bob, got " .. tostring(decoded.name))
		end
		if decoded.score ~= 95.5 then
			error("expected score=95.5, got " .. tostring(decoded.score))
		end
	`
	err := L.DoString(script)
	if err != nil {
		t.Fatalf("fromJSON test failed: %v", err)
	}
}
