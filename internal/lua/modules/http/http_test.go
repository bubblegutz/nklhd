package http

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
	json "github.com/bubblegutz/nklhd/internal/lua/modules/json"
)

func TestHttpModuleRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello world"))
	}))
	defer server.Close()

	L := lua.NewState()
	defer L.Close()
	Register(L)
	json.Register(L)

	script := fmt.Sprintf(`
		local http = require("http")
		local result, err = http.request("GET", %q, {})
		if err ~= nil then
			error("request error: " .. err)
		end
		if result.status ~= 200 then
			error("expected status 200, got " .. tostring(result.status))
		end
		if result.body ~= "hello world" then
			error("body mismatch")
		end
	`, server.URL)
	err := L.DoString(script)
	if err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}
}

func TestHttpModuleRequestWithHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "value" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	L := lua.NewState()
	defer L.Close()
	Register(L)
	json.Register(L)

	script := fmt.Sprintf(`
		local http = require("http")
		local opts = {
			headers = {["X-Custom"] = "value"}
		}
		local result, err = http.request("GET", %q, opts)
		if err ~= nil then
			error("request error: " .. err)
		end
		if result.status ~= 200 then
			error("expected status 200, got " .. tostring(result.status))
		end
	`, server.URL)
	err := L.DoString(script)
	if err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}
}

func TestHttpModuleRequestWithBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		// Read body
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		if string(body) != "post data" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("body mismatch"))
			return
		}
		w.Write([]byte("received"))
	}))
	defer server.Close()

	L := lua.NewState()
	defer L.Close()
	Register(L)
	json.Register(L)

	script := fmt.Sprintf(`
		local http = require("http")
		local opts = {
			body = "post data"
		}
		local result, err = http.request("POST", %q, opts)
		if err ~= nil then
			error("request error: " .. err)
		end
		if result.status ~= 200 then
			error("expected status 200, got " .. tostring(result.status))
		end
		if result.body ~= "received" then
			error("body mismatch")
		end
	`, server.URL)
	err := L.DoString(script)
	if err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}
}

func TestHttpModuleClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/test" {
			w.Write([]byte("test ok"))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	L := lua.NewState()
	defer L.Close()
	Register(L)
	json.Register(L)

	script := fmt.Sprintf(`
		local http = require("http")
		local client = http.client(%q, {})
		local result, err = client:get("/api/test", {})
		if err ~= nil then
			error("client get error: " .. err)
		end
		if result.status ~= 200 then
			error("expected status 200, got " .. tostring(result.status))
		end
		if result.body ~= "test ok" then
			error("body mismatch")
		end
	`, server.URL)
	err := L.DoString(script)
	if err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}
}

func TestHttpModuleClientPostJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("missing content-type"))
			return
		}
		// Simulate echo
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		w.Write(body)
	}))
	defer server.Close()

	L := lua.NewState()
	defer L.Close()
	Register(L)
	json.Register(L)

	script := fmt.Sprintf(`
		local http = require("http")
		local client = http.client(%q, {})
		local data = {name = "alice", age = 30}
		local result, err = client:post("/echo", data, {})
		if err ~= nil then
			error("client post error: " .. err)
		end
		if result.status ~= 200 then
			error("expected status 200, got " .. tostring(result.status))
		end
		-- response should be JSON encoded data
		local json = require("json")
		local decoded, err = json.decode(result.body)
		if err ~= nil then
			error("decode response error: " .. err)
		end
		if decoded.name ~= "alice" then
			error("name mismatch")
		end
	`, server.URL)
	err := L.DoString(script)
	if err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}
}

func TestHttpModuleClientWithBasicAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "admin" || pass != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte("authenticated"))
	}))
	defer server.Close()

	L := lua.NewState()
	defer L.Close()
	Register(L)
	json.Register(L)

	script := fmt.Sprintf(`
		local http = require("http")
		local client = http.client(%q, {})
		client:withBasic("admin", "secret")
		local result, err = client:get("/", {})
		if err ~= nil then
			error("client get error: " .. err)
		end
		if result.status ~= 200 then
			error("expected status 200, got " .. tostring(result.status))
		end
		if result.body ~= "authenticated" then
			error("body mismatch")
		end
	`, server.URL)
	err := L.DoString(script)
	if err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}
}

func TestHttpModuleClientWithBearerAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer mytoken" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte("token ok"))
	}))
	defer server.Close()

	L := lua.NewState()
	defer L.Close()
	Register(L)
	json.Register(L)

	script := fmt.Sprintf(`
		local http = require("http")
		local client = http.client(%q, {})
		client:withBearer("mytoken")
		local result, err = client:get("/", {})
		if err ~= nil then
			error("client get error: " .. err)
		end
		if result.status ~= 200 then
			error("expected status 200, got " .. tostring(result.status))
		end
		if result.body ~= "token ok" then
			error("body mismatch")
		end
	`, server.URL)
	err := L.DoString(script)
	if err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}
}

func TestHttpModuleRequestTimeout(t *testing.T) {
	// Create a server that delays response longer than timeout
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte("too late"))
	}))
	defer server.Close()

	L := lua.NewState()
	defer L.Close()
	Register(L)
	json.Register(L)

	script := fmt.Sprintf(`
		local http = require("http")
		local opts = {timeout_ms = 50}
		local result, err = http.request("GET", %q, opts)
		if err == nil then
			error("expected timeout error")
		end
	`, server.URL)
	err := L.DoString(script)
	if err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}
}

func TestHttpModuleClientTLSInsecure(t *testing.T) {
	// Create TLS server with self-signed cert
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("tls ok"))
	}))
	defer server.Close()

	L := lua.NewState()
	defer L.Close()
	Register(L)
	json.Register(L)

	script := fmt.Sprintf(`
		local http = require("http")
		local client = http.client(%q, {
			tls = {insecure_skip_verify = true}
		})
		local result, err = client:get("/", {})
		if err ~= nil then
			error("client get error: " .. err)
		end
		if result.status ~= 200 then
			error("expected status 200, got " .. tostring(result.status))
		end
		if result.body ~= "tls ok" then
			error("body mismatch")
		end
	`, server.URL)
	err := L.DoString(script)
	if err != nil {
		t.Fatalf("Lua script failed: %v", err)
	}
}