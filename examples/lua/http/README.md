# HTTP Client Example

This example demonstrates the HTTP client module for making HTTP requests from Lua scripts in nklhd.

## Overview

The HTTP module provides two primary APIs:
- `http.request(method, url, options)` - Make a single HTTP request
- `http.client(baseURL, options)` - Create a reusable HTTP client with methods

The module automatically includes support for:
- JSON serialization of Lua tables (when POSTing/PUTing tables)
- Basic and Bearer authentication
- Custom headers and timeouts
- TLS configuration (insecure skip verify for development)

## Features Demonstrated

- **Simple HTTP requests** to public APIs (httpbin.org)
- **Reusable HTTP clients** with base URLs and authentication
- **JSON serialization** automatic when POSTing Lua tables
- **Timeout handling** with configurable timeout_ms option
- **Authentication** via Basic and Bearer tokens
- **TLS configuration** for development/testing

## Running the Example

1. **Build nklhd** (if not already built):
   ```bash
   go build
   ```

2. **Run the HTTP example**:
   ```bash
   ./nklhd --config examples/http/config.toml --verbose
   ```

3. **Explore the mounted filesystem**:
   ```bash
   ls /tmp/nklhd-http/
   ls /tmp/nklhd-http/http/
   cat /tmp/nklhd-http/http/headers
   cat /tmp/nklhd-http/http/status
   cat /tmp/nklhd-http/http/auth
   cat /tmp/nklhd-http/http/timeout
   ```

## Available Routes

The example creates the following routes:

| Route | Method | Description |
|-------|--------|-------------|
| `/http` | GET | Documentation and route list |
| `/http/headers` | GET | Fetch request headers from httpbin.org |
| `/http/status` | GET | Test HTTP status codes via httpbin.org |
| `/http/post` | POST | POST JSON data to httpbin.org (requires JSON body) |
| `/http/auth` | GET | Test Basic authentication |
| `/http/timeout` | GET | Demonstrate timeout handling |

## HTTP Module API Reference

### `http.request(method, url, options)`

Make a single HTTP request.

**Parameters:**
- `method` (string): HTTP method (GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS)
- `url` (string): Full URL to request
- `options` (table, optional): Request options (see below)

**Returns:** `(result, error)` where `result` is a table or `nil` on error, `error` is error message.

**Result table structure:**
```lua
{
  status = 200,          -- HTTP status code
  body = "...",          -- Response body as string
  headers = {            -- Response headers as table
    ["Content-Type"] = "application/json",
    ...
  }
}
```

**Options table:**
```lua
{
  headers = { ["X-Custom"] = "value" },  -- Request headers
  body = "request body",                 -- Request body (string or table)
  timeout_ms = 5000,                     -- Timeout in milliseconds (default 5000)
  tls = { insecure_skip_verify = true }  -- TLS configuration
}
```

**Examples:**
```lua
-- Simple GET request
local result, err = http.request("GET", "https://httpbin.org/headers")

-- POST JSON data (table automatically encoded)
local result, err = http.request("POST", "https://httpbin.org/post", {
  body = { name = "alice", age = 30 }
})

-- With custom headers and timeout
local result, err = http.request("GET", "https://api.example.com/data", {
  headers = { ["Authorization"] = "Bearer token123" },
  timeout_ms = 10000
})
```

### `http.client(baseURL, options)`

Create a reusable HTTP client with a base URL.

**Parameters:**
- `baseURL` (string): Base URL for all requests (e.g., "https://api.example.com/v1")
- `options` (table, optional): Default options for all requests (same as `request()` options)

**Returns:** `(client, error)` where `client` is a client object or `nil` on error.

**Client methods:**
- `client:get(path, options)` - GET request (no body)
- `client:post(path, body, options)` - POST request with body (string or table)
- `client:put(path, body, options)` - PUT request with body (string or table)
- `client:patch(path, body, options)` - PATCH request with body (string or table)
- `client:delete(path, options)` - DELETE request (no body)
- `client:head(path, options)` - HEAD request (no body)
- `client:options(path, options)` - OPTIONS request (no body)
- `client:withBasic(user, pass)` - Set Basic authentication
- `client:withBearer(token)` - Set Bearer token authentication
- `client:withTLSInsecure()` - Disable TLS certificate verification

