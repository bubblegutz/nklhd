-- JSON/YAML conversion example for nklhd.
-- Routes:
--   /tojson   - Write YAML, read JSON (converts YAML to JSON)
--   /toyaml   - Write JSON, read YAML (converts JSON to YAML)
--   /detect   - Write either, read same format (auto-detection)

app.write("/tojson", function(req)
    local data = req.data or ""
    -- Assume input is YAML, convert to JSON
    local jsonStr, err = json.fromYAML(data)
    if err then
        return "error: " .. err
    end
    -- Store converted JSON for later reading
    app._conversion_cache = jsonStr
    app.log("Cache set to: " .. jsonStr)
    return jsonStr
end)

app.read("/tojson", function(req)
    app.log("Cache read: " .. tostring(app._conversion_cache))
    return app._conversion_cache or "{}"
end)

app.write("/toyaml", function(req)
    local data = req.data or ""
    -- Assume input is JSON, convert to YAML
    local yamlStr, err = yaml.fromJSON(data)
    if err then
        return "error: " .. err
    end
    app._conversion_cache = yamlStr
    return yamlStr
end)

app.read("/toyaml", function(req)
    return app._conversion_cache or ""
end)

-- Auto-detection route: write either JSON or YAML, read back the opposite format.
-- Detection heuristic: if first non-whitespace char is '{' or '[', treat as JSON, else YAML.
app.write("/convert", function(req)
    local data = req.data or ""
    -- Trim leading whitespace
    local trimmed = data:gsub("^%s+", ""):gsub("%s+$", "")
    if trimmed == "" then
        app._convert_cache = ""
        app._convert_mode = nil
        return ""
    end
    local first = trimmed:sub(1,1)
    if first == '{' or first == '[' then
        -- Likely JSON, convert to YAML
        local yamlStr, err = yaml.fromJSON(data)
        if err then
            return "error (JSON->YAML): " .. err
        end
        app._convert_cache = yamlStr
        app._convert_mode = "yaml"
    else
        -- Likely YAML, convert to JSON
        local jsonStr, err = json.fromYAML(data)
        if err then
            return "error (YAML->JSON): " .. err
        end
        app._convert_cache = jsonStr
        app._convert_mode = "json"
    end
    return app._convert_cache
end)

app.read("/convert", function(req)
    if not app._convert_cache then
        return ""
    end
    return app._convert_cache
end)

-- Helper route to show usage
app.read("/help", function(req)
    return [[
JSON/YAML Conversion Example

Routes:
  /tojson    - Write YAML, read JSON
  /toyaml    - Write JSON, read YAML
  /convert   - Write either, read opposite format (auto-detection)
  /help      - This message

Examples:
  echo 'name: test' > /mount/tojson    # Write YAML
  cat /mount/tojson                    # Read JSON

  echo '{"x":1}' > /mount/toyaml
  cat /mount/toyaml

  echo 'key: value' > /mount/convert
  cat /mount/convert                   # Outputs JSON

  echo '{"a":1}' > /mount/convert
  cat /mount/convert                   # Outputs YAML
]]
end)