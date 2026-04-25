-- Comprehensive test route definitions for all FUSE methods
-- This file contains test handlers for all FUSE operations

-- Test handlers for all FUSE operations

-- READ operation tests
app.read("/test/version", function(req)
    return "FUSE"
end)

app.read("/test/read", function(req)
    local offset = req.offset or 0
    local data = "This is test read data. "
    if offset >= #data then
        return ""
    end
    return string.sub(data, offset + 1)
end)

app.read("/test/directory", function(req)
    return "api_users\napi_items\nhealth\nlog\ntest"
end)

app.read("/test/getattr", function(req)
    return "File size: 1024 bytes"
end)

app.read("/test/parameters/{id}/{category}", function(req)
    return "ID: " .. req.id .. ", Category: " .. req.category
end)

-- WRITE operation tests
app.write("/test/write", function(req)
    return "Wrote " .. (#req.data or 0) .. " bytes"
end)

app.write("/test/write/echo", function(req)
    return req.data or ""
end)

-- CREATE operation tests (FUSE Create)
app.create("/test/create", function(req)
    return "Created file: " .. (req.name or "unknown")
end)

app.create("/test/create/{name}", function(req)
    return "Created file with name: " .. req.name
end)

-- UNLINK operation tests (FUSE Unlink)
app.unlink("/test/unlink", function(req)
    return "Unlinked file: " .. (req.name or "unknown")
end)

app.unlink("/test/unlink/{name}", function(req)
    return "Unlinked file: " .. req.name
end)

-- Additional operations for completeness

-- LOOKUP operation simulation (handled as read)
app.read("/test/lookup/{path}", function(req)
    return "Looking up: " .. req.path
end)

-- GETATTR operation simulation (already covered)

-- RELEASE operation simulation (no server-side handler, but we can test via read)
app.read("/test/release", function(req)
    return "File released successfully"
end)

-- FLUSH operation simulation
app.read("/test/flush", function(req)
    return "Flush completed"
end)

-- SETATTR operation simulation (write stat)
app.write("/test/setattr", function(req)
    return "Attributes updated"
end)

-- Error handling test
app.read("/test/error", function(req)
    error("Simulated Lua error")
end)

-- Large data handling
app.read("/test/large", function(req)
    local data = ""
    for i = 1, 100 do
        data = data .. "Line " .. i .. " of test data\n"
    end
    return data
end)

-- Offset handling
app.read("/test/offset", function(req)
    local offset = tonumber(req.offset) or 0
    local data = "0123456789"
    if offset >= #data then
        return ""
    end
    return string.sub(data, offset + 1)
end)

-- Counter for round-robin testing (shared across all handlers)
counter = 0
app.read("/test/counter", function(req)
    counter = counter + 1
    return "Counter: " .. counter
end)

app.write("/test/counter", function(req)
    counter = tonumber(req.data) or 0
    return "Counter set to: " .. counter
end)

-- Authentication simulation
app.read("/test/auth", function(req)
    return "Authentication not required"
end)

-- Directory listing handlers for FUSE testing
app.readdir("/", function(req)
    return {"test", "api", "data", "logs"}
end)

app.readdir("/test", function(req)
    return {"version", "read", "write", "create", "unlink", "directory", "getattr", "parameters", "error", "large", "offset", "counter", "auth"}
end)

app.readdir("/api/users", function(req)
    return {"{id}"}  -- parameter placeholder
end)