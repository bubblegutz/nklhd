-- Configuration module for simple nklhd example
-- Provides configuration settings and custom route definitions

local config = {}

-- Basic configuration
config.mount_point = "/tmp/nklhd-simple"
config.verbose = true
config.max_handlers = 8
config.source = "examples/simple/config.toml"

-- Custom routes that can be registered dynamically
config.custom_routes = {
    {
        method = "read",
        path = "/api/info",
        handler = function(req)
            return "Advanced nklhd Example\n" ..
                   "Version: 1.0.0\n" ..
                   "Modules: utils, routes, config\n" ..
                   "Mount point: " .. config.mount_point
        end
    },
    {
        method = "readdir",
        path = "/api/modules",
        handler = function(req)
            return {"utils", "routes", "config", "main"}
        end
    },
    {
        method = "read",
        path = "/api/modules/{name}",
        handler = function(req)
            local module_name = req.name
            if module_name == "utils" then
                return "Utility module\n" ..
                       "Provides helper functions for route handlers\n" ..
                       "Includes: memory usage, uptime, formatting, validation"
            elseif module_name == "routes" then
                return "Routes module\n" ..
                       "Defines all API route handlers\n" ..
                       "Includes: user management, product catalog, system routes"
            elseif module_name == "config" then
                return "Configuration module\n" ..
                       "Provides settings and custom routes\n" ..
                       "Current mount point: " .. config.mount_point
            elseif module_name == "main" then
                return "Main script\n" ..
                       "Orchestrates module loading and route registration\n" ..
                       "Sets up package.path for require()"
            else
                return "Unknown module: " .. module_name
            end
        end
    },
    {
        method = "write",
        path = "/api/echo",
        handler = function(req)
            local data = req.data or ""
            return "Echo (from custom route): " .. data
        end
    }
}

-- Feature flags
config.features = {
    user_management = true,
    product_catalog = true,
    file_operations = true,
    metrics = true,
    logging = true
}

-- Get feature status
function config.is_feature_enabled(feature_name)
    return config.features[feature_name] or false
end

-- Update configuration (simulated)
function config.update_setting(key, value)
    if config[key] ~= nil then
        local old_value = config[key]
        config[key] = value
        return true, "Updated " .. key .. " from " .. tostring(old_value) .. " to " .. tostring(value)
    else
        return false, "Unknown setting: " .. key
    end
end

-- Get all configuration as a table
function config.get_all()
    local all_config = {}
    for k, v in pairs(config) do
        if type(v) ~= "function" then
            all_config[k] = v
        end
    end
    return all_config
end

-- Validate configuration
function config.validate()
    local errors = {}
    
    if not config.mount_point or config.mount_point == "" then
        table.insert(errors, "Mount point is required")
    end
    
    if config.max_handlers and config.max_handlers < 1 then
        table.insert(errors, "max_handlers must be at least 1")
    end
    
    if #errors == 0 then
        return true, "Configuration is valid"
    else
        return false, table.concat(errors, ", ")
    end
end

return config