-- Wikipedia example for nklhd
-- Demonstrates search, article browsing, lazy link loading, and bookmarking
-- using Wikipedia API and FUSE operations.

app.log("Loading Wikipedia example script")

-- Determine script directory for persistent bookmarks
local script_dir = debug.getinfo(1, "S").source:match("@?(.*/)") or "./"
local bookmarks_dir = script_dir .. "bookmarks"

-- Helper: URL encode a string for query parameters
local function url_encode(str)
    if str == nil then return "" end
    str = string.gsub(str, "([^%w%-%.%_%~ ])", function(c)
        return string.format("%%%02X", string.byte(c))
    end)
    str = string.gsub(str, " ", "+")
    return str
end

-- Ensure bookmarks directory exists
if not fs.exists(bookmarks_dir) then
    local ok, err = fs.mkdir(bookmarks_dir, true, 0755)
    if not ok then
        app.log("Warning: could not create bookmarks directory: " .. err)
    end
end

-- Helper: resolve bookmark path
local function resolve_bookmark_path(virtual_path)
    -- virtual_path is like "music/woody_guthrie.md" or "music/"
    -- Convert to filesystem path
    local fs_path = bookmarks_dir .. "/" .. virtual_path
    return fs_path
end

-- Helper: make a request to Wikipedia API
local function wikipedia_api(params)
    local http = require("http")
    local json = require("json")

    local base_url = "https://en.wikipedia.org/w/api.php"
    local query_params = {}
    for k, v in pairs(params) do
        table.insert(query_params, k .. "=" .. url_encode(v))
    end
    local url = base_url .. "?" .. table.concat(query_params, "&") .. "&format=json"

    app.log("Fetching Wikipedia API: " .. url)
    local result, err = http.request("GET", url, {
        timeout_ms = 10000,
        headers = {
            ["User-Agent"] = "nklhd-wikipedia-example/1.0 (https://github.com/bubblegutz/nklhd)"
        }
    })
    if err ~= nil then
        app.log("Wikipedia API error: " .. err)
        return nil, err
    end

    local data, err2 = json.decode(result.body)
    if err2 ~= nil then
        app.log("JSON decode error: " .. err2)
        app.log("Response body (first 200 chars): " .. string.sub(result.body, 1, 200))
        return nil, err2
    end

    return data
end

-- Search Wikipedia articles
local function wikipedia_search(query)
    local data, err = wikipedia_api({
        action = "query",
        list = "search",
        srsearch = query,
        srlimit = "10",
        srprop = "snippet|titlesnippet",
        utf8 = "1"
    })
    if err ~= nil then return nil, err end

    local results = {}
    if data.query and data.query.search then
        for _, item in ipairs(data.query.search) do
            table.insert(results, {
                title = item.title,
                snippet = item.snippet,
                pageid = item.pageid
            })
        end
    end
    return results
end

-- Get article content (extract)
local function wikipedia_article(title)
    local data, err = wikipedia_api({
        action = "query",
        prop = "extracts|links",
        titles = title,
        explaintext = "1",
        pllimit = "20",  -- limit links for demo
        utf8 = "1"
    })
    if err ~= nil then return nil, err end

    -- Extract content
    local pages = data.query and data.query.pages
    if not pages then return nil, "No pages found" end

    local page = next(pages)  -- get first page
    if not page or page.missing then
        return nil, "Article not found"
    end

    local content = page.extract or ""
    local links = {}
    if page.links then
        for _, link in ipairs(page.links) do
            table.insert(links, link.title)
        end
    end

    return {
        title = title,
        content = content,
        links = links
    }
end

-- Convert article to markdown format
local function article_to_markdown(article)
    local md = "# " .. article.title .. "\n\n"
    md = md .. article.content .. "\n\n"
    md = md .. "## Links\n\n"
    for i, link in ipairs(article.links) do
        md = md .. string.format("%d. [%s](article/%s/links/%d)\n", i, link, article.title, i)
    end
    return md
end

-- State storage (in-memory for search results and cached articles)
local state = {
    -- Current search results: table mapping title -> {title, snippet, pageid}
    search_results = {},
    -- Cached article content: title -> markdown content
    articles = {},
    -- Cached article links: title -> array of link titles
    article_links = {},
}

