-- Routes module for simple nklhd example
-- Defines route handlers organized by functionality

local routes = {}
local utils = require("utils")

-- User data storage (in-memory for example)
local users = {
    {id = "1", name = "Alice", email = "alice@example.com", role = "admin"},
    {id = "2", name = "Bob", email = "bob@example.com", role = "user"},
    {id = "3", name = "Charlie", email = "charlie@example.com", role = "user"}
}

-- Product catalog
local products = {
    {id = "100", name = "Widget", price = 9.99, stock = 42},
    {id = "101", name = "Gadget", price = 19.99, stock = 15},
    {id = "102", name = "Thingy", price = 4.99, stock = 100}
}

-- Get all routes defined in this module
function routes.get_routes()
    return {
        -- User management routes
        {method = "readdir", path = "/api/users", handler = routes.list_users},
        {method = "read", path = "/api/users/{id}", handler = routes.get_user},
        {method = "write", path = "/api/users", handler = routes.create_user},
        {method = "unlink", path = "/api/users/{id}", handler = routes.delete_user},
        
        -- Product catalog routes
        {method = "readdir", path = "/api/products", handler = routes.list_products},
        {method = "read", path = "/api/products/{id}", handler = routes.get_product},
        {method = "write", path = "/api/products", handler = routes.create_product},
        
        -- System routes
        {method = "read", path = "/api/health", handler = routes.health_check},
        {method = "readdir", path = "/api/metrics", handler = routes.list_metrics},
        {method = "read", path = "/api/metrics/{name}", handler = routes.get_metric},
        
        -- File operations
        {method = "create", path = "/tmp/{filename}", handler = routes.create_temp_file},
        {method = "unlink", path = "/tmp/{filename}", handler = routes.remove_temp_file},
        
        -- Directory operations
        {method = "lookup", path = "/api", handler = routes.walk_api},
        {method = "getattr", path = "/api/users/{id}", handler = routes.user_stat},
        {method = "open", path = "/api/logs/{date}.log", handler = routes.open_log_file}
    }
end

-- User management handlers
function routes.list_users(req)
    local user_list = {}
    for _, user in ipairs(users) do
        table.insert(user_list, user.id)
    end
    return table.concat(user_list, "\n")
end

function routes.get_user(req)
    local user_id = req.id
    for _, user in ipairs(users) do
        if user.id == user_id then
            return "ID: " .. user.id .. "\n" ..
                   "Name: " .. user.name .. "\n" ..
                   "Email: " .. user.email .. "\n" ..
                   "Role: " .. user.role
        end
    end
    return "User not found"
end

function routes.create_user(req)
    local data = req.data or ""
    utils.log("Creating user with data: " .. data)
    
    -- Parse JSON-like data (simplified)
    local name = data:match("name=([^&]+)") or "Unknown"
    local email = data:match("email=([^&]+)") or ""
    
    local new_user = {
        id = utils.generate_id(),
        name = name,
        email = email,
        role = "user"
    }
    
    table.insert(users, new_user)
    return "User created: " .. new_user.id .. "\n" ..
           "Name: " .. new_user.name .. "\n" ..
           "Email: " .. new_user.email
end

function routes.delete_user(req)
    local user_id = req.id
    for i, user in ipairs(users) do
        if user.id == user_id then
            table.remove(users, i)
            utils.log("Deleted user: " .. user_id)
            return "User deleted: " .. user_id
        end
    end
    return "User not found"
end

-- Product catalog handlers
function routes.list_products(req)
    local product_list = {}
    for _, product in ipairs(products) do
        table.insert(product_list, product.id .. ": " .. product.name .. 
                    " - $" .. product.price .. " (stock: " .. product.stock .. ")")
    end
    return table.concat(product_list, "\n")
end

function routes.get_product(req)
    local product_id = req.id
    for _, product in ipairs(products) do
        if product.id == product_id then
            return "ID: " .. product.id .. "\n" ..
                   "Name: " .. product.name .. "\n" ..
                   "Price: $" .. product.price .. "\n" ..
                   "Stock: " .. product.stock
        end
    end
    return "Product not found"
end

function routes.create_product(req)
    local data = req.data or ""
    utils.log("Creating product with data: " .. data)
    
    local name = data:match("name=([^&]+)") or "New Product"
    local price = tonumber(data:match("price=([^&]+)")) or 0
    local stock = tonumber(data:match("stock=([^&]+)")) or 0
    
    local new_product = {
        id = tostring(math.random(1000, 9999)),
        name = name,
        price = price,
        stock = stock
    }
    
    table.insert(products, new_product)
    return "Product created: " .. new_product.id .. "\n" ..
           "Name: " .. new_product.name .. "\n" ..
           "Price: $" .. new_product.price .. "\n" ..
           "Stock: " .. new_product.stock
end

-- System handlers
function routes.health_check(req)
    local mem_usage = utils.get_memory_usage()
    local uptime = utils.get_uptime()
    
    return "Status: OK\n" ..
           "Uptime: " .. uptime .. " seconds\n" ..
           "Memory: " .. mem_usage .. " MB\n" ..
           "Users: " .. #users .. "\n" ..
           "Products: " .. #products
end

function routes.list_metrics(req)
    return {"health", "users", "products", "uptime", "memory"}
end

function routes.get_metric(req)
    local metric_name = req.name
    if metric_name == "users" then
        return "Total users: " .. #users
    elseif metric_name == "products" then
        return "Total products: " .. #products
    elseif metric_name == "uptime" then
        return "Uptime: " .. utils.get_uptime() .. " seconds"
    elseif metric_name == "memory" then
        return "Memory usage: " .. utils.get_memory_usage() .. " MB"
    else
        return "Unknown metric: " .. metric_name
    end
end

-- File operation handlers
function routes.create_temp_file(req)
    local filename = req.filename
    utils.log("Creating temp file: " .. filename)
    return "Temporary file created: " .. filename .. "\n" ..
           "Path: /tmp/" .. filename .. "\n" ..
           "Time: " .. os.date("%Y-%m-%d %H:%M:%S")
end

function routes.remove_temp_file(req)
    local filename = req.filename
    utils.log("Removing temp file: " .. filename)
    return "Temporary file removed: " .. filename
end

-- Directory operation handlers
function routes.walk_api(req)
    return {
        {name = "users", is_dir = true},
        {name = "products", is_dir = true},
        {name = "health", is_dir = false},
        {name = "metrics", is_dir = true}
    }
end

function routes.user_stat(req)
    local user_id = req.id
    for _, user in ipairs(users) do
        if user.id == user_id then
            return {
                size = #user.name + #user.email + #user.role + 20,
                mode = 420,  -- 0644 in octal (rw-r--r--)
                mtime = os.time(),
                uid = 1000,
                gid = 1000
            }
        end
    end
    return {size = 0, mode = 0, mtime = 0}
end

function routes.open_log_file(req)
    local date = req.date or os.date("%Y-%m-%d")
    return "Opened log file for date: " .. date .. "\n" ..
           "Log entries: " .. math.random(10, 1000) .. "\n" ..
           "File size: " .. utils.format_bytes(math.random(1024, 1048576))
end

return routes