**Examples:**
```lua
-- Create a client for an API
local client, err = http.client("https://api.example.com/v1")
client:withBasic("user", "password")

-- Make requests
local result, err = client:get("/users")
local result, err = client:post("/users", { name = "bob" })  -- table automatically JSON-encoded
local result, err = client:put("/users/123", { name = "charlie", age = 30 })
local result, err = client:patch("/users/123", { age = 31 })
local result, err = client:delete("/users/123")
local result, err = client:head("/users")
local result, err = client:options("/users")

-- With custom headers and options
local result, err = client:get("/users/active", {
  headers = { ["X-Custom"] = "value" },
  timeout_ms = 10000
})

-- POST with string body (not JSON)
local result, err = client:post("/upload", "raw text data", {
  headers = { ["Content-Type"] = "text/plain" }
})
```

### Automatic JSON Serialization

When the `body` option is a Lua table (not a string), it is automatically encoded as JSON with `Content-Type: application/json` header added.

```lua
-- This automatically becomes JSON
local result, err = http.request("POST", "https://httpbin.org/post", {
  body = { name = "alice", age = 30, active = true }
})

-- Equivalent to:
local result, err = http.request("POST", "https://httpbin.org/post", {
  headers = { ["Content-Type"] = "application/json" },
  body = '{"name":"alice","age":30,"active":true}'
})
```

### Error Handling

All functions return `(nil, error_message)` on error:

```lua
local result, err = http.request("GET", "https://invalid-url.example")
if err then
  print("Request failed:", err)
  -- err might be: "request error: Get \"https://...\": dial tcp: lookup ..."
end
```

Common errors:
- Network errors (connection refused, timeout, DNS lookup failure)
- HTTP errors (status >= 400)
- Invalid URLs or malformed requests
- JSON encoding/decoding errors

## Integration with Other Modules

The HTTP module works seamlessly with other nklhd modules:

```lua
local http = require("http")
local json = require("json")

-- Parse JSON response
local result, err = http.request("GET", "https://api.example.com/data")
if not err then
  local data, err2 = json.decode(result.body)
  if not err2 then
    -- Process data table
    return "Got: " .. data.value
  end
end
```

## Testing the Example

Test the example routes using standard filesystem operations:

```bash
# Test headers route
cat /tmp/nklhd-http/http/headers

# Test status route  
cat /tmp/nklhd-http/http/status

# Test POST route (requires JSON input)
echo '{"test": "data"}' > /tmp/nklhd-http/http/post
cat /tmp/nklhd-http/http/post

# Test authentication
cat /tmp/nklhd-http/http/auth

# Test timeout (will show error message)
cat /tmp/nklhd-http/http/timeout
```

## Configuration File

The example uses `config.toml`:

```toml
# HTTP client example configuration
# Demonstrates http module in Lua for making HTTP requests

# Mount point for the FUSE filesystem
mountpoint = "/tmp/nklhd-http"

# Lua script that defines routes (relative to this config file directory)
rootscript = "main.lua"

# Enable verbose debug logging
verbose = false

# Optional: maximum concurrent handlers
# max_handlers = 4
```

## Troubleshooting

### Network Issues
- Ensure network connectivity (httpbin.org is a public test service)
- Check firewall settings if using local APIs
- Use `verbose = true` in config to see debug logs

### TLS Certificate Errors
For development with self-signed certificates:
```lua
local client = http.client("https://localhost:8443")
client:withTLSInsecure()
```

### Timeout Errors
Increase timeout for slow networks:
```lua
local result, err = http.request("GET", "https://api.example.com", {
  timeout_ms = 30000  -- 30 seconds
})
```

### JSON Encoding Issues
Ensure tables are serializable (no functions, userdata, or circular references):
```lua
-- This will fail:
local bad_table = { func = function() end }

-- This works:
local good_table = { name = "test", values = {1, 2, 3} }
```