-- nklhd utilities example
-- Demonstrates the new app.env, app.log, and app.debug functionality

print("nklhd utilities example starting")

-- Access environment variables via app.env
app.log("Current user: " .. (app.env.USER or "unknown"))
app.log("Home directory: " .. (app.env.HOME or "unknown"))
app.log("PATH length: " .. tostring(#(app.env.PATH or "")))

-- Debug messages only appear when debug logging is enabled
app.debug("This debug message will only appear if debug logging is enabled")
app.log("This log message always appears")

-- Use environment variables to configure routes
local max_items = tonumber(app.env.MAX_ITEMS) or 10
local default_greeting = app.env.GREETING or "Hello"

-- Register a route that uses environment variables
app.read("/info", function(req)
    local info = {}
    table.insert(info, "User: " .. (app.env.USER or "unknown"))
    table.insert(info, "Home: " .. (app.env.HOME or "unknown"))
    table.insert(info, "Max items: " .. max_items)
    table.insert(info, "Default greeting: " .. default_greeting)
    table.insert(info, "")
    table.insert(info, "All environment variables:")
    for k, v in pairs(app.env) do
        table.insert(info, k .. "=" .. v)
    end
    return table.concat(info, "\n")
end)

-- A counter that logs each access
local counter = 0
app.read("/counter", function(req)
    counter = counter + 1
    app.log("Counter accessed: " .. counter)
    app.debug("Counter debug: " .. counter)
    return "Counter: " .. counter
end)

-- A route that shows how to conditionally log based on environment
app.read("/debug-test", function(req)
    if app.env.DEBUG_MODE == "1" then
        app.debug("Debug mode is enabled")
        return "Debug mode is ON"
    else
        return "Debug mode is OFF (set DEBUG_MODE=1 to enable)"
    end
end)

app.log("Utilities example routes registered")
print("Utilities example ready")