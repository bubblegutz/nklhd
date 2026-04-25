# nklhd Examples

This directory contains example configurations demonstrating nklhd's capabilities
in both **Lua** and **Tengo**. Every example has a matching implementation in
both languages covering the same routes, operators, and features.

## Quick Start

```bash
# Build nklhd
go build ./cmd/nklhd

# Run any example
./nklhd --config examples/lua/simpler/config.toml --verbose
./nklhd --config examples/tengo/simpler/config.toml --verbose

# Explore the mounted filesystem
cat /tmp/nklhd-simple/hello.txt
```

## Examples

### Core File Operations (`simpler/`)

| | |
|---|---|
| **Lua** `examples/lua/simpler/` | **Tengo** `examples/tengo/simpler/` |
| `config.toml` + `main.lua` | `config.toml` + `main.tengo` |

Basic routing — readdir, read, write, create, unlink, open, getattr.

### Modular Application (`simple/`)

| | |
|---|---|
| **Lua** `examples/lua/simple/` | **Tengo** `examples/tengo/simple/` |
| `main.lua` + `utils.lua` + `routes.lua` + `config.lua` | `main.tengo` + `utils.tengo` + `routes.tengo` + `config.tengo` |

Multi-file modules via `require()` (Lua) and `import()` (Tengo). User/Product CRUD,
health checks, metrics, docs, echo, file operations.

### HTTP Client (`http/`)

| | |
|---|---|
| **Lua** `examples/lua/http/` | **Tengo** `examples/tengo/http/` |
| `http.request()` + `http.client()` | `http.request()` + `http.client()` |

GET/POST requests, basic/bearer auth, timeout configuration, JSON body handling.

### JSON / YAML Serialization (`serialization/`)

| | |
|---|---|
| **Lua** `examples/lua/serialization/` | **Tengo** `examples/tengo/serialization/` |
| `json.encode()` + `json.decode()` | `json.encode()` + `json.decode()` |

JSON encode/decode, JSON↔YAML conversion, env echo.

### JSON / YAML Conversion (`conversion/`)

| | |
|---|---|
| **Lua** `examples/lua/conversion/` | **Tengo** `examples/tengo/conversion/` |
| `json.fromYAML()` + `yaml.fromJSON()` + auto-detect | Same via `yaml.fromJSON()` + `yaml.decode()`/`json.encode()` |

Bidirectional JSON↔YAML with auto-detection (/convert).

### JSON Query (`jq/`)

| | |
|---|---|
| **Lua** `examples/lua/jq/` | **Tengo** `examples/tengo/jq/` |
| `jq.wrap()` chainable API | `jq.pluck()`, `.where()`, `.find()` flat API |

Filtering, plucking, searching on user/product datasets. Lua uses
chainable `jq.wrap()`; Tengo uses flat `jq.pluck()`, `.where()`, `.find()` functions.

### OAuth 2.0 (`oauth/`)

| | |
|---|---|
| **Lua** `examples/lua/oauth/` | **Tengo** `examples/tengo/oauth/` |
| `oauth.client()` with methods | Same via `oauth.client()` |

Device flow, auth code URL generation, token exchange, HTTP client attachment.

### Utilities (`utilities/`)

| | |
|---|---|
| **Lua** `examples/lua/utilities/` | **Tengo** `examples/tengo/utilities/` |
| `app.env`, `app.log()`, `app.debug()` | Same |

Environment variable access, logging, debug output.

### Wikipedia Client (`wikipedia/`)

| | |
|---|---|
| **Lua** `examples/lua/wikipedia/` | **Tengo** `examples/tengo/wikipedia/` |
| `http`, `json`, `fs` modules | Same modules via `req.http`, `.json`, `.fs` |

Wikipedia API search, article browsing, lazy link loading, persistent bookmarking
via `req.fs`.

## Route Definition API

Both languages use the same global `app` object with identical methods:

