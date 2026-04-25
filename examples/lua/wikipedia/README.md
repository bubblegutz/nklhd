# Wikipedia Example for nklhd

This example demonstrates a Wikipedia client using nklhd's FUSE filesystem. It provides a virtual filesystem interface to search Wikipedia, read articles, explore links, and manage bookmarks.

## Features

- **Search Wikipedia**: Write a search query to a file, get results as markdown files
- **Article browsing**: Read articles in markdown format with clickable links
- **Lazy link loading**: Links are fetched only when accessed
- **Persistent bookmarking**: Save articles to a hierarchical bookmark directory that persists across mounts
- **Real Wikipedia API integration**: Uses the Wikipedia REST API (https://en.wikipedia.org/w/api.php)
- **Filesystem module usage**: Demonstrates nklhd's new `fs` module for persistent storage

## Quick Start

1. **Build nklhd** (if not already built):
   ```bash
   cd /home/deega/repos/nklhd
   go build ./cmd/nklhd
   ```

2. **Run the Wikipedia example**:
   ```bash
   ./nklhd --config=examples/wikipedia/config.toml
   ```
   This mounts the filesystem at `/tmp/nklhd-wikipedia` (configurable in config.toml).

3. **In another terminal**, navigate to the mount point and start exploring:
   ```bash
   cd /tmp/nklhd-wikipedia
   ```

## Usage Examples

### Search for articles
```bash
echo "american folk music" > search
```
The search runs asynchronously. Check the output of the command for status.

### List search results
```bash
ls result/
```
Shows markdown files for each search result (e.g., `Woody_Guthrie.md`, `American_folk_music_revival.md`).

### Read an article
```bash
cat result/Woody_Guthrie.md
```
Returns the article content in markdown format with clickable links to related articles.

### Explore article links (lazy-loaded)
```bash
ls article/Woody_Guthrie/links/
```
Lists numbered links (1, 2, 3...). Each link can be read:
```bash
cat article/Woody_Guthrie/links/1
```
This fetches the linked article and returns a summary with a link to the full article.

### Bookmark articles
Create a bookmark directory:
```bash
mkdir bookmarks/music
```
Copy an article to bookmarks:
```bash
cp result/Woody_Guthrie.md bookmarks/music/
```
Bookmarks are stored persistently in `examples/wikipedia/bookmarks/` relative to the Lua script.

### Manage bookmarks
- List bookmarks: `ls bookmarks/`
- Read a bookmark: `cat bookmarks/music/Woody_Guthrie.md`
- Remove a bookmark: `rm bookmarks/music/Woody_Guthrie.md`
- Remove a directory: `rm -r bookmarks/music/`

## Configuration

The example includes a `config.toml` file:

```toml
# Mount point for the FUSE filesystem
mountpoint = "/tmp/nklhd-wikipedia"

# Main Lua script that implements the Wikipedia interface
rootscript = "main.lua"

# Enable verbose debug logging for troubleshooting
verbose = true
```

### Customizing the mount point
Change the `mountpoint` value in `config.toml` to any directory where you have write permissions.

## Implementation Details

### Lua Script (`main.lua`)
The main script demonstrates:
- Using nklhd's `http` module for API requests
- Using nklhd's `json` module for parsing Wikipedia API responses
- Using nklhd's `fs` module for persistent bookmark storage
- Route registration for FUSE operations (`read`, `write`, `readdir`, `create`, `unlink`)
- In-memory caching of search results and articles
- Lazy loading of linked articles

### Filesystem Module
The example uses the new `fs` module (replacing the old `shell` module) which provides filesystem operations:
- `fs.mkdir(path, parents, perm)`
- `fs.cp(src, dst, recursive)`
- `fs.rm(path, recursive)`
- `fs.cat(path)`
- `fs.write(path, content)`
- `fs.ls(path, recursive)`
- `fs.exists(path)`
- `fs.isdir(path)`
- `fs.dir(path)` (get parent directory)
- And more...

### Bookmark Persistence
Bookmarks are stored in `examples/wikipedia/bookmarks/` directory relative to the Lua script location. This ensures bookmarks persist across different mounts of the filesystem.

## Limitations

- Search results are limited to 10 articles
- Article links are limited to 20 per article (for performance)
- API requests have a 10-second timeout
- Requires internet access to Wikipedia
- Bookmark directory must be writable

## Troubleshooting

### Mount fails
- Ensure you have FUSE installed on your system
- Check that the mount point directory exists and is writable
- Run with `--verbose` flag for debug output

### No search results
- Check internet connectivity
- Wikipedia API may be temporarily unavailable
- Search query may be too specific

### Bookmark operations fail
- Ensure the `bookmarks/` directory exists in the example directory
- Check permissions on the directory

## Extending the Example

The example can be extended to:
- Add more Wikipedia API features (categories, images, etc.)
- Implement caching of articles to disk
- Add search history
- Support multiple languages
- Add authentication for private bookmark syncing

## Files

- `config.toml` - Configuration file
- `main.lua` - Main implementation script
- `README.md` - This documentation
- `bookmarks/` - Persistent bookmark storage (created automatically)

## See Also

- [nklhd Documentation](../AGENTS.md) - Project overview and development guide
- [Wikipedia API Documentation](https://en.wikipedia.org/w/api.php) - API reference
- [Other nklhd examples](../) - More examples of nklhd capabilities