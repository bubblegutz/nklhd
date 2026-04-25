-- jq module example
-- Demonstrates jQuery-like table querying for Lua tables

app.log("Loading jq example script")

-- Sample dataset
local users = {
    { id = 1, name = "Alice", age = 30, role = "admin", tags = {"lua", "go"} },
    { id = 2, name = "Bob", age = 25, role = "user", tags = {"python"} },
    { id = 3, name = "Charlie", age = 35, role = "admin", tags = {"go", "rust"} },
    { id = 4, name = "Diana", age = 28, role = "moderator", tags = {"lua", "js"} },
    { id = 5, name = "Eve", age = 40, role = "user", tags = {"go", "python"} }
}

local products = {
    { id = 101, name = "Laptop", price = 999.99, category = "electronics", stock = 10 },
    { id = 102, name = "Mouse", price = 29.99, category = "electronics", stock = 50 },
    { id = 103, name = "Desk", price = 299.99, category = "furniture", stock = 5 },
    { id = 104, name = "Chair", price = 149.99, category = "furniture", stock = 8 },
    { id = 105, name = "Monitor", price = 399.99, category = "electronics", stock = 12 }
}

-- Route that displays documentation
app.read("/jq/help", function(req)
    return [[
jq Module Example
=================
Available routes:
  /jq/help           - This help message
  /jq/users          - List all users (raw table)
  /jq/users-filtered - Filter users with age > 30
  /jq/users-names    - Extract just names using pluck
  /jq/users-admins   - Filter by role
  /jq/users-tags     - Find users with specific tag
  /jq/products       - List all products
  /jq/products-expensive - Filter products with price > 100
  /jq/products-electronics - Filter by category
  /jq/search/{pattern} - Find items matching pattern (e.g., "go")

The jq module provides jQuery-like querying for Lua tables.
Use jq.wrap(table) to create a query object, then chain methods:

  local q = jq.wrap(users)
  q:filter(function(u) return u.age > 30 end):pluck("name"):get()

Methods: find, filter, map, pluck, where, has, first, last, get, each, count.
]]
end)

app.readdir("/jq", function(req)
    return "help\nusers\nusers-filtered\nusers-names\nusers-admins\nusers-tags\nproducts\nproducts-expensive\nproducts-electronics\nsearch\nstats"
end)

-- Raw users table
app.read("/jq/users", function(req)
    local json = require("json")
    local encoded, err = json.encode(users)
    if err then return "Error encoding JSON: " .. err end
    return encoded
end)

-- Filter users with age > 30
app.read("/jq/users-filtered", function(req)
    local q = jq.wrap(users)
    local filtered = q:filter(function(u) return u.age > 30 end):get()
    local json = require("json")
    local encoded, err = json.encode(filtered)
    if err then return "Error encoding JSON: " .. err end
    return encoded
end)

-- Extract just names using pluck
app.read("/jq/users-names", function(req)
    local q = jq.wrap(users)
    local names = q:pluck("name"):get()
    local json = require("json")
    local encoded, err = json.encode(names)
    if err then return "Error encoding JSON: " .. err end
    return encoded
end)

-- Filter by role
app.read("/jq/users-admins", function(req)
    local q = jq.wrap(users)
    local admins = q:where("role", "admin"):get()
    local json = require("json")
    local encoded, err = json.encode(admins)
    if err then return "Error encoding JSON: " .. err end
    return encoded
end)

-- Find users with specific tag (using find with pattern)
app.read("/jq/users-tags", function(req)
    local q = jq.wrap(users)
    -- Find users whose tags contain "go"
    local goUsers = q:find("/go/"):get()
    local json = require("json")
    local encoded, err = json.encode(goUsers)
    if err then return "Error encoding JSON: " .. err end
    return encoded
end)

-- Raw products table
app.read("/jq/products", function(req)
    local json = require("json")
    local encoded, err = json.encode(products)
    if err then return "Error encoding JSON: " .. err end
    return encoded
end)

-- Filter products with price > 100
app.read("/jq/products-expensive", function(req)
    local q = jq.wrap(products)
    local expensive = q:filter(function(p) return p.price > 100 end):get()
    local json = require("json")
    local encoded, err = json.encode(expensive)
    if err then return "Error encoding JSON: " .. err end
    return encoded
end)

-- Filter by category
app.read("/jq/products-electronics", function(req)
    local q = jq.wrap(products)
    local electronics = q:where("category", "electronics"):get()
    local json = require("json")
    local encoded, err = json.encode(electronics)
    if err then return "Error encoding JSON: " .. err end
    return encoded
end)

-- Search across both datasets using pattern matching
app.read("/jq/search/{pattern}", function(req)
    local pattern = req.pattern
    if not pattern then return "Missing pattern" end
    
    -- Combine datasets
    local allItems = {}
    for _, u in ipairs(users) do u._type = "user"; table.insert(allItems, u) end
    for _, p in ipairs(products) do p._type = "product"; table.insert(allItems, p) end
    
    local q = jq.wrap(allItems)
    local matches = q:find("/" .. pattern .. "/"):get()
    
    local json = require("json")
    local encoded, err = json.encode(matches)
    if err then return "Error encoding JSON: " .. err end
    return encoded
end)

-- Demonstrate count and first/last
app.read("/jq/stats", function(req)
    local userQ = jq.wrap(users)
    local productQ = jq.wrap(products)
    
    local userCount = userQ:count()
    local productCount = productQ:count()
    local oldestUser = userQ:last() -- sorted by age? Actually last in array
    local cheapestProduct = productQ:first() -- first in array
    
    local json = require("json")
    local stats = {
        user_count = userCount,
        product_count = productCount,
        oldest_user = oldestUser,
        cheapest_product = cheapestProduct
    }
    local encoded, err = json.encode(stats)
    if err then return "Error encoding JSON: " .. err end
    return encoded
end)

app.log("jq example routes registered")