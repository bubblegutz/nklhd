# OAuth Module Example

This example demonstrates the OAuth module for OAuth 2.0 client functionality in Lua scripts within nklhd.

## Overview

The OAuth module provides OAuth 2.0 client functionality for both Device Authorization Flow (for devices without browsers) and Authorization Code Flow (for web applications). It handles token exchange, polling, and can attach tokens to HTTP clients for authenticated requests.

## Features Demonstrated

- **OAuth client creation** with configuration (client ID, token URLs, TLS settings)
- **Device Authorization Flow** for limited-input devices
- **Authorization Code Flow** for web applications
- **Token exchange** for converting authorization codes to access tokens
- **Token persistence** callback support
- **HTTP client integration** for attaching tokens to requests

## Running the Example

1. **Build nklhd** (if not already built):
   ```bash
   go build
   ```

2. **Run the OAuth example**:
   ```bash
   ./nklhd --config examples/oauth/config.toml --verbose
   ```

3. **Explore the mounted filesystem**:
   ```bash
   ls /tmp/nklhd-oauth/
   cat /tmp/nklhd-oauth/oauth
   cat /tmp/nklhd-oauth/oauth/client
   cat /tmp/nklhd-oauth/oauth/authcode
   ```

## Available Routes

| Route | Method | Description |
|-------|--------|-------------|
| `/oauth` | GET | Documentation and route list |
| `/oauth/client` | GET | Example OAuth client configuration |
| `/oauth/device` | GET | Simulated device flow start |
| `/oauth/device/poll` | POST | Simulated device flow polling (requires JSON body with `device_code`) |
| `/oauth/authcode` | GET | Generate authorization code URL |
| `/oauth/exchange` | POST | Simulate authorization code exchange (requires JSON body with `code`) |
| `/oauth/attach` | GET | Demonstrate attaching token to HTTP client |

**Note:** These routes demonstrate API usage with simulated responses. For real OAuth flows, you need actual OAuth provider endpoints and credentials.

## OAuth Module API Reference

### `oauth.client(config)`

Creates an OAuth client from a configuration table.

**Parameters:**
- `config` (table): Configuration with the following fields:
  - `client_id` (string): OAuth client identifier (required)
  - `token_url` (string): Token endpoint URL (required)
  - `device_url` (string): Device authorization endpoint URL (optional, for device flow)
  - `timeout_ms` (number): HTTP timeout in milliseconds (default: 5000)
  - `tls` (table): TLS configuration with `insecure_skip_verify` boolean

**Returns:** `oauth_client` userdata object

**Example:**
```lua
local config = {
    client_id = "your-client-id",
    token_url = "https://provider.example.com/oauth/token",
    device_url = "https://provider.example.com/oauth/device/code",
    timeout_ms = 10000,
    tls = { insecure_skip_verify = false }
}
local client = oauth.client(config)
```

### oauth_client Methods

#### `client:device_flow_start(scope)`

Initiates the OAuth 2.0 Device Authorization Flow.

**Parameters:**
- `scope` (string): Space-separated list of scopes (optional)

**Returns:** `(result, error)` where `result` is a table with device flow response or `nil` on error

**Result table fields:**
- `device_code` (string): Device verification code
- `user_code` (string): User-friendly code
- `verification_uri` (string): URL for user to visit
- `verification_uri_complete` (string): URL with user code pre-filled
- `expires_in` (number): Seconds until codes expire
- `interval` (number): Polling interval in seconds

**Example:**
```lua
local result, err = client:device_flow_start("read write profile")
if err then
    print("Error:", err)
else
    print("Visit:", result.verification_uri)
    print("Enter code:", result.user_code)
    -- Store device_code for polling
    local deviceCode = result.device_code
end
```

#### `client:device_poll(deviceCode, interval, maxAttempts)`

Polls the token endpoint for device flow completion.

**Parameters:**
- `deviceCode` (string): Device code from `device_flow_start()`
- `interval` (number): Seconds between polling attempts (default: 5)
- `maxAttempts` (number): Maximum number of polling attempts (default: 12)

**Returns:** `(result, error)` where `result` is a token response table or `nil` on error