-- Route: write to /search triggers a search
app.write("/search", function(req)
    local query = req.data:gsub("%s+$", ""):gsub("^%s+", "")  -- trim whitespace
    if query == "" then
        return "Error: empty search query"
    end

    app.log("Searching Wikipedia for: " .. query)
    local results, err = wikipedia_search(query)
    if err ~= nil then
        return "Search error: " .. err
    end

    -- Store results in state
    state.search_results = {}
    for _, result in ipairs(results) do
        state.search_results[result.title] = result
    end

    -- Create result directory entries
    return "Search completed. " .. #results .. " results found.\n" ..
           "Run: ls result/ to see results.\n" ..
           "Or: cat result/Article_Title.md to read an article."
end)

-- Directory listing for /result
app.readdir("/result", function(_)
    local files = {}
    for title, _ in pairs(state.search_results) do
        -- Sanitize filename: replace spaces and slashes
        local filename = title:gsub("[ /]", "_") .. ".md"
        table.insert(files, filename)
    end
    return files
end)

-- Read a search result article (markdown)
app.read("/result/{title}.md", function(req)
    -- Handle potential missing params (e.g., when called from Attr)
    local params = req.params or {}
    local title = params.title
    if not title then
        -- Try to extract from path
        local path = req.path or ""
        title = path:match("/([^/]+)%.md$")
        if not title then
            return "Error: could not determine article title"
        end
    end
    title = title:gsub("_", " "):gsub("%.md$", "")
    local result = state.search_results[title]
    if not result then
        return "Error: article not found in search results"
    end

    -- Check if article already cached
    if not state.articles[title] then
        local article, err = wikipedia_article(title)
        if err ~= nil then
            return "Error fetching article: " .. err
        end
        state.articles[title] = article
        state.article_links[title] = article.links
    end

    -- Convert to markdown
    local md = article_to_markdown(state.articles[title])
    return md
end)

-- Directory listing for article links
app.readdir("/article/{title}/links", function(req)
    -- Handle potential missing params (e.g., when called from Attr)
    local params = req.params or {}
    local title = params.title
    if not title then
        -- Try to extract from path
        local path = req.path or ""
        title = path:match("/article/([^/]+)/links")
        if not title then
            return {}  -- empty directory
        end
    end
    title = title:gsub("_", " ")
    local links = state.article_links[title]
    if not links then
        return {}  -- empty directory
    end

    local files = {}
    for i = 1, #links do
        table.insert(files, tostring(i))
    end
    return files
end)

-- Read a specific link (lazy fetch)
app.read("/article/{title}/links/{link_id}", function(req)
    -- Handle potential missing params (e.g., when called from Attr)
    local params = req.params or {}
    local title = params.title
    if not title then
        -- Try to extract from path
        local path = req.path or ""
        title = path:match("/article/([^/]+)/links/[^/]+")
        if not title then
            return "Error: could not determine article title"
        end
    end
    title = title:gsub("_", " ")
    
    local link_id = tonumber(params.link_id)
    if not link_id then
        -- Try to extract from path
        local path = req.path or ""
        local link_str = path:match("/article/[^/]+/links/([^/]+)")
        link_id = tonumber(link_str)
    end
    local links = state.article_links[title]
    if not links or not link_id or link_id < 1 or link_id > #links then
        return "Error: invalid link ID"
    end

    local link_title = links[link_id]
    -- Fetch the linked article (cache)
    if not state.articles[link_title] then
        local article, err = wikipedia_article(link_title)
        if err ~= nil then
            return "Error fetching linked article: " .. err
        end
        state.articles[link_title] = article
        state.article_links[link_title] = article.links
    end

    -- Return a summary with link to read full article
    local article = state.articles[link_title]
    local summary = article.content:sub(1, 500) .. "..."
    return string.format("# %s\n\n%s\n\n[Read full article](result/%s.md)",
        link_title, summary, link_title:gsub("[ /]", "_"))
end)

-- Bookmark system (persistent to disk)

-- Write to a bookmark file (cp command writes content)
app.write("/bookmarks/{path}", function(req)
    -- Handle potential missing params (e.g., when called from Attr)
    local params = req.params or {}
    local virtual_path = params.path
    if not virtual_path then
        -- Try to extract from path
        local path = req.path or ""
        virtual_path = path:match("/bookmarks/(.+)")
        if not virtual_path then
            return "Error: could not determine bookmark path"
        end
    end
    local fs_path = resolve_bookmark_path(virtual_path)

    -- Ensure parent directory exists
    local parent = fs.dir(fs_path)
    if not fs.exists(parent) then
        local ok, err = fs.mkdir(parent, true, 0755)
        if not ok then
            return "Error creating directory: " .. err
        end
    end

    local ok, err = fs.write(fs_path, req.data)
    if not ok then
        return "Error writing bookmark: " .. err
    end

    return "Bookmark saved: " .. virtual_path
end)

