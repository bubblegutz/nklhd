package oauth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	lua "github.com/yuin/gopher-lua"
)

func TestOAuthModule(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	Register(L)

	// Test that the module is loaded
	err := L.DoString(`
		local oauth = require("oauth")
		if type(oauth) ~= "table" then
			error("expected oauth module table")
		end
		if type(oauth.client) ~= "function" then
			error("expected oauth.client function")
		end
	`)
	if err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}
}

func TestOAuthClientCreation(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	Register(L)

	script := `
		local oauth = require("oauth")
		local cfg = {
			client_id = "test-client",
			token_url = "https://example.com/token",
			device_url = "https://example.com/device",
			timeout_ms = 5000,
			tls = { insecure_skip_verify = true }
		}
		local client = oauth.client(cfg)
		if type(client) ~= "userdata" then
			error("expected userdata, got " .. type(client))
		end
	`
	err := L.DoString(script)
	if err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}
}

func TestOAuthDeviceFlowStart(t *testing.T) {
	// Mock OAuth device endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"device_code": "dev123",
			"user_code": "USER123",
			"verification_uri": "https://example.com/verify"
		}`))
	}))
	defer server.Close()

	L := lua.NewState()
	defer L.Close()
	Register(L)

	// Inject server URL into Lua globals
	L.SetGlobal("TEST_DEVICE_URL", lua.LString(server.URL))

	script := `
		local oauth = require("oauth")
		local cfg = {
			client_id = "test-client",
			token_url = "https://example.com/token",  -- not used in this call
			device_url = TEST_DEVICE_URL,
		}
		local client = oauth.client(cfg)
		local data, err = client:device_flow_start("scope")
		if err ~= nil then
			error("device_flow_start error: " .. tostring(err))
		end
		if type(data) ~= "table" then
			error("expected table, got " .. type(data))
		end
		if data.device_code ~= "dev123" then
			error("device_code mismatch: " .. tostring(data.device_code))
		end
		if data.user_code ~= "USER123" then
			error("user_code mismatch: " .. tostring(data.user_code))
		end
	`
	err := L.DoString(script)
	if err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}
}

func TestOAuthDevicePoll(t *testing.T) {
	var attempt int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"error":"authorization_pending"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"token123","token_type":"bearer"}`))
	}))
	defer server.Close()

	L := lua.NewState()
	defer L.Close()
	Register(L)

	L.SetGlobal("TEST_TOKEN_URL", lua.LString(server.URL))

	script := `
		local oauth = require("oauth")
		local cfg = {
			client_id = "test-client",
			token_url = TEST_TOKEN_URL,
			device_url = "https://example.com/device",
		}
		local client = oauth.client(cfg)
		local data, err = client:device_poll("device123", 1, 2)
		if err ~= nil then
			error("device_poll error: " .. tostring(err))
		end
		if type(data) ~= "table" then
			error("expected table, got " .. type(data))
		end
		if data.access_token ~= "token123" then
			error("access_token mismatch: " .. tostring(data.access_token))
		end
	`
	err := L.DoString(script)
	if err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}
}

func TestOAuthAuthCodeURL(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	Register(L)

	script := `
		local oauth = require("oauth")
		local cfg = {
			client_id = "client-id",
			token_url = "https://example.com/token",
			device_url = "https://example.com/device",
		}
		local client = oauth.client(cfg)
		local url, err = client:auth_code_url("https://example.com/auth", "read write", "state123")
		if err ~= nil then
			error("auth_code_url error: " .. tostring(err))
		end
		if type(url) ~= "string" then
			error("expected string, got " .. type(url))
		end
		-- check that URL contains expected parameters
		if not url:find("client_id=client%-id") then
			error("missing client_id in URL")
		end
		if not url:find("scope=read%+write") then
			error("missing scope in URL")
		end
		if not url:find("state=state123") then
			error("missing state in URL")
		end
	`
	err := L.DoString(script)
	if err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}
}

func TestOAuthExchangeCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"exchanged","refresh_token":"refresh"}`))
	}))
	defer server.Close()

	L := lua.NewState()
	defer L.Close()
	Register(L)

	L.SetGlobal("TEST_TOKEN_URL", lua.LString(server.URL))

	script := `
		local oauth = require("oauth")
		local cfg = {
			client_id = "client-id",
			token_url = TEST_TOKEN_URL,
			device_url = "https://example.com/device",
		}
		local client = oauth.client(cfg)
		local data, err = client:exchange_code("authcode", "https://example.com/callback", "secret")
		if err ~= nil then
			error("exchange_code error: " .. tostring(err))
		end
		if type(data) ~= "table" then
			error("expected table, got " .. type(data))
		end
		if data.access_token ~= "exchanged" then
			error("access_token mismatch: " .. tostring(data.access_token))
		end
	`
	err := L.DoString(script)
	if err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}
}