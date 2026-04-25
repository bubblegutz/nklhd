-- Serialization example demonstrating json and yaml modules

app.log("Loading serialization example")

-- Route that returns a JSON object
app.read("/api/json/hello", function(req)
    local data = {
        message = "Hello from nklhd",
        timestamp = os.time(),
        version = "1.0"
    }
    local encoded, err = json.encode(data)
    if err then
        return "Error encoding JSON: " .. err
    end
    return encoded
end)

-- Conversion routes: JSON ↔ YAML
-- Route that accepts JSON and returns YAML
app.write("/api/convert/json-to-yaml", function(req)
    local json_data = req.data
    if not json_data or json_data == "" then
        return "No JSON data provided"
    end
    
    local decoded, err = json.decode(json_data)
    if err then
        return "Invalid JSON: " .. err
    end
    
    local yaml_output, err = yaml.encode(decoded)
    if err then
        return "Error encoding YAML: " .. err
    end
    
    return yaml_output
end)

-- Route that accepts YAML and returns JSON
app.write("/api/convert/yaml-to-json", function(req)
    local yaml_data = req.data
    if not yaml_data or yaml_data == "" then
        return "No YAML data provided"
    end
    
    local decoded, err = yaml.decode(yaml_data)
    if err then
        return "Invalid YAML: " .. err
    end
    
    local json_output, err = json.encode(decoded)
    if err then
        return "Error encoding JSON: " .. err
    end
    
    return json_output
end)

-- Route that returns a YAML list of environment variables
app.read("/api/env", function(req)
    local env_list = {}
    for key, value in pairs(app.env) do
        table.insert(env_list, { key = key, value = value })
    end
    -- Sort by key
    table.sort(env_list, function(a, b) return a.key < b.key end)
    
    local encoded, err = yaml.encode(env_list)
    if err then
        return "Error encoding YAML: " .. err
    end
    return encoded
end)

-- Route that demonstrates nested data structures
app.read("/api/data/complex", function(req)
    local complex = {
        users = {
            { name = "Alice", age = 30, tags = {"admin", "user"} },
            { name = "Bob", age = 25, tags = {"user"} },
            { name = "Charlie", age = 35, tags = {"admin", "moderator"} }
        },
        metadata = {
            total = 3,
            generated = os.date("%Y-%m-%d %H:%M:%S"),
            format = "JSON"
        }
    }
    
    local encoded, err = json.encode(complex)
    if err then
        return "Error encoding JSON: " .. err
    end
    return encoded
end)

-- Route that echoes back JSON or YAML based on Accept header (simulated)
app.read("/api/echo/{format}", function(req)
    local format = req.format or "json"
    local data = {
        original = req.data or "",
        length = string.len(req.data or ""),
        format = format,
        timestamp = os.time()
    }
    
    if format == "yaml" then
        local encoded, err = yaml.encode(data)
        if err then
            return "Error encoding YAML: " .. err
        end
        return encoded
    else
        local encoded, err = json.encode(data)
        if err then
            return "Error encoding JSON: " .. err
        end
        return encoded
    end
end)

app.log("Serialization example loaded successfully")