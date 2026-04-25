# jq (jQuery-like) Module Example

This example demonstrates the jq module for jQuery-like table querying and manipulation in Lua scripts within nklhd.

## Overview

The jq module provides a fluent interface for querying and manipulating Lua tables, inspired by jQuery's DOM traversal methods. It's particularly useful for filtering, mapping, and extracting data from complex nested tables.

## Features Demonstrated

- **Table wrapping**: `jq.wrap(table)` creates a queryable object
- **Filtering**: Select elements matching a predicate function
- **Mapping**: Transform elements with a mapper function
- **Plucking**: Extract specific fields from objects
- **Pattern matching**: Find elements containing matching text (string pattern)
- **Counting**: Get number of elements in query
- **First/Last**: Get first or last element
- **Where clause**: Filter by exact field matches
- **Has check**: Check if elements have specific keys

## Running the Example

1. **Build nklhd** (if not already built):
   ```bash
   go build
   ```

2. **Run the jq example**:
   ```bash
   ./nklhd --config examples/jq/config.toml --verbose
   ```

3. **Explore the mounted filesystem**:
   ```bash
   ls /tmp/nklhd-jq/
   ls /tmp/nklhd-jq/jq/
   cat /tmp/nklhd-jq/jq/users
   cat /tmp/nklhd-jq/jq/users/names
   cat /tmp/nklhd-jq/jq/products/expensive
   cat /tmp/nklhd-jq/jq/search/go
   ```

## Available Routes

| Route | Method | Description |
|-------|--------|-------------|
| `/jq` | GET | Documentation and route list |
| `/jq/users` | GET | Raw users table (JSON) |
| `/jq/users/filtered` | GET | Users with age > 30 |
| `/jq/users/names` | GET | Just user names (pluck) |
| `/jq/users/admins` | GET | Users with role="admin" |
| `/jq/users/tags` | GET | Users with tag "go" |
| `/jq/products` | GET | Raw products table (JSON) |
| `/jq/products/expensive` | GET | Products with price > 100 |
| `/jq/products/electronics` | GET | Products in electronics category |
| `/jq/search/{pattern}` | GET | Search across datasets for pattern |
| `/jq/stats` | GET | Statistics about datasets |

## jq Module API Reference

### `jq.wrap(value)`

Creates a jq_query object from a Lua table (array or object).

**Parameters:**
- `value` (table): Lua table to wrap

**Returns:** `jq_query` userdata object

**Example:**
```lua
local users = {{name="Alice", age=30}, {name="Bob", age=25}}
local q = jq.wrap(users)
```

### jq_query Methods

All methods return a new jq_query object (chainable) except `get()`, `count()`, `first()`, `last()`, and `each()`.

#### `query:filter(predicate)`

Filter elements using a predicate function.

**Parameters:**
- `predicate` (function): Called with each element, returns truthy to keep

**Example:**
```lua
local adults = q:filter(function(u) return u.age >= 18 end):get()
```

#### `query:map(mapper)`

Transform each element with a mapper function.

**Parameters:**
- `mapper` (function): Called with each element, returns transformed value

**Example:**
```lua
local names = q:map(function(u) return u.name end):get()
```

#### `query:pluck(field)`

Extract a specific field from each object.

**Parameters:**
- `field` (string): Field name to extract

**Example:**
```lua
local names = q:pluck("name"):get()
```

#### `query:where(criteria)`

Filter objects where fields match given values (exact equality).

**Parameters:**
- `criteria` (table): Field-value pairs to match

**Example:**
```lua
local admins = q:where({role="admin"}):get()
```

#### `query:find(pattern)`

Find elements containing text matching the pattern.

**Parameters:**
- `pattern` (string): Pattern string like `/go/` (substring match)

**Example:**
```lua
local matches = q:find("/go/"):get()  -- finds elements with "go" in any string field
```

#### `query:has(key)`

Filter objects that have the specified key (field exists).

**Parameters:**
- `key` (string): Field name to check existence

**Example:**
```lua
local withEmail = q:has("email"):get()
```

#### `query:first()`

Get the first element in the query.

**Returns:** Single element (table or value)

**Example:**
```lua
local firstUser = q:first():get()
```

#### `query:last()`

Get the last element in the query.

**Returns:** Single element (table or value)

**Example:**
```lua
local lastUser = q:last():get()
```

#### `query:get()`

Execute the query and return the result as a Lua table.

**Returns:** Table containing query results

**Example:**
```lua
local results = q:filter(function(x) return x.active end):get()
```