**Token response fields:**
- `access_token` (string): Access token for API calls
- `token_type` (string): Usually "bearer"
- `expires_in` (number): Seconds until token expires
- `refresh_token` (string): Refresh token (if provided)
- `scope` (string): Granted scopes

**Example:**
```lua
local token, err = client:device_poll(deviceCode, 5, 12)
if err then
    print("Polling error:", err)
else
    print("Access token:", token.access_token)
    -- Store token securely
end
```

#### `client:auth_code_url(authEndpoint, scope, state)`

Constructs the authorization code URL for the user to visit.

**Parameters:**
- `authEndpoint` (string): Authorization endpoint URL
- `scope` (string): Space-separated list of scopes (optional)
- `state` (string): CSRF protection state parameter (optional)

**Returns:** `(url, error)` where `url` is the full authorization URL or `nil` on error

**Example:**
```lua
local url, err = client:auth_code_url(
    "https://provider.example.com/oauth/authorize",
    "read write",
    "random_state_123"
)
if err then
    print("Error:", err)
else
    print("Redirect user to:", url)
    -- In a web app, redirect the user to this URL
end
```

#### `client:exchange_code(code, redirectURI, clientSecret)`

Exchanges an authorization code for an access token.

**Parameters:**
- `code` (string): Authorization code from redirect
- `redirectURI` (string): Redirect URI used in authorization request
- `clientSecret` (string): Client secret (optional, for confidential clients)

**Returns:** `(result, error)` where `result` is a token response table or `nil` on error

**Example:**
```lua
local token, err = client:exchange_code(
    "AUTHORIZATION_CODE_123",
    "https://yourapp.example.com/callback",
    "your-client-secret"
)
if err then
    print("Exchange error:", err)
else
    print("Access token:", token.access_token)
    -- Store token securely
end
```

#### `client:attach_to(httpClient, token)`

Attaches an OAuth token to an HTTP client for authenticated requests.

**Parameters:**
- `httpClient` (userdata): HTTP client from `http.client()`
- `token` (string): Access token to attach

**Returns:** `(success, error)` where `success` is boolean or `nil` on error

**Note:** This is a placeholder implementation. In production, consider using `oauth2.Transport` or manually setting the `Authorization` header.

**Example:**
```lua
local http = require("http")
local httpClient = http.client("https://api.example.com")
local success, err = client:attach_to(httpClient, "ACCESS_TOKEN_123")
if not success then
    print("Attachment failed:", err)
end
```

## Token Persistence Callback

You can set a token persistence callback on the OAuth client to automatically store tokens when they are obtained:

```lua
local config = {
    client_id = "your-client-id",
    token_url = "https://provider.example.com/oauth/token"
}
local client = oauth.client(config)

-- This function would be called after successful token exchange
-- Implement it to store tokens in a database or secure storage
client.token_persist_func = function(tokenData)
    print("Token obtained:", tokenData.access_token)
    -- Store tokenData somewhere secure
end
```

## OAuth Flows in Detail

### Device Authorization Flow

The Device Authorization Flow is designed for devices that lack a browser or have limited input capabilities (e.g., smart TVs, gaming consoles, CLI tools).

**Typical workflow:**
1. Call `device_flow_start()` to get device and user codes
2. Display the `verification_uri` and `user_code` to the user
3. User visits the URI on another device (phone/computer) and enters the code
4. Poll with `device_poll()` until user completes authorization
5. Receive access token for API calls

**Example implementation:**
```lua
local client = oauth.client(config)
local result, err = client:device_flow_start("read write")
if err then error(err) end

print("Please visit: " .. result.verification_uri)
print("And enter code: " .. result.user_code)

-- Poll for token
local token, err = client:device_poll(result.device_code, 5, 12)
if err then error(err) end

print("Access token obtained:", token.access_token)
```

### Authorization Code Flow

The Authorization Code Flow is the standard flow for web applications.

**Typical workflow:**
1. Generate authorization URL with `auth_code_url()`
2. Redirect user to this URL
3. User authenticates and authorizes the application
4. User is redirected back to your `redirect_uri` with a `code` parameter
5. Exchange the code for a token with `exchange_code()`

