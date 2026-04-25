-- OAuth module example
-- Demonstrates OAuth 2.0 client functionality for Lua scripts

app.log("Loading OAuth example script")

-- Route that displays documentation
app.read("/oauth/help", function(req)
    return [[
OAuth Module Example
====================
Available routes:
  /oauth/help            - This help message
  /oauth/client          - Show OAuth client configuration
  /oauth/device          - Demonstrate device flow (simulated)
  /oauth/device-poll     - Poll for device authorization
  /oauth/authcode        - Generate authorization code URL
  /oauth/exchange        - Simulate token exchange
  /oauth/attach          - Demonstrate attaching token to HTTP client

The oauth module provides OAuth 2.0 client functionality:
- Device Authorization Flow (for devices without browsers)
- Authorization Code Flow (for web applications)

IMPORTANT: This example uses placeholder endpoints. Replace with your own
OAuth provider URLs and credentials for real usage.

OAuth providers you can test with:
- GitHub: https://github.com/settings/developers
- Google: https://console.cloud.google.com/apis/credentials
- Okta: https://developer.okta.com
]]
end)

app.readdir("/oauth", function(req)
    return "help\nclient\ndevice\ndevice-poll\nauthcode\nexchange\nattach"
end)

-- Show OAuth client configuration
app.read("/oauth/client", function(req)
    local config = {
        client_id = "YOUR_CLIENT_ID",
        token_url = "https://provider.example.com/oauth/token",
        device_url = "https://provider.example.com/oauth/device/code",
        timeout_ms = 5000,
        tls = { insecure_skip_verify = false }
    }
    
    local json = require("json")
    local encoded, err = json.encode(config)
    if err then return "Error encoding JSON: " .. err end
    
    return "Example OAuth client configuration:\n\n" .. encoded .. "\n\n" ..
           "To create a client:\n" ..
           "  local client = oauth.client(config)"
end)

-- Demonstrate device flow start (simulated)
app.read("/oauth/device", function(req)
    -- In a real scenario, you would use real OAuth provider endpoints
    -- This is a simulation that shows the API usage
    local config = {
        client_id = "test_client",
        token_url = "https://provider.example.com/oauth/token",
        device_url = "https://provider.example.com/oauth/device/code"
    }
    
    local client = oauth.client(config)
    if not client then return "Failed to create OAuth client" end
    
    -- In real usage, you would call:
    -- local result, err = client:device_flow_start("read write")
    -- This would make an actual HTTP request to the device URL
    
    -- For this example, we return simulated response
    local simulatedResponse = {
        device_code = "DEVICE_CODE_123",
        user_code = "USER-CODE-456",
        verification_uri = "https://provider.example.com/activate",
        verification_uri_complete = "https://provider.example.com/activate?user_code=USER-CODE-456",
        expires_in = 1800,
        interval = 5
    }
    
    local json = require("json")
    local encoded, err = json.encode(simulatedResponse)
    if err then return "Error encoding JSON: " .. err end
    
    return "Simulated device flow response (requires real OAuth provider):\n\n" .. encoded .. "\n\n" ..
           "API usage:\n" ..
           "  local result, err = client:device_flow_start(\"read write\")\n" ..
           "  if err then print(\"Error:\", err) else\n" ..
           "    -- Tell user to visit verification_uri with user_code\n" ..
           "    print(\"Visit:\", result.verification_uri)\n" ..
           "    print(\"Enter code:\", result.user_code)\n" ..
           "  end"
end)

-- Demonstrate device flow polling (simulated)
app.write("/oauth/device-poll", function(req)
    local json = require("json")
    local data, err = json.decode(req.data)
    if err then return "Invalid JSON: " .. err end
    
    local deviceCode = data.device_code
    if not deviceCode then return "Missing device_code in request body" end
    
    -- In real usage, you would poll with the actual device code
    -- local result, err = client:device_poll(deviceCode, 5, 12)
    
    -- Simulated successful token response
    local simulatedToken = {
        access_token = "ACCESS_TOKEN_789",
        token_type = "bearer",
        expires_in = 3600,
        refresh_token = "REFRESH_TOKEN_ABC",
        scope = "read write"
    }
    
    local encoded, err2 = json.encode(simulatedToken)
    if err2 then return "Error encoding JSON: " .. err2 end
    
    return "Simulated token exchange (requires real OAuth provider):\n\n" .. encoded .. "\n\n" ..
           "API usage:\n" ..
           "  local result, err = client:device_poll(deviceCode, 5, 12)\n" ..
           "  -- interval=5 seconds, maxAttempts=12\n" ..
           "  if err then print(\"Polling error:\", err)\n" ..
           "  else print(\"Access token:\", result.access_token) end"
end)

