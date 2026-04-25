-- Simple nklhd example: Basic route definitions
-- This file demonstrates the core routing API using the global 'app' object

-- Root directory listing
app.readdir("/", function(req)
    return {"hello.txt", "counter.txt", "echo", "files/", "uploads/"}
end)

-- Simple text file
app.read("/hello.txt", function(req)
    return "Hello from nklhd FUSE filesystem!\n" ..
           "This file is served by a Lua handler."
end)

-- Counter that increments on each read
local counter = 0
app.read("/counter.txt", function(req)
    counter = counter + 1
    return "Counter: " .. counter .. "\n" ..
           "Read " .. counter .. " time" .. (counter == 1 and "" or "s")
end)

-- Echo handler that returns input data
app.write("/echo", function(req)
    local data = req.data or ""
    return "Echo: " .. data
end)

-- Directory with nested files
app.readdir("/files", function(req)
    return {"readme.txt", "nested/"}
end)

app.read("/files/readme.txt", function(req)
    return "This is a nested file.\n" ..
           "You can create complex directory structures with Lua handlers."
end)

app.readdir("/files/nested", function(req)
    return {"deep.txt"}
end)

app.read("/files/nested/deep.txt", function(req)
    return "Deeply nested file content."
end)

-- File statistics (FUSE getattr operation)
app.getattr("/hello.txt", function(req)
    return {
        size = 68,  -- length of "Hello from nklhd..." string
        mode = 420,  -- 0644 in octal (rw-r--r--)
        mtime = os.time()
    }
end)

-- Lookup operation (for directory traversal)
app.lookup("/files", function(req)
    return {
        {name = "readme.txt", is_dir = false},
        {name = "nested", is_dir = true}
    }
end)

-- Create operation (FUSE create)
app.create("/newfile", function(req)
    return "Created file: " .. (req.name or "unknown")
end)

-- Unlink operation (FUSE unlink/remove)
app.unlink("/deletefile", function(req)
    return "Deleted file: " .. (req.name or "unknown")
end)

-- Open operation (FUSE open)
app.open("/hello.txt", function(req)
    return "File opened with flags: " .. (req.flags or "0")
end)

-- Uploads directory listing
app.readdir("/uploads", function(req)
    return {}  -- Empty directory initially
end)

-- Wildcard create operation (FUSE create with pattern matching)
app.create("/uploads/*", function(req)
    local filename = req["*"] or "unknown"
    return "Created upload file: " .. filename .. "\n" ..
           "Path: /uploads/" .. filename .. "\n" ..
           "Time: " .. os.date("%Y-%m-%d %H:%M:%S")
end)