| Method | Lua | Tengo |
|--------|-----|-------|
| Method | Lua | Tengo |
|--------|-----|-------|
| `read` | `app.read("/hello", function(req)`<br>`  return "Hello"`<br>`end)` | `app.read("/hello", func(req) {`<br>`    return "Hello"`<br>`})` |
| `write` | `app.write("/echo", function(req)`<br>`  return "Echo: "..req.data`<br>`end)` | `app.write("/echo", func(req) {`<br>`    return "Echo: " + req.data`<br>`})` |
| `readdir` | `app.readdir("/api", function(req)`<br>`  return "a\nb\nc"`<br>`end)` | `app.readdir("/api", func(req) {`<br>`    return "a\nb\nc"`<br>`})` |
| `create` | `app.create("/tmp/{f}", function(req)`<br>`  return "created"`<br>`end)` | `app.create("/tmp/{f}", func(req) {`<br>`    return "created"`<br>`})` |
| `unlink` | `app.unlink("/tmp/{f}", function(req)`<br>`  return "removed"`<br>`end)` | `app.unlink("/tmp/{f}", func(req) {`<br>`    return "removed"`<br>`})` |
| `getattr` | `app.getattr("/u/{id}", function(req)`<br>`  return "size=100"`<br>`end)` | `app.getattr("/u/{id}", func(req) {`<br>`    return "size=100"`<br>`})` |
| `open` | `app.open("/logs/{d}", function(req)`<br>`  return "opened"`<br>`end)` | `app.open("/logs/{d}", func(req) {`<br>`    return "opened"`<br>`})` |

**Request object** (`req`) provides:
- `req.data` — file content being written (write handlers)
- `req.path` — request path
- `req.operation` — operation name (read, write, etc.)
- `req.{param}` — path parameters directly on req (e.g., `req.id` for `{id}`)

**Return values**: Lua handlers return any value; Tengo handlers must return strings.

**Modules inside handlers**: both Lua and Tengo handlers access modules directly:

```lua
-- Lua: modules are global
app.read("/encode", function(req)
    local data = {name = "alice", age = 30}
    return json.encode(data)
end)
```

```go
// Tengo: modules are accessed via req (without req.modules prefix)
app.read("/encode", func(req) {
    data := {name: "alice", age: 30}
    return string(req.json.encode(data))
})
```

## Built-in Modules

| Module | Lua API | Tengo API |
|--------|---------|-----------|
| JSON | `json.encode()` / `json.decode()` / `json.fromYAML()` | `json.encode()` / `json.decode()` *(via `req.json.*`)* |
| YAML | `yaml.encode()` / `yaml.decode()` / `yaml.fromJSON()` | `yaml.encode()` / `yaml.decode()` / `yaml.fromJSON()` *(via `req.yaml.*`)* |
| HTTP | `http.request()` / `http.client()` | `http.request()` / `http.client()` *(via `req.http.*`)* |
| jq | `jq.wrap()` (chainable) | `jq.pluck()` / `.where()` / `.find()` *(flat, via `req.jq.*`)* |
| OAuth | `oauth.client()` with methods | `oauth.client()` *(via `req.oauth.*`)* |
| Filesystem | `fs.cat()`, `fs.write()`, `fs.mkdir()`, etc. | `fs.cat()`, `fs.write()`, `fs.mkdir()` *(via `req.fs.*`)* |
| Text | _(standard Lua string lib)_ | `text.*` *(via `req.text.*`)* |
| Fmt | _(standard Lua string.format)_ | `fmt.*` *(via `req.fmt.*`)* |

Tengo modules are accessed as `req.<name>` inside handlers (e.g., `req.json.encode(data)`),
or via `import("name")` at the script top level. Lua modules use `require("name")`.

## Config Files

```toml
# Standard TOML config — works for both Lua and Tengo
mountpoint = "/tmp/nklhd"
rootscript = "main.lua"    # or "main.tengo"
verbose = true
```

Lua-only scripts can also be run via direct `.lua` config:
```bash
./nklhd --config examples/lua/simpler/main.lua --mount /tmp/nklhd
```

## Tengo Specifics

- **Stateless handlers**: each handler runs in a fresh VM; script-level variables
  are shallow-copied. Mutable maps (set by key, delete by key) persist across calls,
  but scalar globals do not.
- **`req.*`**: modules are flat on the request object (e.g., `req.json.encode()`).
- **Top-level `import()`**: for loading stdlib modules and relative `.tengo` files
  at script load time.
- Available stdlib modules via `req`: `json`, `os`, `text`, `fmt`, `math`,
  `rand`, `times`, `enum`, `hex`, `base64`, `fs`, `http`, `jq`, `yaml`, `oauth`.

## Troubleshooting

- **Module not found**: ensure `package.path` (Lua) or use `import()` (Tengo)
  with correct relative paths.
- **nklhd won't start**: check mount point exists and is writable, verify script
  syntax, use `--verbose`.
- **Tengo handlers can't access globals**: use `req.*` for modules;
  use mutable maps for shared state.