-- Generate authorization code URL
app.read("/oauth/authcode", function(req)
    local config = {
        client_id = "YOUR_CLIENT_ID",
        token_url = "https://provider.example.com/oauth/token"
    }
    
    local client = oauth.client(config)
    if not client then return "Failed to create OAuth client" end
    
    -- Generate authorization URL
    local authEndpoint = "https://provider.example.com/oauth/authorize"
    local scope = "read write"
    local state = "random_state_123"
    
    local url, err = client:auth_code_url(authEndpoint, scope, state)
    if err then return "Error generating auth code URL: " .. err end
    
    return "Authorization code URL:\n" .. url .. "\n\n" ..
           "API usage:\n" ..
           "  local url, err = client:auth_code_url(authEndpoint, scope, state)\n" ..
           "  -- Redirect user to this URL\n" ..
           "  -- User will be redirected back to your redirect_uri with ?code=..."
end)

-- Simulate authorization code exchange
app.write("/oauth/exchange", function(req)
    local json = require("json")
    local data, err = json.decode(req.data)
    if err then return "Invalid JSON: " .. err end
    
    local code = data.code
    local redirectUri = data.redirect_uri or "https://yourapp.example.com/callback"
    local clientSecret = data.client_secret or ""
    
    if not code then return "Missing code in request body" end
    
    local config = {
        client_id = "YOUR_CLIENT_ID",
        token_url = "https://provider.example.com/oauth/token"
    }
    
    local client = oauth.client(config)
    if not client then return "Failed to create OAuth client" end
    
    -- In real usage, you would exchange the code for a token
    -- local result, err = client:exchange_code(code, redirectUri, clientSecret)
    
    -- Simulated token response
    local simulatedToken = {
        access_token = "ACCESS_TOKEN_XYZ",
        token_type = "bearer",
        expires_in = 3600,
        refresh_token = "REFRESH_TOKEN_DEF",
        scope = "read write"
    }
    
    local encoded, err2 = json.encode(simulatedToken)
    if err2 then return "Error encoding JSON: " .. err2 end
    
    return "Simulated token exchange (requires real OAuth provider):\n\n" .. encoded .. "\n\n" ..
           "API usage:\n" ..
           "  local result, err = client:exchange_code(code, redirectUri, clientSecret)\n" ..
           "  if err then print(\"Exchange error:\", err)\n" ..
           "  else\n" ..
           "    print(\"Access token:\", result.access_token)\n" ..
           "    -- Store token securely\n" ..
           "  end"
end)

-- Demonstrate attaching token to HTTP client
app.read("/oauth/attach", function(req)
    local http = require("http")
    local json = require("json")
    
    -- Create an HTTP client
    local httpClient = http.client("https://api.example.com")
    
    -- Create an OAuth client (simulated)
    local oauthConfig = {
        client_id = "YOUR_CLIENT_ID",
        token_url = "https://provider.example.com/oauth/token"
    }
    local oauthClient = oauth.client(oauthConfig)
    
    -- Simulated token
    local token = "ACCESS_TOKEN_123"
    
    -- Attach token to HTTP client (placeholder implementation)
    -- In the actual implementation, this would set up the Authorization header
    local success, err = oauthClient:attach_to(httpClient, token)
    
    local result = {
        http_client_created = httpClient ~= nil,
        oauth_client_created = oauthClient ~= nil,
        attachment_success = success or false,
        attachment_error = err
    }
    
    local encoded, err2 = json.encode(result)
    if err2 then return "Error encoding JSON: " .. err2 end
    
    return "Token attachment demonstration:\n\n" .. encoded .. "\n\n" ..
           "API usage:\n" ..
           "  local success, err = oauthClient:attach_to(httpClient, token)\n" ..
           "  -- This configures the HTTP client to include the token in requests"
end)

app.log("OAuth example routes registered")