-- HTTP module example
-- Demonstrates the http.request and http.client APIs

app.log("Loading HTTP example script")

-- Route that displays HTTP module documentation
app.read("/http/help", function(req)
    return [[
HTTP Module Example
===================
Available routes:
  /http/help         - This help message
  /http/request      - Demonstrate http.request
  /http/client       - Demonstrate http.client
  /http/headers      - Show request headers via httpbin.org
  /http/status       - Check HTTP status codes
  /http/post         - POST with JSON body
  /http/auth         - Basic auth example
  /http/timeout      - Timeout configuration

Use http.request(method, url, opts) to make HTTP requests.
Use http.client(baseURL, opts) to create a reusable client.

See the source code for examples.
]]
end)

app.readdir("/http", function(req)
    return "help\nrequest\nclient\nheaders\nstatus\npost\nauth\ntimeout"
end)

-- Demonstrate http.request with a public test service (httpbin.org)
app.read("/http/headers", function(req)
    local http = require("http")
    local result, err = http.request("GET", "https://httpbin.org/headers", {
        headers = {["User-Agent"] = "nklhd-http-example"}
    })
    if err ~= nil then
        return "Error: " .. err
    end
    -- Pretty-print JSON response using json module
    local json = require("json")
    local decoded, err2 = json.decode(result.body)
    if err2 ~= nil then
        return "JSON decode error: " .. err2 .. "\nRaw response:\n" .. result.body
    end
    -- Extract headers part
    local headers = decoded.headers
    local out = "Response headers from httpbin.org:\n"
    for k, v in pairs(headers) do
        out = out .. string.format("  %s: %s\n", k, v)
    end
    return out
end)

-- Demonstrate http.client with a base URL
app.read("/http/status", function(req)
    local http = require("http")
    -- Create a client for httpbin.org
    local client = http.client("https://httpbin.org", {})
    local result, err = client:get("/status/200", {})
    if err ~= nil then
        return "Error: " .. err
    end
    return string.format("Status: %d\nBody: %s", result.status, result.body)
end)

-- Demonstrate POST with JSON body
app.write("/http/post", function(req)
    local http = require("http")
    local json = require("json")
    -- Parse incoming JSON body
    local data, err = json.decode(req.data)
    if err ~= nil then
        return "Invalid JSON: " .. err
    end
    -- Send to httpbin.org/post
    local result, err2 = http.request("POST", "https://httpbin.org/post", {
        headers = {["Content-Type"] = "application/json"},
        body = req.data
    })
    if err2 ~= nil then
        return "HTTP error: " .. err2
    end
    -- Return the response from httpbin
    return result.body
end)

-- Demonstrate client with authentication (just example, no real auth)
app.read("/http/auth", function(req)
    local http = require("http")
    local client = http.client("https://httpbin.org", {})
    client:withBasic("user", "passwd")
    local result, err = client:get("/basic-auth/user/passwd", {})
    if err ~= nil then
        return "Error: " .. err
    end
    return string.format("Auth test: status %d\n%s", result.status, result.body)
end)

-- Demonstrate timeout configuration
app.read("/http/timeout", function(req)
    local http = require("http")
    -- This request will timeout quickly (10ms) because httpbin.org/delay/10 waits 10 seconds
    local result, err = http.request("GET", "https://httpbin.org/delay/10", {
        timeout_ms = 10
    })
    if err ~= nil then
        return "Expected timeout error: " .. err
    end
    return "Unexpected success: " .. result.body
end)

app.log("HTTP example routes registered")