-- Read a bookmark file
app.read("/bookmarks/{path}", function(req)
    -- Handle potential missing params (e.g., when called from Attr)
    local params = req.params or {}
    local virtual_path = params.path
    if not virtual_path then
        -- Try to extract from path
        local path = req.path or ""
        virtual_path = path:match("/bookmarks/(.+)")
        if not virtual_path then
            return "Error: could not determine bookmark path"
        end
    end
    local fs_path = resolve_bookmark_path(virtual_path)

    if not fs.exists(fs_path) then
        return "Error: bookmark not found"
    end

    local content, err = fs.cat(fs_path)
    if err ~= nil then
        return "Error reading bookmark: " .. err
    end

    return content
end)

-- Create a bookmark directory (mkdir command)
app.create("/bookmarks/{path}", function(req)
    -- Handle potential missing params (e.g., when called from Attr)
    local params = req.params or {}
    local virtual_path = params.path
    if not virtual_path then
        -- Try to extract from path
        local path = req.path or ""
        virtual_path = path:match("/bookmarks/(.+)")
        if not virtual_path then
            return "Error: could not determine bookmark path"
        end
    end
    local fs_path = resolve_bookmark_path(virtual_path)

    -- Check if already exists
    if fs.exists(fs_path) then
        return "Error: already exists"
    end

    local ok, err = fs.mkdir(fs_path, false, 0755)
    if not ok then
        return "Error creating directory: " .. err
    end

    return "Directory created: " .. virtual_path
end)

-- Remove a bookmark or directory (rm command)
app.unlink("/bookmarks/{path}", function(req)
    -- Handle potential missing params (e.g., when called from Attr)
    local params = req.params or {}
    local virtual_path = params.path
    if not virtual_path then
        -- Try to extract from path
        local path = req.path or ""
        virtual_path = path:match("/bookmarks/(.+)")
        if not virtual_path then
            return "Error: could not determine bookmark path"
        end
    end
    local fs_path = resolve_bookmark_path(virtual_path)

    if not fs.exists(fs_path) then
        return "Error: not found"
    end

    -- Determine if directory (need recursive) or file
    local recursive = false
    if fs.isdir(fs_path) then
        recursive = true
    end

    local ok, err = fs.rm(fs_path, recursive)
    if not ok then
        return "Error removing: " .. err
    end

    return "Removed: " .. virtual_path
end)

-- List bookmarks directory
app.readdir("/bookmarks", function(req)
    local virtual_path = req.path:gsub("^/bookmarks/?", "")
    if virtual_path == "" then virtual_path = "." end

    local fs_path = resolve_bookmark_path(virtual_path)

    if not fs.exists(fs_path) then
        return {}
    end

    if not fs.isdir(fs_path) then
        return {}  -- not a directory
    end

    local entries, err = fs.ls(fs_path, false)
    if err ~= nil then
        app.log("Error listing bookmarks: " .. err)
        return {}
    end

    local result = {}
    for _, entry in ipairs(entries) do
        local full_fs = fs_path .. "/" .. entry
        if fs.isdir(full_fs) then
            table.insert(result, entry .. "/")
        else
            table.insert(result, entry)
        end
    end

    return result
end)

-- Root directory listing
app.readdir("/", function(_)
    return {
        "search",
        "result/",
        "article/",
        "bookmarks/",
        "README.md"
    }
end)

-- README
app.read("/README.md", function(_)
    return [[# Wikipedia Example

This example demonstrates a Wikipedia client using nklhd's FUSE filesystem.

## Usage

1. Search for articles:
   ```bash
   echo "american folk music" > search
   ```

2. List search results:
   ```bash
   ls result/
   ```

3. Read an article:
   ```bash
   cat result/woody_guthrie.md
   ```

4. Explore article links (lazy-loaded):
   ```bash
   ls article/woody_guthrie/links/
   cat article/woody_guthrie/links/1
   ```

5. Bookmark articles:
   ```bash
   cp result/woody_guthrie.md bookmarks/music/
   ```

6. Create bookmark categories:
   ```bash
   mkdir bookmarks/music
   ```

## Features

- Real Wikipedia API integration
- Lazy loading of links
- Hierarchical bookmark system with persistence (stored in `bookmarks/` directory)
- Markdown formatting

## Implementation Notes

- Uses nklhd's http module for API requests
- Uses nklhd's fs module for persistent bookmark storage
- Caches articles and links in memory
- Demonstrates read, write, readdir, create, unlink operations
]]
end)

app.log("Wikipedia example loaded successfully")