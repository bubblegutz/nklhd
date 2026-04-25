-- Comprehensive FUSE test configuration
-- Tests all pattern types and all FUSE operations

-- Global counter for testing state persistence
global_counter = 0
shared_state = {}

-- ============================================
-- EXACT PATH TESTS
-- ============================================

-- Basic read operations
app.read("/version", function(req)
    return "nklhd-1.0"
end)

app.read("/status", function(req)
    return "OK"
end)

app.read("/readme", function(req)
    return "This is a test file with exact path matching."
end)

-- Write operations
app.write("/config", function(req)
    local data = req.data or ""
    return "Config updated: " .. #data .. " bytes"
end)

app.write("/log", function(req)
    local data = req.data or ""
    -- Simulate logging
    if not shared_state.logs then
        shared_state.logs = {}
    end
    table.insert(shared_state.logs, data)
    return "Logged: " .. #shared_state.logs .. " entries"
end)

-- Create operations (FUSE create)
app.create("/newfile", function(req)
    return "File created: " .. (req.name or "unknown")
end)

-- Remove operations (FUSE unlink)
app.unlink("/unlink", function(req)
    return "File unlinked: " .. (req.name or "unknown")
end)

-- Stat operations (FUSE getattr)
app.getattr("/getattr", function(req)
    -- Return stat-like information
    return "size: 1024\nmode: 0644\nuid: 1000\ngid: 1000"
end)

-- Open operations (FUSE open)
app.open("/openfile", function(req)
    return "File opened: flags=" .. (req.flags or "0")
end)

-- Lookup operations (FUSE lookup)
app.lookup("/path/to/file", function(req)
    return "Looking up: " .. req.path
end)

-- ============================================
-- PARAMETER PATH TESTS: {param}
-- ============================================

-- Single parameter
app.read("/users/{id}", function(req)
    return "User ID: " .. req.id
end)

app.write("/users/{id}", function(req)
    return "Updated user " .. req.id .. " with data: " .. (req.data or "")
end)

-- Multiple parameters
app.read("/users/{id}/posts/{post_id}", function(req)
    return "User " .. req.id .. ", Post " .. req.post_id
end)

app.create("/projects/{project}/files/{filename}", function(req)
    return "Created file " .. req.filename .. " in project " .. req.project
end)

-- Parameter with glob suffix
app.read("/download/{file}.*", function(req)
    return "Downloading " .. req.file .. " with extension"
end)

-- Nested parameters
app.read("/api/{version}/resources/{resource}/{id}", function(req)
    return "API " .. req.version .. " Resource " .. req.resource .. " ID " .. req.id
end)

-- ============================================
-- WILDCARD PATH TESTS: *
-- ============================================

-- Terminal wildcard (matches any remaining path)
app.read("/api/data/*", function(req)
    return "Wildcard path: " .. req.path .. " captured: " .. (req["*"] or "")
end)

-- Single component wildcard
app.read("/logs/*/details", function(req)
    return "Log category: " .. (req["*"] or "unknown") .. " details"
end)

-- Multiple wildcards
app.read("/a/*/b/*/c", function(req)
    return "Wildcards: " .. (req["*1"] or "") .. " and " .. (req["*2"] or "")
end)

-- Wildcard with parameter
app.read("/users/{id}/files/*", function(req)
    return "User " .. req.id .. " file: " .. (req["*"] or "")
end)

-- ============================================
-- GLOB PATTERN TESTS
-- ============================================

-- Prefix glob: prefix*
app.read("/files/data*", function(req)
    return "Data file: " .. req.path
end)

-- Suffix glob: *suffix
app.read("/logs/*.txt", function(req)
    return "Text log: " .. req.path
end)

-- Contains glob: *middle*
app.read("/tmp/*temp*", function(req)
    return "Temp file: " .. req.path
end)

-- Fixed glob: prefix*suffix
app.read("/docs/*_v*.txt", function(req)
    return "Versioned doc: " .. req.path
end)

