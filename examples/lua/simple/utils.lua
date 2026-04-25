-- Utility module for simple nklhd example
-- Provides helper functions for route handlers

local utils = {}

-- Track start time for uptime calculation
local start_time = os.time()

-- Simple memory usage simulation
function utils.get_memory_usage()
    -- In a real implementation, you might read from /proc/meminfo or similar
    -- For this example, return a simulated value
    return math.random(50, 200) / 100  -- 0.5 to 2.0 MB
end

-- Calculate uptime in seconds
function utils.get_uptime()
    return os.time() - start_time
end

-- Format bytes to human readable string
function utils.format_bytes(bytes)
    local units = {"B", "KB", "MB", "GB", "TB"}
    local unit_index = 1
    
    while bytes >= 1024 and unit_index < #units do
        bytes = bytes / 1024
        unit_index = unit_index + 1
    end
    
    return string.format("%.2f %s", bytes, units[unit_index])
end

-- Parse query parameters from request path
function utils.parse_query(req)
    local query = {}
    if req.query then
        for pair in req.query:gmatch("[^&]+") do
            local key, value = pair:match("([^=]*)=?(.*)")
            if key then
                query[key] = value
            end
        end
    end
    return query
end

-- Validate user input (simple example)
function utils.validate_input(input, max_length)
    if not input or input:len() == 0 then
        return false, "Input cannot be empty"
    end
    if input:len() > (max_length or 1000) then
        return false, "Input too long (max " .. (max_length or 1000) .. " characters)"
    end
    -- Basic safety check
    if input:find("[<>\"']") then
        return false, "Input contains unsafe characters"
    end
    return true, nil
end

-- Generate a unique ID
function utils.generate_id()
    local template = 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'
    return template:gsub('[xy]', function(c)
        local v = (c == 'x') and math.random(0, 0xf) or math.random(8, 0xb)
        return string.format('%x', v)
    end)
end

-- Log message with timestamp
function utils.log(message)
    local timestamp = os.date("%Y-%m-%d %H:%M:%S")
    print(string.format("[%s] %s", timestamp, message))
end

-- Table serialization for debugging
function utils.table_to_string(tbl, indent)
    indent = indent or 0
    local str = ""
    local spaces = string.rep("  ", indent)
    
    if type(tbl) ~= "table" then
        return tostring(tbl)
    end
    
    str = str .. "{\n"
    for k, v in pairs(tbl) do
        local key = (type(k) == "string") and ("\"" .. k .. "\"") or tostring(k)
        local value = (type(v) == "table") and utils.table_to_string(v, indent + 1) or 
                     ((type(v) == "string") and ("\"" .. v .. "\"") or tostring(v))
        str = str .. spaces .. "  " .. key .. " = " .. value .. ",\n"
    end
    str = str .. spaces .. "}"
    return str
end

return utils