-- Simple nklhd example: Modular Lua code with require()
-- This example demonstrates organizing route definitions across multiple files

-- Set up package.path to include the directory of this script
-- This allows require() to find modules relative to the main script
local script_dir = debug.getinfo(1, "S").source:match("@?(.*/)") or "./"
package.path = script_dir .. "?.lua;" .. package.path

print("Loading simple nklhd example from: " .. script_dir)
print("Package path: " .. package.path)

-- Load utility module with helper functions
local utils = require("utils")
print("Loaded utils module")

-- Load route definitions module
local routes = require("routes")
print("Loaded routes module")

-- Load configuration module
local config = require("config")
print("Loaded config module")

-- Register routes defined in the routes module
-- The routes module returns a table with route definitions
for _, route_def in ipairs(routes.get_routes()) do
    local method = route_def.method
    local path = route_def.path
    local handler = route_def.handler
    
    if method == "read" then
        app.read(path, handler)
    elseif method == "write" then
        app.write(path, handler)
    elseif method == "readdir" then
        app.readdir(path, handler)
    elseif method == "create" then
        app.create(path, handler)
    elseif method == "unlink" then
        app.unlink(path, handler)
    elseif method == "getattr" then
        app.getattr(path, handler)
    elseif method == "lookup" then
        app.lookup(path, handler)
    elseif method == "open" then
        app.open(path, handler)
    else
        print("WARNING: Unknown method '" .. method .. "' for path '" .. path .. "'")
    end
end

-- Register additional routes defined in config module
if config.custom_routes then
    for _, route in ipairs(config.custom_routes) do
        app[route.method](route.path, route.handler)
    end
end

-- Root directory that lists available routes
app.readdir("/", function(req)
    return {"api/", "docs/", "status.txt", "config.txt"}
end)

-- API documentation
app.readdir("/docs", function(req)
    return {"api.md", "examples/"}
end)

app.read("/docs/api.md", function(req)
    return "# nklhd API Documentation\n\n" ..
           "This simple example demonstrates:\n" ..
           "- Modular code organization with require()\n" ..
           "- Utility functions in separate modules\n" ..
           "- Configuration management\n" ..
           "- Dynamic route registration\n\n" ..
           "## Available API endpoints\n" ..
           "- /api/users/ - User management\n" ..
           "- /api/products/ - Product catalog\n" ..
           "- /status.txt - System status\n" ..
           "- /config.txt - Current configuration"
end)

-- System status
app.read("/status.txt", function(req)
    local mem_usage = utils.get_memory_usage()
    local uptime = utils.get_uptime()
    
    return "System Status:\n" ..
           "Uptime: " .. uptime .. " seconds\n" ..
           "Memory usage: " .. mem_usage .. " MB\n" ..
           "Routes registered: " .. #routes.get_routes() .. "\n" ..
           "Config loaded from: " .. config.source .. "\n" ..
           "Generated: " .. os.date("%Y-%m-%d %H:%M:%S")
end)

-- Configuration info
app.read("/config.txt", function(req)
    return "Configuration:\n" ..
           "Mount point: " .. config.mount_point .. "\n" ..
           "Verbose logging: " .. tostring(config.verbose) .. "\n" ..
           "Max handlers: " .. tostring(config.max_handlers) .. "\n" ..
           "Script directory: " .. script_dir
end)

print("Advanced example loaded successfully")