**Example implementation:**
```lua
local client = oauth.client(config)

-- Generate authorization URL
local state = "random_csrf_token"
local url, err = client:auth_code_url(
    "https://provider.example.com/oauth/authorize",
    "read write profile",
    state
)
if err then error(err) end

-- In a web framework, redirect the user to 'url'

-- Later, when you receive the callback with 'code':
local token, err = client:exchange_code(
    code,  -- from query parameter
    "https://yourapp.example.com/callback",
    "your-client-secret"  -- if confidential client
)
if err then error(err) end

print("Access token obtained:", token.access_token)
```

## Integration with HTTP Module

Once you have an access token, you can use it with the HTTP module to make authenticated API requests:

```lua
local http = require("http")

-- Create HTTP client for API
local apiClient = http.client("https://api.example.com")

-- Set Authorization header manually (recommended approach)
local result, err = apiClient:get("/user/profile", {
    headers = {
        ["Authorization"] = "Bearer " .. token.access_token
    }
})

-- Or use the attach_to method (placeholder)
local success, err = oauthClient:attach_to(apiClient, token.access_token)
if success then
    local result, err = apiClient:get("/user/profile")
    -- ...
end
```

## Real Provider Examples

### GitHub OAuth
```lua
local config = {
    client_id = "YOUR_GITHUB_CLIENT_ID",
    token_url = "https://github.com/login/oauth/access_token",
    device_url = "https://github.com/login/device/code"  -- GitHub supports device flow
}
```

### Google OAuth
```lua
local config = {
    client_id = "YOUR_GOOGLE_CLIENT_ID",
    token_url = "https://oauth2.googleapis.com/token",
    -- Google uses different endpoints for device flow
}
```

### Custom OAuth 2.0 Provider
```lua
local config = {
    client_id = "your-client-id",
    token_url = "https://auth.example.com/oauth/token",
    device_url = "https://auth.example.com/oauth/device/code",
    timeout_ms = 10000,
    tls = { insecure_skip_verify = false }  -- Set to true for self-signed certs in dev
}
```

## Error Handling

All OAuth methods return `(nil, error_message)` on error:

```lua
local result, err = client:device_flow_start("scope")
if err then
    print("OAuth error:", err)
    -- err might be: "device flow start error: Post \"https://...\": dial tcp: ..."
    return
end
```

Common errors:
- Network errors (connection refused, timeout, DNS failure)
- OAuth protocol errors (`invalid_client`, `invalid_grant`, etc.)
- Invalid URLs or malformed requests
- TLS certificate errors

## Configuration File

The example uses `config.toml`:

```toml
# OAuth module example configuration
# Demonstrates OAuth 2.0 client functionality for Lua scripts

# Mount point for the FUSE filesystem
mountpoint = "/tmp/nklhd-oauth"

# Lua script that defines routes (relative to this config file directory)
rootscript = "main.lua"

# Enable verbose debug logging
verbose = false

# Optional: maximum concurrent handlers
# max_handlers = 4
```

## Security Considerations

1. **Client Secrets**: Never hardcode client secrets in Lua scripts. Use environment variables or secure configuration management.
2. **Token Storage**: Store tokens securely (encrypted database, secure key store). The token persistence callback helps automate this.
3. **TLS Verification**: Always verify TLS certificates in production. Use `insecure_skip_verify = true` only for development/testing.
4. **State Parameter**: Always use a random `state` parameter in authorization code flow to prevent CSRF attacks.
5. **Token Expiration**: Handle token expiration and refresh tokens appropriately.

## Testing the Example

Test the example routes using standard filesystem operations:

```bash
# View documentation
cat /tmp/nklhd-oauth/oauth

# See example client configuration
cat /tmp/nklhd-oauth/oauth/client

# Generate an authorization URL
cat /tmp/nklhd-oauth/oauth/authcode

# Simulate device flow (returns example response)
cat /tmp/nklhd-oauth/oauth/device

# Test token attachment demonstration
cat /tmp/nklhd-oauth/oauth/attach
```

**Note:** These are simulated responses. For real OAuth flows, you need to replace the placeholder endpoints and credentials with actual OAuth provider configuration.