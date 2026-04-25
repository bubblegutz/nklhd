# Simple nklhd Example

This example demonstrates basic nklhd functionality with a single Lua script defining routes.

## Files

- `config.toml` - Configuration file specifying mount point and Lua script
- `main.lua` - Lua script defining routes using the global `app` object

## Features Demonstrated

- Basic route registration (`app.read`, `app.write`, `app.readdir`)
- Directory structures with nested files
- File statistics (`app.stat`) and walk operations (`app.walk`)
- Simple state management (in-memory counter)
- All core FUSE operations

## Route Overview

- `/` - Directory listing
- `/hello.txt` - Simple text file with greeting
- `/counter.txt` - Counter that increments on each read
- `/echo` - Echo handler that returns input data
- `/files/` - Directory with nested files
- `/files/readme.txt` - Nested file example
- `/files/nested/deep.txt` - Deeply nested file

## Running the Example

```bash
# Build nklhd (if not already built)
go build

# Run with the example config
./nklhd --config examples/simple/config.toml

# Alternative: specify mount point explicitly
./nklhd --config examples/simpler/config.toml --mount /tmp/my-nklhd

# With verbose logging
./nklhd --config examples/simpler/config.toml --verbose
```

## Exploring the Filesystem

Once mounted, explore the filesystem:

```bash
# List root directory
ls /tmp/nklhd-simple/

# Read files
cat /tmp/nklhd-simple/hello.txt
cat /tmp/nklhd-simple/counter.txt

# Write to echo handler
echo "test data" > /tmp/nklhd-simple/echo

# Explore directory structure
ls /tmp/nklhd-simple/files/
ls /tmp/nklhd-simple/files/nested/
cat /tmp/nklhd-simple/files/nested/deep.txt
```

## Configuration Details

The `config.toml` file:
```toml
mountpoint = "/tmp/nklhd-simple"
rootscript = "main.lua"
verbose = false
```

## Key Concepts

1. **Route Registration**: Routes are defined in Lua using `app.method(path, handler)`
2. **Handler Functions**: Receive a `req` table with request parameters
3. **State Management**: Lua variables persist across requests within the same script
4. **File Operations**: All standard FUSE operations are supported

## Testing Without Mounting

You can examine the Lua script directly:

```bash
# Check syntax
lua examples/simpler/main.lua

# Quick test of route definitions
lua -e 'app = {
  read=function(p,f) print("read", p) end,
  write=function(p,f) print("write", p) end,
  readdir=function(p,f) print("readdir", p) end
}; dofile("examples/simpler/main.lua")'
```