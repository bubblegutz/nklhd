# Simple nklhd Example

This example demonstrates modular nklhd application with multiple Lua files organized using `require()`.

## Files

- `config.toml` - Configuration with verbose logging enabled
- `main.lua` - Main script that sets up `package.path` and loads modules
- `utils.lua` - Utility module with helper functions
- `routes.lua` - Route definitions organized by functionality
- `config.lua` - Configuration module with custom routes

## Features Demonstrated

- Modular code organization with Lua's `require()` system
- Dynamic route registration from module data structures
- User management API with CRUD operations
- Product catalog with inventory management
- System metrics and health checks
- Configuration management module
- Custom route definitions in configuration
- File and directory operations with metadata

## Module Architecture

```
main.lua (entry point)
├── utils.lua (helper functions)
├── routes.lua (route definitions)
└── config.lua (configuration)
```

### main.lua
- Sets up `package.path` to include script directory
- Loads all modules using `require()`
- Registers routes dynamically from module data
- Defines root directory and documentation routes

### utils.lua
- Provides utility functions: memory usage, uptime, formatting, validation
- Logging with timestamps
- Table serialization for debugging
- Input validation and sanitization

### routes.lua
- Defines route handlers organized by domain
- User management (list, get, create, delete)
- Product catalog operations
- System health checks and metrics
- File and directory operations

### config.lua
- Configuration settings management
- Custom route definitions
- Feature flags
- Configuration validation

## API Endpoints

### User Management
- `GET /api/users` - List all users
- `GET /api/users/{id}` - Get user details
- `POST /api/users` - Create new user
- `DELETE /api/users/{id}` - Delete user

### Product Catalog
- `GET /api/products` - List all products
- `GET /api/products/{id}` - Get product details
- `POST /api/products` - Create new product

### System
- `GET /api/health` - Health check with system status
- `GET /api/metrics` - List available metrics
- `GET /api/metrics/{name}` - Get specific metric

### Documentation
- `GET /` - Root directory listing
- `GET /docs/api.md` - API documentation
- `GET /status.txt` - System status
- `GET /config.txt` - Configuration info

## Running the Example

```bash
# Build nklhd (if not already built)
go build

# Run with the example config
./nklhd --config examples/simple/config.toml

# With custom mount point
./nklhd --config examples/simple/config.toml --mount /tmp/my-simple-nklhd

# With verbose logging (already enabled in config)
./nklhd --config examples/simple/config.toml --verbose
```

## Exploring the Filesystem

```bash
# List root directory
ls /tmp/nklhd-simple/

# Read documentation
cat /tmp/nklhd-simple/docs/api.md

# Check system status
cat /tmp/nklhd-simple/status.txt

# Explore API
ls /tmp/nklhd-simple/api/
cat /tmp/nklhd-simple/api/health

# User management
cat /tmp/nklhd-simple/api/users
cat /tmp/nklhd-simple/api/users/1

# Product catalog
cat /tmp/nklhd-simple/api/products
```

## Module Loading Details

### package.path Configuration
The main script configures `package.path` to include its directory:
```lua
local script_dir = debug.getinfo(1, "S").source:match("@?(.*/)") or "./"
package.path = script_dir .. "?.lua;" .. package.path
```

This enables relative imports:
```lua
local utils = require("utils")  -- loads utils.lua from same directory
```

### Dynamic Route Registration
Routes are defined as data structures in modules and registered dynamically:
```lua
-- In routes.lua
function routes.get_routes()
    return {
        {method = "read", path = "/api/users", handler = routes.list_users},
        {method = "write", path = "/api/users", handler = routes.create_user},
        -- ...
    }
end

-- In main.lua
for _, route_def in ipairs(routes.get_routes()) do
    app[route_def.method](route_def.path, route_def.handler)
end
```

## Configuration Details

The `config.toml` file enables verbose logging:
```toml
mountpoint = "/tmp/nklhd-simple"
rootscript = "main.lua"
verbose = true
```

## Key Concepts

1. **Modular Design**: Separate concerns into different Lua modules
2. **Dynamic Configuration**: Routes and settings defined as data structures
3. **State Management**: Modules maintain their own state (users, products)
4. **Error Handling**: Input validation and error reporting
5. **Logging**: Structured logging with timestamps

## Testing the Modules

```bash
# Test individual modules
lua -e 'package.path = "?.lua;" .. package.path; utils = require("utils"); print(utils.get_uptime())'

# Test route definitions
lua -e 'package.path = "?.lua;" .. package.path; routes = require("routes"); print(#routes.get_routes())'

# Simulate the full application
lua -e '
package.path = "?.lua;" .. package.path
app = {
  read = function(p,f) print("read", p) end,
  write = function(p,f) print("write", p) end,
  list = function(p,f) print("list", p) end,
  create = function(p,f) print("create", p) end,
  remove = function(p,f) print("remove", p) end,
  stat = function(p,f) print("stat", p) end,
  walk = function(p,f) print("walk", p) end,
  open = function(p,f) print("open", p) end
}
dofile("main.lua")
'
```