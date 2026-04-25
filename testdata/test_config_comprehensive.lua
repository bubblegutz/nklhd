-- Comprehensive test route definitions exercising all FUSE methods
-- This file contains test handlers for all FUSE operations

-- Test handlers for all FUSE operations
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

app.write("/test/write", function(req)
    return "Wrote " .. (#req.data or 0) .. " bytes"
end)

app.create("/test/create", function(req)
    return "Created file: " .. (req.name or "unknown")
end)

app.unlink("/test/unlink", function(req)
    return "Unlinked file: " .. (req.name or "unknown")
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

-- Test error handling
app.read("/test/error", function(req)
    error("Simulated Lua error")
end)

-- Test large data handling
app.read("/test/large", function(req)
    local data = ""
    for i = 1, 100 do
        data = data .. "Line " .. i .. " of test data\n"
    end
    return data
end)

-- Test offset handling
app.read("/test/offset", function(req)
    local offset = tonumber(req.offset) or 0
    local data = "0123456789"
    if offset >= #data then
        return ""
    end
    return string.sub(data, offset + 1)
end)

-- Test concurrent access counter
counter = 0
app.read("/test/counter", function(req)
    counter = counter + 1
    return "Counter: " .. counter
end)

app.write("/test/counter", function(req)
    counter = tonumber(req.data) or 0
    return "Counter set to: " .. counter
end)

-- Test authentication simulation
app.read("/test/auth", function(req)
    return "Authentication not required"
end)

-- Test lookup operations
app.read("/test/lookup/{path}", function(req)
    return "Looking up: " .. req.path
end)

-- Test release operation
app.read("/test/release", function(req)
    return "File released successfully"
end)

-- Test flush operation
app.read("/test/flush", function(req)
    return "Flush completed"
end)

-- Test setattr operation
app.write("/test/setattr", function(req)
    return "Attributes updated"
end)