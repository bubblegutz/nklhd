# nklhd

A virtual filesystem that turns API routes into a filesystem — accessible via **FUSE**, **9p**, or **SSH/SFTP**, with route handlers written in **Lua** or **Tengo**.

## Quick Start

```bash
go build ./cmd/nklhd

# Mount a FUSE filesystem
./nklhd --config examples/lua/simpler/config.toml --mount /tmp/nklhd

# Read/write files = API calls
cat /tmp/nklhd/hello.txt
echo "hello world" > /tmp/nklhd/echo

# 9p protocol (no FUSE needed)
./nklhd --config examples/lua/simpler/config.toml --protocol 9p --listen localhost:5640

# SSH/SFTP protocol (no FUSE needed)
./nklhd --config examples/lua/simpler/config.toml --protocol ssh

# Run all available protocols
./nklhd --config examples/lua/simpler/config.toml --protocol all

# Unmount
./nklhd --umount /tmp/nklhd
```

## Examples

Every example has matching Lua and Tengo implementations. See [`examples/README.md`](examples/README.md).

| Example | Lua | Tengo |
|---------|-----|-------|
| Core file ops | `examples/lua/simpler/` | `examples/tengo/simpler/` |
| Modular CRUD app | `examples/lua/simple/` | `examples/tengo/simple/` |
| HTTP client | `examples/lua/http/` | `examples/tengo/http/` |
| JSON/YAML | `examples/lua/serialization/` | `examples/tengo/serialization/` |
| JSON↔YAML conversion | `examples/lua/conversion/` | `examples/tengo/conversion/` |
| jq queries | `examples/lua/jq/` | `examples/tengo/jq/` |
| OAuth 2.0 | `examples/lua/oauth/` | `examples/tengo/oauth/` |
| Utilities | `examples/lua/utilities/` | `examples/tengo/utilities/` |
| Wikipedia client | `examples/lua/wikipedia/` | `examples/tengo/wikipedia/` |

## Config

Single TOML file. `rootscript` points to your Lua or Tengo entry point.

```toml
mountpoint = "/tmp/nklhd"
rootscript = "main.lua"       # or "main.tengo"
verbose = true
protocol = "fuse"             # fuse, 9p, ssh, both, or all
ninepaddr = "localhost:5640"  # 9p listen address
sshaddr = "localhost:5022"    # SSH listen address
sshauthorizedkeys = "/path/to/authorized_keys"
sshhostkey = "/path/to/host_key"
```

## Route API

Both languages register routes on the global `app` object:

```
app.read(path, handler)     # file reads
app.write(path, handler)    # file writes
app.readdir(path, handler)  # directory listings
app.create(path, handler)   # file creation
app.unlink(path, handler)   # file removal
app.getattr(path, handler)  # file attributes
app.open(path, handler)     # file open
```

Path parameters: `app.read("/users/{id}", handler)` → `req.id` or `req.params.id`.

## Modules

Available in both languages:

| Module | Lua | Tengo |
|--------|-----|-------|
| JSON | `json.encode/decode/fromYAML` | `req.json.*` |
| YAML | `yaml.encode/decode/fromJSON` | `req.yaml.*` |
| HTTP | `http.request/client` | `req.http.*` |
| jq | `jq.wrap()` chainable | `req.jq.*` flat |
| OAuth | `oauth.client()` | `req.oauth.*` |
| Filesystem | `fs.*` | `req.fs.*` |
| Text/Fmt | Lua stdlib | `req.text/fmt.*` |

Lua uses `require("module")`. Tengo uses `req.<name>` in handlers or `import("name")` at top level.

## Project Structure

```
cmd/nklhd/        — CLI entry point
internal/
  config/         — Config loading (fig + flag)
  exports/        — Language-independent API interfaces
  fuse/           — FUSE server implementation
  ninep/          — 9p server implementation
  sshfs/          — SSH/SFTP server implementation
  lua/            — Lua executor + modules
  tengo/          — Tengo executor + modules
  router/         — Pattern-matching router (radix tree)
  script/         — Script runner abstraction
tests/            — Integration tests
examples/         — Example configs in lua/ and tengo/
```

## Testing

```bash
go test ./...
```