-- Complex glob with parameters
app.read("/api/{version}/*.json", function(req)
    return "JSON API " .. req.version .. " endpoint: " .. req.path
end)

-- ============================================
-- READDIR HANDLERS (Directory listings)
-- ============================================

app.readdir("/", function(req)
    return {
        "version",
        "status", 
        "readme",
        "config",
        "log",
        "newfile",
        "unlink",
        "getattr",
        "openfile",
        "users/",
        "api/",
        "files/",
        "logs/",
        "tmp/",
        "docs/",
        "projects/",
        "download/",
        "path/",
        "counter",
        "large",
        "error",
        "offset"
    }
end)

app.readdir("/users", function(req)
    return {"{id}", "alice", "bob", "charlie/"}
end)

app.readdir("/api", function(req)
    return {"data/", "{version}/"}
end)

app.readdir("/files", function(req)
    return {"data123", "data456", "backup_2025", "archive*"}
end)

app.readdir("/logs", function(req)
    return {"app.log", "error.txt", "debug.txt", "system/"}
end)

app.readdir("/projects", function(req)
    return {"{project}/"}
end)

-- ============================================
-- ADVANCED TESTS
-- ============================================

-- Stateful counter
app.read("/counter", function(req)
    global_counter = global_counter + 1
    return "Counter: " .. global_counter
end)

app.write("/counter", function(req)
    local val = tonumber(req.data)
    if val then
        global_counter = val
        return "Counter set to: " .. global_counter
    else
        return "Invalid counter value"
    end
end)

-- Large data handling
app.read("/large", function(req)
    local result = ""
    for i = 1, 1000 do
        result = result .. "Line " .. i .. ": " .. string.rep("x", 50) .. "\n"
    end
    return result
end)