#### `query:count()`

Count the number of elements in the query.

**Returns:** Integer count

**Example:**
```lua
local numAdults = q:filter(function(u) return u.age >= 18 end):count()
```

#### `query:each(iterator)`

Iterate over each element, calling the iterator function.

**Parameters:**
- `iterator` (function): Called with each element (index, value)

**Example:**
```lua
q:each(function(i, user)
    print(i, user.name)
end)
```

## Example Usage

### Basic Filtering and Mapping
```lua
local users = {
    {name="Alice", age=30, role="admin"},
    {name="Bob", age=25, role="user"},
    {name="Charlie", age=35, role="admin"}
}

local q = jq.wrap(users)
local adminNames = q:where({role="admin"}):pluck("name"):get()
-- Returns {"Alice", "Charlie"}
```

### Pattern Matching
```lua
local data = {
    {title="Lua programming guide", tags={"lua", "tutorial"}},
    {title="Go performance tips", tags={"go", "performance"}},
    {title="Python data science", tags={"python", "data"}}
}

local q = jq.wrap(data)
local goItems = q:find("/go/"):get()
-- Returns item with "Go performance tips"
```

### Complex Queries
```lua
local products = {
    {id=1, name="Laptop", price=999, category="electronics", stock=10},
    {id=2, name="Mouse", price=29, category="electronics", stock=50},
    {id=3, name="Desk", price=299, category="furniture", stock=5}
}

local q = jq.wrap(products)
local expensiveElectronics = q
    :filter(function(p) return p.price > 100 end)
    :where({category="electronics"})
    :get()
-- Returns only "Laptop"
```

## Integration with JSON Module

The example uses the JSON module to encode results for HTTP response:

```lua
local filtered = jq.wrap(data):filter(predicate):get()
local json = require("json")
local encoded, err = json.encode(filtered)
return encoded
```

## Testing the Example

Test the example routes using standard filesystem operations:

```bash
# View documentation
cat /tmp/nklhd-jq/jq

# List all users
cat /tmp/nklhd-jq/jq/users

# Get just user names
cat /tmp/nklhd-jq/jq/users/names

# Find expensive products
cat /tmp/nklhd-jq/jq/products/expensive

# Search for items with "go"
cat /tmp/nklhd-jq/jq/search/go

# Get statistics
cat /tmp/nklhd-jq/jq/stats
```

## Configuration File

The example uses `config.toml`:

```toml
# jq module example configuration
# Demonstrates jQuery-like table querying for Lua tables

# Mount point for the FUSE filesystem
mountpoint = "/tmp/nklhd-jq"

# Lua script that defines routes (relative to this config file directory)
rootscript = "main.lua"

# Enable verbose debug logging
verbose = false

# Optional: maximum concurrent handlers
# max_handlers = 4
```

## Troubleshooting

### No Results Returned
- Ensure your predicate functions return `true` for elements to keep
- Pattern matching uses substring search: `/go/` matches "go" anywhere in string fields
- `where()` requires exact field matches (case-sensitive)

### JSON Encoding Errors
- Ensure all table elements are serializable (no functions, userdata)
- Use `json.encode()` for complex nested tables

### Module Not Found
- The jq module is automatically available in nklhd Lua scripts
- No `require()` needed - use `jq.wrap()` directly
- If you get "attempt to call global 'jq'", ensure you're running within nklhd

## Advanced Patterns

### Chaining Multiple Operations
```lua
local result = jq.wrap(data)
    :filter(function(x) return x.active end)
    :map(function(x) return {id=x.id, name=x.name} end)
    :where({status="approved"})
    :get()
```

### Nested Table Traversal
```lua
local companies = {
    {
        name="Company A",
        employees={{name="Alice", age=30}, {name="Bob", age=25}}
    },
    {
        name="Company B", 
        employees={{name="Charlie", age=35}, {name="Diana", age=28}}
    }
}

-- Get all employee names across all companies
local allEmployees = {}
for _, company in ipairs(companies) do
    local empNames = jq.wrap(company.employees):pluck("name"):get()
    for _, name in ipairs(empNames) do
        table.insert(allEmployees, name)
    end
end
```

### Combining with Other Modules
```lua
local http = require("http")
local result, err = http.request("GET", "https://api.example.com/users")
if not err then
    local json = require("json")
    local data, err2 = json.decode(result.body)
    if not err2 then
        local activeUsers = jq.wrap(data.users)
            :filter(function(u) return u.active end)
            :pluck("email")
            :get()
        -- Process activeUsers
    end
end
```