-- Offset handling
app.read("/offset", function(req)
    local data = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
    local offset = tonumber(req.offset) or 0
    if offset >= #data then
        return ""
    end
    return string.sub(data, offset + 1, math.min(offset + 10, #data))
end)

-- Error handling
app.read("/error", function(req)
    error("Simulated Lua error for testing")
end)

-- Data transformation
app.write("/transform", function(req)
    local data = req.data or ""
    return "UPPER: " .. string.upper(data) .. " LOWER: " .. string.lower(data)
end)

-- Conditional responses
app.read("/check/{value}", function(req)
    local val = req.value
    if val == "ok" then
        return "OK"
    elseif val == "error" then
        error("Requested error")
    else
        return "Value: " .. val
    end
end)

-- Complex write with validation
app.write("/validate", function(req)
    local data = req.data or ""
    if #data < 5 then
        return "ERROR: Data too short (min 5 bytes)"
    end
    if #data > 100 then
        return "ERROR: Data too long (max 100 bytes)"
    end
    return "VALID: " .. #data .. " bytes"
end)

-- Create with metadata
app.create("/metadata", function(req)
    return "Created with mode: " .. (req.mode or "0644") .. " uid: " .. (req.uid or "1000")
end)

-- Remove with confirmation
app.unlink("/confirm", function(req)
    local confirm = req.confirm or "no"
    if confirm == "yes" then
        return "File unlinked"
    else
        return "Cancelled: confirmation required"
    end
end)

-- Stat with dynamic info
app.getattr("/dynamic", function(req)
    local size = #(req.path or "")
    return "size: " .. size .. "\nmode: 0755\nuid: 0\ngid: 0"
end)

-- Open with flags
app.open("/flags", function(req)
    local flags = tonumber(req.flags) or 0
    local mode = "r"
    if flags % 2 == 1 then
        mode = "w"
    elseif flags % 4 == 2 then
        mode = "rw"
    end
    return "Opened with mode: " .. mode .. " (flags: " .. flags .. ")"
end)

-- Lookup with validation
app.lookup("/validate/{path}", function(req)
    local path = req.path or ""
    if string.find(path, "..") then
        error("Invalid path: contains '..'")
    end
    return "Valid path: " .. path
end)

-- ============================================
-- CONCURRENCY AND STATE TESTS
-- ============================================

-- Shared counter with atomic increment
app.read("/atomic", function(req)
    if not shared_state.atomic then
        shared_state.atomic = 0
    end
    shared_state.atomic = shared_state.atomic + 1
    return "Atomic value: " .. shared_state.atomic
end)

-- Shared dictionary
app.write("/dict/{key}", function(req)
    local key = req.key
    local value = req.data or ""
    if not shared_state.dict then
        shared_state.dict = {}
    end
    shared_state.dict[key] = value
    return "Set " .. key .. " = " .. value
end)

app.read("/dict/{key}", function(req)
    if not shared_state.dict then
        return "Key not found"
    end
    local value = shared_state.dict[req.key]
    if value then
        return value
    else
        return "Key not found: " .. req.key
    end
end)

-- Queue operations
app.write("/queue", function(req)
    local data = req.data or ""
    if not shared_state.queue then
        shared_state.queue = {}
    end
    table.insert(shared_state.queue, data)
    return "Enqueued: " .. #shared_state.queue .. " items"
end)

app.read("/queue", function(req)
    if not shared_state.queue or #shared_state.queue == 0 then
        return "Queue empty"
    end
    local item = table.remove(shared_state.queue, 1)
    return "Dequeued: " .. item .. " (" .. #shared_state.queue .. " remaining)"
end)

-- ============================================
-- PERFORMANCE AND STRESS TESTS
-- ============================================

-- Echo server (returns what was sent)
app.write("/echo", function(req)
    return req.data or ""
end)

app.read("/echo", function(req)
    return "Send data via write to echo"
end)

-- Memory test
app.read("/memory", function(req)
    local size = tonumber(req.size) or 100
    if size > 10000 then
        size = 10000
    end
    return string.rep("X", size)
end)

-- Compute-intensive operation
app.read("/compute/{n}", function(req)
    local n = tonumber(req.n) or 10
    local sum = 0
    for i = 1, n do
        sum = sum + i
    end
    return "Sum 1.." .. n .. " = " .. sum
end)

-- ============================================
-- EDGE CASES
-- ============================================

-- Empty response
app.read("/empty", function(req)
    return ""
end)

-- Nil response (should be treated as empty)
app.read("/nil", function(req)
    return nil
end)

-- Very long path
app.read("/" .. string.rep("a", 50) .. "/" .. string.rep("b", 50), function(req)
    return "Long path handled"
end)

-- Special characters in parameters
app.read("/special/{param}", function(req)
    return "Param: " .. req.param .. " (length: " .. #req.param .. ")"
end)

-- Unicode test
app.read("/unicode/{text}", function(req)
    return "Text: " .. req.text .. " (bytes: " .. #req.text .. ")"
end)

-- Binary data handling (via write/read)
app.write("/binary", function(req)
    local data = req.data or ""
    -- Store binary data
    shared_state.binary = data
    return "Binary data stored: " .. #data .. " bytes"
end)

app.read("/binary", function(req)
    return shared_state.binary or ""
end)

-- ============================================
-- COMPOSITE OPERATIONS
-- ============================================

-- Read after write
app.write("/store", function(req)
    shared_state.stored = req.data or ""
    return "Stored: " .. #shared_state.stored .. " bytes"
end)

app.read("/store", function(req)
    return shared_state.stored or ""
end)

-- Increment via write+read
app.write("/increment", function(req)
    local val = tonumber(req.data) or 0
    if not shared_state.inc then
        shared_state.inc = 0
    end
    shared_state.inc = shared_state.inc + val
    return "Incremented by " .. val .. ", total: " .. shared_state.inc
end)

app.read("/increment", function(req)
    return tostring(shared_state.inc or 0)
end)