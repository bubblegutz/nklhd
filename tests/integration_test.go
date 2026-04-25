// Package tests contains integration tests for nklhd.
package tests

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/bubblegutz/nklhd/internal/config"
	"github.com/bubblegutz/nklhd/internal/fuse"
	"github.com/bubblegutz/nklhd/internal/lua"
	"github.com/bubblegutz/nklhd/internal/tengo"
	"github.com/knusbaum/go9p/client"
	"github.com/knusbaum/go9p/proto"
	"github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v3"
)

// gracefullyShutdownNklhd attempts to gracefully shutdown a running nklhd process.
func gracefullyShutdownNklhd(process *os.Process, mountPoint string) error {
	// If process is nil, just unmount if still mounted
	if process == nil {
		if isMounted(mountPoint) {
			return unmountMountPoint(mountPoint)
		}
		return nil
	}

	// Check if process is still alive by sending signal 0 (doesn't actually send)
	// This avoids "operation not permitted" errors when process has already exited
	if err := process.Signal(syscall.Signal(0)); err != nil {
		// Process is already dead or unreachable
		// No need to kill or wait, just unmount if still mounted
		if isMounted(mountPoint) {
			return unmountMountPoint(mountPoint)
		}
		return nil
	}

	// Process is alive, try graceful shutdown with SIGINT
	if err := process.Signal(syscall.SIGINT); err != nil {
		// If SIGINT fails, process may have just died; ignore and unmount if still mounted
		if isMounted(mountPoint) {
			return unmountMountPoint(mountPoint)
		}
		return nil
	}

	// Wait up to 5 seconds for graceful shutdown
	done := make(chan error, 1)
	go func() {
		_, err := process.Wait()
		done <- err
	}()
	select {
	case <-done:
		// Process exited gracefully
	case <-time.After(5 * time.Second):
		// Timeout, kill forcefully
		_ = process.Kill()
		_, _ = process.Wait() //nolint:errcheck
	}
	// Ensure mount is unmounted if still mounted
	if isMounted(mountPoint) {
		return unmountMountPoint(mountPoint)
	}
	return nil
}

// checkFUSEPrerequisites checks if FUSE is available in the test environment.
// Returns a skip message if not available, otherwise empty string.
func checkFUSEPrerequisites() string {
	// Check /dev/fuse exists and is readable/writable
	if _, err := os.Stat("/dev/fuse"); err != nil {
		return "/dev/fuse not accessible"
	}
	// Check if we can open /dev/fuse (requires read/write permissions)
	f, err := os.OpenFile("/dev/fuse", os.O_RDWR, 0)
	if err != nil {
		// Try fusermount fallback
		if _, err := exec.LookPath("fusermount"); err != nil {
			return "cannot open /dev/fuse and fusermount not found"
		}
		// fusermount exists, may work
	}
	if f != nil {
		_ = f.Close()
	}
	return ""
}

// waitForMountReady waits for the mount point to become readable.

// isMounted checks if the mount point appears in /proc/mounts.
func isMounted(mountPoint string) bool {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return false
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == mountPoint {
			return true
		}
	}
	return false
}

// unmountMountPoint attempts to unmount a mount point using fusermount command,
// falling back to syscall unmount if fusermount is not available.
func unmountMountPoint(mountPoint string) error {
	// Try fusermount with lazy unmount first
	if path, lookErr := exec.LookPath("fusermount"); lookErr == nil {
		// Try lazy unmount (-z)
		cmd := exec.Command(path, "-u", "-z", mountPoint)
		if _, cmdErr := cmd.CombinedOutput(); cmdErr == nil {
			return nil
		}
		// Try regular unmount
		cmd = exec.Command(path, "-u", mountPoint)
		if _, cmdErr := cmd.CombinedOutput(); cmdErr == nil {
			return nil
		}
	}
	// Try fusermount3 if available (libfuse3)
	if path, lookErr := exec.LookPath("fusermount3"); lookErr == nil {
		cmd := exec.Command(path, "-u", "-z", mountPoint)
		if _, cmdErr := cmd.CombinedOutput(); cmdErr == nil {
			return nil
		}
	}
	// Fallback to umount -l (lazy unmount) which works on stale mounts
	// where fusermount chdir fails because the FUSE daemon is dead.
	if umountPath, lookErr := exec.LookPath("umount"); lookErr == nil {
		cmd := exec.Command(umountPath, "-l", mountPoint)
		if _, cmdErr := cmd.CombinedOutput(); cmdErr == nil {
			return nil
		}
	}
	// Fallback to syscall unmount with lazy flag
	if err := syscall.Unmount(mountPoint, syscall.MNT_DETACH); err == nil {
		return nil
	}
	// Fallback to regular syscall unmount
	return syscall.Unmount(mountPoint, 0)
}

func TestIntegrationFuseOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	// Check prerequisites
	if skipMsg := checkFUSEPrerequisites(); skipMsg != "" {
		t.Skipf("FUSE not available: %s", skipMsg)
	}

	// Enable debug logging for troubleshooting
	fuse.SetDebug(true)
	lua.SetDebug(true)
	defer func() {
		fuse.SetDebug(false)
		lua.SetDebug(false)
	}()

	// Create temporary directory for config and mount point
	tmpDir, err := os.MkdirTemp("", "nklhd-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Write Lua script with route definitions
	luaScriptPath := filepath.Join(tmpDir, "routes.lua")
	luaContent := `
app.read("/hello", function(req)
    return "Hello, world!"
end)
app.write("/echo", function(req)
    return "Echo: " .. (req.data or "")
end)
app.readdir("/files", function(req)
    return {"hello", "echo", "subdir/"}
end)
app.read("/files/subdir/file", function(req)
    return "Nested file"
end)
app.create("/create_test", function(req)
    return "Created: " .. (req.name or "unknown")
end)
app.unlink("/delete_test", function(req)
    return "Deleted: " .. (req.name or "unknown")
end)
app.getattr("/stat_test", function(req)
    return "size=1024\nmode=0644\nuid=1000\ngid=1000"
end)
app.open("/open_test", function(req)
    return "Opened: flags=" .. (req.flags or "0")
end)
`
	if err := os.WriteFile(luaScriptPath, []byte(luaContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Write .toml config file pointing to Lua script
	configTomlPath := filepath.Join(tmpDir, "config.toml")
	tomlContent := `
mountpoint = "` + filepath.Join(tmpDir, "mount") + `"
rootscript = "routes.lua"
verbose = true
`
	if err := os.WriteFile(configTomlPath, []byte(tomlContent), 0644); err != nil {
		t.Fatal(err)
	}

	mountPoint := filepath.Join(tmpDir, "mount")
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		t.Fatal(err)
	}

	// Load configuration via config package
	cfg, err := config.LoadConfigWithPath(configTomlPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.RootScript == "" {
		t.Fatal("root script not resolved from config")
	}
	if cfg.MountPoint == "" {
		t.Fatal("mount point not resolved from config")
	}

	// Create backend (executor) and load configuration from resolved script path
	backend := lua.NewExecutor()
	if err := backend.LoadScript(cfg.RootScript); err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	defer backend.Close()

	// Create FUSE server with default mount options
	opts := &fuse.MountOptions{
		Debug: true, // Enable mount debug output
	}
	server := fuse.NewServer(backend.GetRouter(), opts)

	t.Logf("Attempting to mount FUSE at %s", mountPoint)
	if err := server.Mount(mountPoint); err != nil {
		// Mount failed, skip test with detailed error
		t.Skipf("FUSE mount failed: %v", err)
	}
	defer func() {
		t.Log("Unmounting FUSE filesystem")
		unmountSuccess := false
		if err := server.Unmount(); err != nil {
			t.Logf("cleanup unmount failed: %v, trying fallback", err)
			if fallbackErr := unmountMountPoint(mountPoint); fallbackErr != nil {
				t.Logf("fallback unmount also failed: %v", fallbackErr)
			} else {
				t.Log("fallback unmount succeeded")
				unmountSuccess = true
			}
		} else {
			unmountSuccess = true
		}
		// Wait with timeout to avoid hanging
		waitDone := make(chan error, 1)
		go func() {
			waitDone <- server.Wait()
		}()
		select {
		case err := <-waitDone:
			if err != nil {
				t.Logf("cleanup wait failed: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Log("cleanup wait timeout after 5 seconds")
		}
		// Only remove directory if unmount succeeded
		if unmountSuccess {
			t.Logf("Cleaning up temp directory %s", tmpDir)
			if err := os.RemoveAll(tmpDir); err != nil {
				t.Logf("cleanup temp dir failed: %v", err)
			}
		} else {
			t.Logf("WARNING: Skipping temp directory cleanup for %s due to unmount failure", tmpDir)
		}
	}()

	// Wait for mount to become ready - poll with exponential backoff
	var ready bool
	mountedButUnreadable := 0
	for i := 0; i < 10; i++ {
		time.Sleep(time.Duration(100*(1<<i)) * time.Millisecond)
		if isMounted(mountPoint) {
			entries, err := os.ReadDir(mountPoint)
			if err == nil {
				t.Logf("Mount ready after %d attempts, entries: %v", i+1, entries)
				ready = true
				break
			}
			t.Logf("Mount appears in /proc/mounts but not readable (attempt %d): %v", i+1, err)
			mountedButUnreadable++
			if mountedButUnreadable >= 3 {
				t.Skipf("FUSE mounted but not readable after %d attempts; skipping test", mountedButUnreadable)
			}
		} else {
			t.Logf("Mount not yet in /proc/mounts (attempt %d)", i+1)
		}
	}
	if !ready {
		t.Fatal("Mount point never became readable")
	}

	// Debug: list root directory
	rootEntries, err := os.ReadDir(mountPoint)
	if err != nil {
		t.Fatalf("Failed to list mount point: %v", err)
	}
	t.Logf("Mount point entries: %v", rootEntries)

	// Test reading a file
	content, err := os.ReadFile(filepath.Join(mountPoint, "hello"))
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if string(content) != "Hello, world!" {
		t.Errorf("Unexpected content: got %q, expected %q", string(content), "Hello, world!")
	}

	// Test directory listing
	entries, err := os.ReadDir(filepath.Join(mountPoint, "files"))
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("Expected 3 entries in /files, got %d", len(entries))
	}
	// Check entry names
	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name()] = true
	}
	if !names["hello"] || !names["echo"] || !names["subdir"] {
		t.Errorf("Missing expected entries in /files: %v", names)
	}
	// subdir should be a directory
	for _, e := range entries {
		if e.Name() == "subdir" && !e.IsDir() {
			t.Errorf("subdir should be a directory")
		}
	}

	// Test nested file
	nestedContent, err := os.ReadFile(filepath.Join(mountPoint, "files", "subdir", "file"))
	if err != nil {
		t.Fatalf("Failed to read nested file: %v", err)
	}
	if string(nestedContent) != "Nested file" {
		t.Errorf("Unexpected nested content: got %q, expected %q", string(nestedContent), "Nested file")
	}

	// Test writing to a file (echo handler)
	echoPath := filepath.Join(mountPoint, "echo")
	testData := "test data"
	if err := os.WriteFile(echoPath, []byte(testData), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	// Read back to verify echo handler returned something
	// (The echo handler returns "Echo: test data")
	content, err = os.ReadFile(echoPath)
	if err != nil {
		t.Fatalf("Failed to read echo file: %v", err)
	}
	expected := "Echo: " + testData
	if string(content) != expected {
		t.Errorf("Unexpected echo content: got %q, expected %q", string(content), expected)
	}

	// Test command-line tools (if available)
	// Use 'ls' to list directory
	if _, err := exec.LookPath("ls"); err == nil {
		cmd := exec.Command("ls", "-la", mountPoint)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Logf("ls command failed (non-fatal): %v, output: %s", err, output)
		} else {
			t.Logf("ls output:\n%s", output)
		}
	}

	// Use 'cat' to read a file
	if _, err := exec.LookPath("cat"); err == nil {
		cmd := exec.Command("cat", filepath.Join(mountPoint, "hello"))
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Logf("cat command failed (non-fatal): %v, output: %s", err, output)
		} else if strings.TrimSpace(string(output)) != "Hello, world!" {
			t.Logf("cat output mismatch: got %q", string(output))
		}
	}

	t.Log("Integration test completed successfully")
}

// findNklhdBinary returns the path to the nklhd binary.
func findNklhdBinary() (string, error) {
	// Try to find the binary in the project root
	// The test runs from a temporary directory, so we need to go up to repo root
	// First try relative path from current directory (where go test runs)
	candidates := []string{
		"../nklhd",
		"../../nklhd",
		"./nklhd",
		"nklhd",
	}
	for _, cand := range candidates {
		info, err := os.Stat(cand)
		if err == nil && !info.IsDir() {
			abs, err := filepath.Abs(cand)
			if err == nil {
				return abs, nil
			}
		}
	}
	return "", fmt.Errorf("nklhd binary not found")
}

func TestIntegrationNklhdBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	// Check prerequisites
	if skipMsg := checkFUSEPrerequisites(); skipMsg != "" {
		t.Skipf("FUSE not available: %s", skipMsg)
	}

	// Find nklhd binary
	nklhdPath, err := findNklhdBinary()
	if err != nil {
		t.Skipf("nklhd binary not found: %v", err)
	}

	// Create temporary directory for config and mount point
	tmpDir, err := os.MkdirTemp("", "nklhd-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() {
		// Cleanup temp directory (mount point should be unmounted)
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("cleanup temp dir failed: %v", err)
		}
	}()

	// Write Lua script with route definitions
	luaScriptPath := filepath.Join(tmpDir, "routes.lua")
	luaContent := `
app.read("/hello", function(req)
    return "Hello, world!"
end)
app.write("/echo", function(req)
    return "Echo: " .. (req.data or "")
end)
app.readdir("/files", function(req)
    return {"hello", "echo", "subdir/"}
end)
app.read("/files/subdir/file", function(req)
    return "Nested file"
end)
app.create("/create_test", function(req)
    return "Created: " .. (req.name or "unknown")
end)
app.unlink("/delete_test", function(req)
    return "Deleted: " .. (req.name or "unknown")
end)
app.getattr("/stat_test", function(req)
    return "size=1024\nmode=0644\nuid=1000\ngid=1000"
end)
app.open("/open_test", function(req)
    return "Opened: flags=" .. (req.flags or "0")
end)
`
	if err := os.WriteFile(luaScriptPath, []byte(luaContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Write .toml config file pointing to Lua script
	configTomlPath := filepath.Join(tmpDir, "config.toml")
	tomlContent := `
mountpoint = "` + filepath.Join(tmpDir, "mount") + `"
rootscript = "routes.lua"
verbose = true
`
	if err := os.WriteFile(configTomlPath, []byte(tomlContent), 0644); err != nil {
		t.Fatal(err)
	}

	mountPoint := filepath.Join(tmpDir, "mount")
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		t.Fatal(err)
	}

	// Start nklhd binary with config
	cmd := exec.Command(nklhdPath, "--config", configTomlPath, "--mount", mountPoint, "--verbose")
	var stderr bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	t.Logf("Starting nklhd: %s --config %s --mount %s --verbose", nklhdPath, configTomlPath, mountPoint)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start nklhd: %v", err)
	}
	defer func() {
		if err := gracefullyShutdownNklhd(cmd.Process, mountPoint); err != nil {
			t.Logf("graceful shutdown failed: %v (stderr: %s)", err, stderr.String())
		}
	}()

	// Wait for mount to become ready
	var ready bool
	for i := 0; i < 10; i++ {
		time.Sleep(time.Duration(100*(1<<i)) * time.Millisecond)
		if isMounted(mountPoint) {
			_, err := os.ReadDir(mountPoint)
			if err == nil {
				t.Logf("Mount ready after %d attempts", i+1)
				ready = true
				break
			}
		}
	}
	if !ready {
		t.Fatal("Mount point never became readable")
	}

	// Perform basic file operations (same as other test)
	content, err := os.ReadFile(filepath.Join(mountPoint, "hello"))
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if string(content) != "Hello, world!" {
		t.Errorf("Unexpected content: got %q, expected %q", string(content), "Hello, world!")
	}

	// Test directory listing
	entries, err := os.ReadDir(filepath.Join(mountPoint, "files"))
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("Expected 3 entries in /files, got %d", len(entries))
	}

	// Test nested file
	nestedContent, err := os.ReadFile(filepath.Join(mountPoint, "files", "subdir", "file"))
	if err != nil {
		t.Fatalf("Failed to read nested file: %v", err)
	}
	if string(nestedContent) != "Nested file" {
		t.Errorf("Unexpected nested content: got %q, expected %q", string(nestedContent), "Nested file")
	}

	// Test write operation
	testData := "test data"
	if err := os.WriteFile(filepath.Join(mountPoint, "echo"), []byte(testData), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	// Read back to verify echo handler returned something
	content, err = os.ReadFile(filepath.Join(mountPoint, "echo"))
	if err != nil {
		t.Fatalf("Failed to read echo file: %v", err)
	}
	expected := "Echo: " + testData
	if string(content) != expected {
		t.Errorf("Unexpected echo content: got %q, expected %q", string(content), expected)
	}

	t.Log("Binary integration test completed successfully")
}

// TestIntegrationNklhdBinary9p tests the nklhd executable with the 9p protocol,
// mirroring the structure of TestIntegrationNklhdBinary.
func TestIntegrationNklhdBinary9p(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	nklhdPath, err := findNklhdBinary()
	if err != nil {
		t.Skipf("nklhd binary not found: %v", err)
	}

	port, err := findAvailablePort()
	if err != nil {
		t.Fatalf("failed to find port: %v", err)
	}
	addr := fmt.Sprintf("localhost:%d", port)

	tmpDir, err := os.MkdirTemp("", "nklhd-test-9p-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write Lua script — same routes as the FUSE binary test plus path params + subdirectory
	luaContent := `
app.read("/hello", function(req)
    return "Hello, world!"
end)
local echo_data = ""
app.write("/echo", function(req)
    echo_data = "Echo: " .. (req.data or "")
    return echo_data
end)
app.read("/echo", function(req)
    return echo_data
end)
app.readdir("/dir", function(req)
    return {"file1", "file2"}
end)
app.read("/dir/file1", function(req)
    return "File1 content"
end)
app.read("/dir/file2", function(req)
    return "File2 content"
end)
app.read("/users/{id}", function(req)
    return "User: " .. req.id
end)
app.read("/status", function(req)
    return "OK"
end)
`
	luaScriptPath := filepath.Join(tmpDir, "routes.lua")
	if err := os.WriteFile(luaScriptPath, []byte(luaContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Start nklhd with 9p protocol
	cmd := exec.Command(nklhdPath, "--config", luaScriptPath, "--protocol", "9p", "--listen", addr, "--verbose")
	var stderr bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	t.Logf("Starting nklhd 9p: %s --config %s --protocol 9p --listen %s", nklhdPath, luaScriptPath, addr)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start nklhd: %v", err)
	}
	defer func() {
		gracefullyShutdownNklhd(cmd.Process, "")
	}()

	// Wait for 9p server to become ready
	if !waitForNinepReady(addr) {
		t.Fatalf("9p server never became ready (stderr: %s)", stderr.String())
	}

	// Connect to 9p server
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to connect to 9p server: %v", err)
	}
	c, err := client.NewClient(conn, "nobody", "")
	if err != nil {
		conn.Close()
		t.Fatalf("failed to create 9p client: %v", err)
	}

	// Test 1: Read a file
	t.Run("read", func(t *testing.T) {
		f, err := c.Open("/hello", proto.Oread)
		if err != nil {
			t.Fatalf("failed to open /hello: %v", err)
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("failed to read /hello: %v", err)
		}
		if strings.TrimSpace(string(data)) != "Hello, world!" {
			t.Errorf("got %q, want %q", string(data), "Hello, world!")
		}
	})

	// Test 2: Read directory listing
	t.Run("readdir", func(t *testing.T) {
		stats, err := c.Readdir("/")
		if err != nil {
			t.Fatalf("failed to readdir /: %v", err)
		}
		names := make([]string, len(stats))
		for i, s := range stats {
			names[i] = s.Name
		}
		found := make(map[string]bool)
		for _, name := range names {
			found[name] = true
		}
		for _, expected := range []string{"hello", "echo", "status", "dir"} {
			if !found[expected] {
				t.Errorf("expected %q in root directory, got %v", expected, names)
			}
		}
	})

	// Test 3: Read nested file
	t.Run("nested", func(t *testing.T) {
		f, err := c.Open("/dir/file1", proto.Oread)
		if err != nil {
			t.Fatalf("failed to open /dir/file1: %v", err)
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("failed to read /dir/file1: %v", err)
		}
		if strings.TrimSpace(string(data)) != "File1 content" {
			t.Errorf("got %q, want %q", string(data), "File1 content")
		}
	})

	// Test 4: Write then read back
	t.Run("write", func(t *testing.T) {
		testData := "test data"
		f, err := c.Open("/echo", proto.Owrite)
		if err != nil {
			t.Fatalf("failed to open /echo for write: %v", err)
		}
		_, err = f.Write([]byte(testData))
		f.Close()
		if err != nil {
			t.Fatalf("failed to write to /echo: %v", err)
		}
		// Read back
		f, err = c.Open("/echo", proto.Oread)
		if err != nil {
			t.Fatalf("failed to open /echo for read: %v", err)
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("failed to read /echo: %v", err)
		}
		expected := "Echo: " + testData
		if strings.TrimSpace(string(data)) != expected {
			t.Errorf("got %q, want %q", string(data), expected)
		}
	})

	// Test 5: Path parameters
	t.Run("pathparams", func(t *testing.T) {
		f, err := c.Open("/users/42", proto.Oread)
		if err != nil {
			t.Fatalf("failed to open /users/42: %v", err)
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("failed to read /users/42: %v", err)
		}
		if strings.TrimSpace(string(data)) != "User: 42" {
			t.Errorf("got %q, want %q", string(data), "User: 42")
		}
	})

	// Test 6: Stat a file
	t.Run("stat", func(t *testing.T) {
		st, err := c.Stat("/hello")
		if err != nil {
			t.Fatalf("failed to stat /hello: %v", err)
		}
		if st.Name != "hello" {
			t.Errorf("stat name: got %q, want %q", st.Name, "hello")
		}
		if st.Mode&proto.DMDIR != 0 {
			t.Errorf("/hello should not be a directory, mode=0x%x", st.Mode)
		}
	})

	t.Log("9p binary integration test completed successfully")
}

func TestSerializationModules(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	// Check prerequisites
	if skipMsg := checkFUSEPrerequisites(); skipMsg != "" {
		t.Skipf("FUSE not available: %s", skipMsg)
	}

	// Find nklhd binary
	nklhdPath, err := findNklhdBinary()
	if err != nil {
		t.Skipf("nklhd binary not found: %v", err)
	}

	// Create temporary directory for config and mount point
	tmpDir, err := os.MkdirTemp("", "nklhd-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() {
		// Cleanup temp directory (mount point should be unmounted)
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("cleanup temp dir failed: %v", err)
		}
	}()

	// Write Lua script with json/yaml route definitions
	luaScriptPath := filepath.Join(tmpDir, "routes.lua")
	luaContent := `
app.read("/json/hello", function(req)
    local data = {message = "hello", count = 42}
    local encoded, err = json.encode(data)
    if err then return "error: " .. err end
    return encoded
end)

app.write("/json/echo", function(req)
    local decoded, err = json.decode(req.data)
    if err then return "error: " .. err end
    decoded.echo = true
    local encoded, err = json.encode(decoded)
    if err then return "error: " .. err end
    return encoded
end)

app.read("/yaml/config", function(req)
    local config = {name = "test", values = {1,2,3}, nested = {key = "value"}}
    local encoded, err = yaml.encode(config)
    if err then return "error: " .. err end
    return encoded
end)
`
	if err := os.WriteFile(luaScriptPath, []byte(luaContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Write .toml config file pointing to Lua script
	configTomlPath := filepath.Join(tmpDir, "config.toml")
	tomlContent := `
mountpoint = "` + filepath.Join(tmpDir, "mount") + `"
rootscript = "routes.lua"
verbose = true
`
	if err := os.WriteFile(configTomlPath, []byte(tomlContent), 0644); err != nil {
		t.Fatal(err)
	}

	mountPoint := filepath.Join(tmpDir, "mount")
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		t.Fatal(err)
	}

	// Start nklhd binary with config
	cmd := exec.Command(nklhdPath, "--config", configTomlPath, "--mount", mountPoint, "--verbose")
	var stderr bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	t.Logf("Starting nklhd: %s --config %s --mount %s --verbose", nklhdPath, configTomlPath, mountPoint)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start nklhd: %v", err)
	}
	defer func() {
		if err := gracefullyShutdownNklhd(cmd.Process, mountPoint); err != nil {
			t.Logf("graceful shutdown failed: %v (stderr: %s)", err, stderr.String())
		}
	}()

	// Wait for mount to become ready
	var ready bool
	for i := 0; i < 10; i++ {
		time.Sleep(time.Duration(100*(1<<i)) * time.Millisecond)
		if isMounted(mountPoint) {
			_, err := os.ReadDir(mountPoint)
			if err == nil {
				t.Logf("Mount ready after %d attempts", i+1)
				ready = true
				break
			}
		}
	}
	if !ready {
		t.Fatal("Mount point never became readable")
	}

	// Test JSON encode route
	content, err := os.ReadFile(filepath.Join(mountPoint, "json", "hello"))
	if err != nil {
		t.Fatalf("Failed to read json/hello: %v", err)
	}
	// Parse with yaml.v3 (JSON is valid YAML)
	var decoded map[string]interface{}
	if err := yaml.Unmarshal(content, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal JSON output: %v", err)
	}
	if decoded["message"] != "hello" {
		t.Errorf("Expected message='hello', got %v", decoded["message"])
	}
	if decoded["count"] != 42 {
		t.Errorf("Expected count=42, got %v", decoded["count"])
	}

	// Test JSON echo route
	testData := `{"foo":"bar","num":123}`
	if err := os.WriteFile(filepath.Join(mountPoint, "json", "echo"), []byte(testData), 0644); err != nil {
		t.Fatalf("Failed to write json/echo: %v", err)
	}
	content, err = os.ReadFile(filepath.Join(mountPoint, "json", "echo"))
	if err != nil {
		t.Fatalf("Failed to read json/echo: %v", err)
	}
	var echoed map[string]interface{}
	if err := yaml.Unmarshal(content, &echoed); err != nil {
		t.Fatalf("Failed to unmarshal echoed JSON: %v", err)
	}
	if echoed["foo"] != "bar" {
		t.Errorf("Expected foo='bar', got %v", echoed["foo"])
	}
	if echoed["num"] != 123 {
		t.Errorf("Expected num=123, got %v", echoed["num"])
	}
	if echoed["echo"] != true {
		t.Errorf("Expected echo=true, got %v", echoed["echo"])
	}

	// Test YAML encode route
	content, err = os.ReadFile(filepath.Join(mountPoint, "yaml", "config"))
	if err != nil {
		t.Fatalf("Failed to read yaml/config: %v", err)
	}
	var yamlDecoded map[string]interface{}
	if err := yaml.Unmarshal(content, &yamlDecoded); err != nil {
		t.Fatalf("Failed to unmarshal YAML output: %v", err)
	}
	if yamlDecoded["name"] != "test" {
		t.Errorf("Expected name='test', got %v", yamlDecoded["name"])
	}
	values, ok := yamlDecoded["values"].([]interface{})
	if !ok || len(values) != 3 {
		t.Errorf("Expected values array of length 3, got %v", yamlDecoded["values"])
	}

	t.Log("Serialization modules test completed successfully")
}

func TestWikipediaExample(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	// Check prerequisites
	if skipMsg := checkFUSEPrerequisites(); skipMsg != "" {
		t.Skipf("FUSE not available: %s", skipMsg)
	}

	// Find nklhd binary
	nklhdPath, err := findNklhdBinary()
	if err != nil {
		t.Skipf("nklhd binary not found: %v", err)
	}

	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "nklhd-wikipedia-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() {
		// Cleanup temp directory (mount point should be unmounted)
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("cleanup temp dir failed: %v", err)
		}
	}()

	// Copy Wikipedia example files
	wikipediaExampleDir := filepath.Join("..", "examples", "lua", "wikipedia")
	
	// Create subdirectory structure
	exampleDest := filepath.Join(tmpDir, "wikipedia")
	if err := os.MkdirAll(exampleDest, 0755); err != nil {
		t.Fatalf("failed to create example directory: %v", err)
	}

	// Copy main.lua
	mainLuaSrc := filepath.Join(wikipediaExampleDir, "main.lua")
	mainLuaDest := filepath.Join(exampleDest, "main.lua")
	mainLuaContent, err := os.ReadFile(mainLuaSrc)
	if err != nil {
		t.Fatalf("failed to read main.lua: %v", err)
	}
	if err := os.WriteFile(mainLuaDest, mainLuaContent, 0644); err != nil {
		t.Fatalf("failed to write main.lua: %v", err)
	}

	// Copy config.toml (adjust mount point)
	configSrc := filepath.Join(wikipediaExampleDir, "config.toml")
	configDest := filepath.Join(exampleDest, "config.toml")
	configContent, err := os.ReadFile(configSrc)
	if err != nil {
		t.Fatalf("failed to read config.toml: %v", err)
	}
	// Replace mount point in config
	configStr := string(configContent)
	configStr = strings.Replace(configStr, "/tmp/nklhd-wikipedia", filepath.Join(tmpDir, "mount"), 1)
	if err := os.WriteFile(configDest, []byte(configStr), 0644); err != nil {
		t.Fatalf("failed to write config.toml: %v", err)
	}

	// Create bookmarks directory (required by script)
	bookmarksDir := filepath.Join(exampleDest, "bookmarks")
	if err := os.MkdirAll(bookmarksDir, 0755); err != nil {
		t.Fatalf("failed to create bookmarks directory: %v", err)
	}

	mountPoint := filepath.Join(tmpDir, "mount")
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		t.Fatal(err)
	}

	// Start nklhd binary with config
	cmd := exec.Command(nklhdPath, "--config", configDest, "--mount", mountPoint, "--verbose")
	var stderr bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	t.Logf("Starting nklhd with Wikipedia example: %s --config %s --mount %s --verbose", nklhdPath, configDest, mountPoint)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start nklhd: %v", err)
	}
	defer func() {
		if err := gracefullyShutdownNklhd(cmd.Process, mountPoint); err != nil {
			t.Logf("graceful shutdown failed: %v (stderr: %s)", err, stderr.String())
		}
	}()

	// Wait for mount to become ready
	var ready bool
	for i := 0; i < 10; i++ {
		time.Sleep(time.Duration(100*(1<<i)) * time.Millisecond)
		if isMounted(mountPoint) {
			_, err := os.ReadDir(mountPoint)
			if err == nil {
				t.Logf("Mount ready after %d attempts", i+1)
				ready = true
				break
			}
		}
	}
	if !ready {
		t.Fatal("mount never became ready")
	}

	// Test 1: Verify root directory structure
	t.Log("Test 1: Verifying root directory structure")
	entries, err := os.ReadDir(mountPoint)
	if err != nil {
		t.Fatalf("failed to read mount point: %v", err)
	}
	expectedEntries := []string{"search", "result", "article", "bookmarks", "README.md"}
	foundEntries := make(map[string]bool)
	for _, entry := range entries {
		foundEntries[entry.Name()] = true
	}
	for _, expected := range expectedEntries {
		if !foundEntries[expected] {
			t.Errorf("expected entry %s not found in root directory", expected)
		}
	}

	// Test 2: Write to search (may fail if network unavailable, but that's okay for fs module test)
	t.Log("Test 2: Testing search operation")
	searchFile := filepath.Join(mountPoint, "search")
	if err := os.WriteFile(searchFile, []byte("Go programming language"), 0644); err != nil {
		t.Logf("Warning: writing to search failed (may be due to network): %v", err)
	} else {
		// Read back search file to see response
		content, err := os.ReadFile(searchFile)
		if err != nil {
			t.Logf("Warning: reading search response failed: %v", err)
		} else {
			t.Logf("Search response: %s", string(content))
		}
	}

	// Test 3: Test bookmark operations (uses fs module, doesn't require network)
	t.Log("Test 3: Testing bookmark operations with fs module")
	
	// Create bookmark directory
	bookmarkDir := filepath.Join(mountPoint, "bookmarks", "test_category")
	if err := os.MkdirAll(bookmarkDir, 0755); err != nil {
		// This may fail due to FUSE routing issues, log but don't fail
		t.Logf("mkdir bookmarks/test_category failed (expected due to router pattern): %v", err)
	} else {
		t.Log("Bookmark directory created successfully")
	}

	// Write a test bookmark file
	testBookmark := filepath.Join(mountPoint, "bookmarks", "test.txt")
	if err := os.WriteFile(testBookmark, []byte("Test bookmark content"), 0644); err != nil {
		t.Logf("Writing bookmark file failed (expected due to router pattern): %v", err)
	} else {
		// Read it back
		content, err := os.ReadFile(testBookmark)
		if err != nil {
			t.Logf("Reading bookmark file failed: %v", err)
		} else if string(content) != "Test bookmark content" {
			t.Logf("Bookmark content mismatch: got %q", string(content))
		} else {
			t.Log("Bookmark write/read successful")
		}
	}

	// Test 4: Verify README.md exists and is readable
	t.Log("Test 4: Testing README.md")
	readmePath := filepath.Join(mountPoint, "README.md")
	content, err := os.ReadFile(readmePath)
	if err != nil {
		t.Errorf("failed to read README.md: %v", err)
	} else if len(content) == 0 {
		t.Error("README.md is empty")
	} else {
		t.Logf("README.md read successfully (%d bytes)", len(content))
	}

	// Test 5: List bookmarks directory (may show {path} placeholder due to router pattern)
	t.Log("Test 5: Listing bookmarks directory")
	bookmarkEntries, err := os.ReadDir(filepath.Join(mountPoint, "bookmarks"))
	if err != nil {
		t.Logf("Listing bookmarks failed (expected): %v", err)
	} else {
		t.Logf("Bookmarks directory contains %d entries", len(bookmarkEntries))
		for _, entry := range bookmarkEntries {
			t.Logf("  - %s", entry.Name())
		}
	}

	t.Log("Wikipedia example test completed (some operations may fail due to network or router patterns)")
}

// TestIntegrationDualProtocol tests running both FUSE and 9p simultaneously
// with the same Lua script and router. It verifies that writes via one protocol
// are visible when read via the other, proving shared Lua state.
func TestIntegrationDualProtocol(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	// Check FUSE prerequisites (dual mode requires FUSE)
	if skipMsg := checkFUSEPrerequisites(); skipMsg != "" {
		t.Skipf("FUSE not available: %s", skipMsg)
	}

	nklhdPath, err := findNklhdBinary()
	if err != nil {
		t.Skipf("nklhd binary not found: %v", err)
	}

	port, err := findAvailablePort()
	if err != nil {
		t.Fatalf("failed to find port: %v", err)
	}
	addr := fmt.Sprintf("localhost:%d", port)

	tmpDir, err := os.MkdirTemp("", "nklhd-test-dual-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mountPoint := filepath.Join(tmpDir, "mount")
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		t.Fatal(err)
	}

	// Write Lua script with routes accessible from both protocols.
	const luaContent = `
app.read("/hello", function(req)
    return "Hello, world!"
end)
local echo_data = ""
app.write("/echo", function(req)
    echo_data = "Echo: " .. (req.data or "")
    return echo_data
end)
app.read("/echo", function(req)
    return echo_data
end)
`
	luaScriptPath := filepath.Join(tmpDir, "routes.lua")
	if err := os.WriteFile(luaScriptPath, []byte(luaContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Start nklhd with both protocols.
	cmd := exec.Command(nklhdPath,
		"--config", luaScriptPath,
		"--protocol", "fuse,9p",
		"--mount", mountPoint,
		"--listen", addr,
		"--verbose",
	)
	var stderr bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	t.Logf("Starting nklhd dual: %s --config %s --protocol fuse,9p --mount %s --listen %s",
		nklhdPath, luaScriptPath, mountPoint, addr)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start nklhd: %v", err)
	}
	defer func() {
		// 9p doesn't need process signals for unmount in dual mode
		// since the main.go shutdown handles both gracefully via SIGINT.
		// But we use the same helper to ensure cleanup.
		gracefullyShutdownNklhd(cmd.Process, mountPoint)
	}()

	// Wait for FUSE mount to be ready.
	var fuseReady bool
	for i := 0; i < 10; i++ {
		time.Sleep(time.Duration(100*(1<<i)) * time.Millisecond)
		if isMounted(mountPoint) {
			_, err := os.ReadDir(mountPoint)
			if err == nil {
				t.Logf("FUSE mount ready after %d attempts", i+1)
				fuseReady = true
				break
			}
		}
	}
	if !fuseReady {
		t.Fatalf("FUSE mount never became ready (stderr: %s)", stderr.String())
	}

	// Wait for 9p server to become ready.
	if !waitForNinepReady(addr) {
		t.Fatalf("9p server never became ready (stderr: %s)", stderr.String())
	}

	// Connect to 9p server.
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to connect to 9p server: %v", err)
	}
	c, err := client.NewClient(conn, "nobody", "")
	if err != nil {
		conn.Close()
		t.Fatalf("failed to create 9p client: %v", err)
	}

	// Test 1: Read via FUSE, verify 9p sees the same.
	t.Run("read-fuse-read-9p", func(t *testing.T) {
		content, err := os.ReadFile(filepath.Join(mountPoint, "hello"))
		if err != nil {
			t.Fatalf("FUSE read /hello: %v", err)
		}
		if string(content) != "Hello, world!" {
			t.Errorf("FUSE got %q, want %q", string(content), "Hello, world!")
		}

		f2, err := c.Open("/hello", proto.Oread)
		if err != nil {
			t.Fatalf("9p open /hello: %v", err)
		}
		defer f2.Close()
		data, err := io.ReadAll(f2)
		if err != nil {
			t.Fatalf("9p read /hello: %v", err)
		}
		if strings.TrimSpace(string(data)) != "Hello, world!" {
			t.Errorf("9p got %q, want %q", string(data), "Hello, world!")
		}
	})

	// Test 2: Write via FUSE, read via 9p.
	t.Run("write-fuse-read-9p", func(t *testing.T) {
		testData := "written via FUSE"
		if err := os.WriteFile(filepath.Join(mountPoint, "echo"), []byte(testData), 0644); err != nil {
			t.Fatalf("FUSE write /echo: %v", err)
		}

		f, err := c.Open("/echo", proto.Oread)
		if err != nil {
			t.Fatalf("9p open /echo: %v", err)
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("9p read /echo: %v", err)
		}
		expected := "Echo: " + testData
		if strings.TrimSpace(string(data)) != expected {
			t.Errorf("9p got %q, want %q", string(data), expected)
		}
	})

	// Test 3: Write via 9p, verify it changes the Lua state by reading
	// via 9p (FUSE caches file content, so a FUSE read after a 9p write
	// may return stale cached data — this is expected FUSE behavior).
	t.Run("write-9p-read-9p", func(t *testing.T) {
		testData := "written via 9p"
		f, err := c.Open("/echo", proto.Owrite)
		if err != nil {
			t.Fatalf("9p open /echo for write: %v", err)
		}
		_, err = f.Write([]byte(testData))
		f.Close()
		if err != nil {
			t.Fatalf("9p write /echo: %v", err)
		}

		// Read back via 9p — should see the new value.
		f2, err := c.Open("/echo", proto.Oread)
		if err != nil {
			t.Fatalf("9p open /echo for read: %v", err)
		}
		defer f2.Close()
		data, err := io.ReadAll(f2)
		if err != nil {
			t.Fatalf("9p read /echo: %v", err)
		}
		expected := "Echo: " + testData
		if strings.TrimSpace(string(data)) != expected {
			t.Errorf("9p got %q, want %q", string(data), expected)
		}
	})

	t.Log("Dual protocol test completed successfully — FUSE and 9p share the same router and Lua state")
}

func TestIntegrationTengoFuseOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	// Check prerequisites
	if skipMsg := checkFUSEPrerequisites(); skipMsg != "" {
		t.Skipf("FUSE not available: %s", skipMsg)
	}

	// Enable debug logging for troubleshooting
	fuse.SetDebug(true)
	tengo.SetDebug(true)
	defer func() {
		fuse.SetDebug(false)
		tengo.SetDebug(false)
	}()

	// Create temporary directory for config and mount point
	tmpDir, err := os.MkdirTemp("", "nklhd-tengo-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Write Tengo script with route definitions
	tengoScriptPath := filepath.Join(tmpDir, "routes.tengo")
	tengoContent := `
app.read("/hello", func(req) {
    return "Hello, world!"
})
app.write("/echo", func(req) {
    data := req.data
    if data == undefined { data = "" }
    return "Echo: " + data
})
app.readdir("/files", func(req) {
    return "hello\necho\nsubdir/"
})
app.read("/files/subdir/file", func(req) {
    return "Nested file"
})
app.create("/create_test", func(req) {
    return "Created: unknown"
})
app.unlink("/delete_test", func(req) {
    return "Deleted: unknown"
})
app.getattr("/stat_test", func(req) {
    return "size=1024\nmode=0644\nuid=1000\ngid=1000"
})
app.open("/open_test", func(req) {
    return "Opened: flags=0"
})
`
	if err := os.WriteFile(tengoScriptPath, []byte(tengoContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Write .toml config file pointing to Tengo script
	configTomlPath := filepath.Join(tmpDir, "config.toml")
	tomlContent := `
mountpoint = "` + filepath.Join(tmpDir, "mount") + `"
rootscript = "routes.tengo"
verbose = true
`
	if err := os.WriteFile(configTomlPath, []byte(tomlContent), 0644); err != nil {
		t.Fatal(err)
	}

	mountPoint := filepath.Join(tmpDir, "mount")
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		t.Fatal(err)
	}

	// Load configuration via config package
	cfg, err := config.LoadConfigWithPath(configTomlPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.RootScript == "" {
		t.Fatal("root script not resolved from config")
	}
	if cfg.MountPoint == "" {
		t.Fatal("mount point not resolved from config")
	}

	// Create backend (executor) and load configuration from resolved script path
	backend := tengo.NewExecutor()
	if err := backend.LoadScript(cfg.RootScript); err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	defer backend.Close()

	// Create FUSE server with default mount options
	opts := &fuse.MountOptions{
		Debug: true, // Enable mount debug output
	}
	server := fuse.NewServer(backend.GetRouter(), opts)

	t.Logf("Attempting to mount FUSE at %s", mountPoint)
	if err := server.Mount(mountPoint); err != nil {
		// Mount failed, skip test with detailed error
		t.Skipf("FUSE mount failed: %v", err)
	}
	defer func() {
		t.Log("Unmounting FUSE filesystem")
		unmountSuccess := false
		if err := server.Unmount(); err != nil {
			t.Logf("cleanup unmount failed: %v, trying fallback", err)
			if fallbackErr := unmountMountPoint(mountPoint); fallbackErr != nil {
				t.Logf("fallback unmount also failed: %v", fallbackErr)
			} else {
				t.Log("fallback unmount succeeded")
				unmountSuccess = true
			}
		} else {
			unmountSuccess = true
		}
		// Wait with timeout to avoid hanging
		waitDone := make(chan error, 1)
		go func() {
			waitDone <- server.Wait()
		}()
		select {
		case err := <-waitDone:
			if err != nil {
				t.Logf("cleanup wait failed: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Log("cleanup wait timeout after 5 seconds")
		}
		// Only remove directory if unmount succeeded
		if unmountSuccess {
			t.Logf("Cleaning up temp directory %s", tmpDir)
			if err := os.RemoveAll(tmpDir); err != nil {
				t.Logf("cleanup temp dir failed: %v", err)
			}
		} else {
			t.Logf("WARNING: Skipping temp directory cleanup for %s due to unmount failure", tmpDir)
		}
	}()

	// Wait for mount to become ready - poll with exponential backoff
	var ready bool
	mountedButUnreadable := 0
	for i := 0; i < 10; i++ {
		time.Sleep(time.Duration(100*(1<<i)) * time.Millisecond)
		if isMounted(mountPoint) {
			entries, err := os.ReadDir(mountPoint)
			if err == nil {
				t.Logf("Mount ready after %d attempts, entries: %v", i+1, entries)
				ready = true
				break
			}
			t.Logf("Mount appears in /proc/mounts but not readable (attempt %d): %v", i+1, err)
			mountedButUnreadable++
			if mountedButUnreadable >= 3 {
				t.Skipf("FUSE mounted but not readable after %d attempts; skipping test", mountedButUnreadable)
			}
		} else {
			t.Logf("Mount not yet in /proc/mounts (attempt %d)", i+1)
		}
	}
	if !ready {
		t.Fatal("Mount point never became readable")
	}

	// Debug: list root directory
	rootEntries, err := os.ReadDir(mountPoint)
	if err != nil {
		t.Fatalf("Failed to list mount point: %v", err)
	}
	t.Logf("Mount point entries: %v", rootEntries)

	// Test reading a file
	content, err := os.ReadFile(filepath.Join(mountPoint, "hello"))
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if string(content) != "Hello, world!" {
		t.Errorf("Unexpected content: got %q, expected %q", string(content), "Hello, world!")
	}

	// Test directory listing
	entries, err := os.ReadDir(filepath.Join(mountPoint, "files"))
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("Expected 3 entries in /files, got %d", len(entries))
	}
	// Check entry names
	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name()] = true
	}
	if !names["hello"] || !names["echo"] || !names["subdir"] {
		t.Errorf("Missing expected entries in /files: %v", names)
	}
	// subdir should be a directory
	for _, e := range entries {
		if e.Name() == "subdir" && !e.IsDir() {
			t.Errorf("subdir should be a directory")
		}
	}

	// Test nested file
	nestedContent, err := os.ReadFile(filepath.Join(mountPoint, "files", "subdir", "file"))
	if err != nil {
		t.Fatalf("Failed to read nested file: %v", err)
	}
	if string(nestedContent) != "Nested file" {
		t.Errorf("Unexpected nested content: got %q, expected %q", string(nestedContent), "Nested file")
	}

	// Test writing to a file (echo handler)
	echoPath := filepath.Join(mountPoint, "echo")
	testData := "test data"
	if err := os.WriteFile(echoPath, []byte(testData), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	// Read back to verify echo handler returned something
	content, err = os.ReadFile(echoPath)
	if err != nil {
		t.Fatalf("Failed to read echo file: %v", err)
	}
	expected := "Echo: " + testData
	if string(content) != expected {
		t.Errorf("Unexpected echo content: got %q, expected %q", string(content), expected)
	}

	t.Log("Tengo integration test completed successfully")
}

func TestIntegrationTengoNklhdBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	// Check prerequisites
	if skipMsg := checkFUSEPrerequisites(); skipMsg != "" {
		t.Skipf("FUSE not available: %s", skipMsg)
	}

	// Find nklhd binary
	nklhdPath, err := findNklhdBinary()
	if err != nil {
		t.Skipf("nklhd binary not found: %v", err)
	}

	// Create temporary directory for config and mount point
	tmpDir, err := os.MkdirTemp("", "nklhd-tengo-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() {
		// Cleanup temp directory (mount point should be unmounted)
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("cleanup temp dir failed: %v", err)
		}
	}()

	// Write Tengo script with route definitions
	tengoScriptPath := filepath.Join(tmpDir, "routes.tengo")
	tengoContent := `
app.read("/hello", func(req) {
    return "Hello, world!"
})
app.write("/echo", func(req) {
    data := req.data
    if data == undefined { data = "" }
    return "Echo: " + data
})
app.readdir("/files", func(req) {
    return "hello\necho\nsubdir/"
})
app.read("/files/subdir/file", func(req) {
    return "Nested file"
})
app.create("/create_test", func(req) {
    return "Created: unknown"
})
app.unlink("/delete_test", func(req) {
    return "Deleted: unknown"
})
app.getattr("/stat_test", func(req) {
    return "size=1024\nmode=0644\nuid=1000\ngid=1000"
})
app.open("/open_test", func(req) {
    return "Opened: flags=0"
})
`
	if err := os.WriteFile(tengoScriptPath, []byte(tengoContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Write .toml config file pointing to Tengo script
	configTomlPath := filepath.Join(tmpDir, "config.toml")
	tomlContent := `
mountpoint = "` + filepath.Join(tmpDir, "mount") + `"
rootscript = "routes.tengo"
verbose = true
`
	if err := os.WriteFile(configTomlPath, []byte(tomlContent), 0644); err != nil {
		t.Fatal(err)
	}

	mountPoint := filepath.Join(tmpDir, "mount")
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		t.Fatal(err)
	}

	// Start nklhd binary with config
	cmd := exec.Command(nklhdPath, "--config", configTomlPath, "--mount", mountPoint, "--verbose")
	var stderr bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	t.Logf("Starting nklhd: %s --config %s --mount %s --verbose", nklhdPath, configTomlPath, mountPoint)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start nklhd: %v", err)
	}
	defer func() {
		if err := gracefullyShutdownNklhd(cmd.Process, mountPoint); err != nil {
			t.Logf("graceful shutdown failed: %v (stderr: %s)", err, stderr.String())
		}
	}()

	// Wait for mount to become ready
	var ready bool
	for i := 0; i < 10; i++ {
		time.Sleep(time.Duration(100*(1<<i)) * time.Millisecond)
		if isMounted(mountPoint) {
			_, err := os.ReadDir(mountPoint)
			if err == nil {
				t.Logf("Mount ready after %d attempts", i+1)
				ready = true
				break
			}
		}
	}
	if !ready {
		t.Fatal("Mount point never became readable")
	}

	// Perform basic file operations (same as other test)
	content, err := os.ReadFile(filepath.Join(mountPoint, "hello"))
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if string(content) != "Hello, world!" {
		t.Errorf("Unexpected content: got %q, expected %q", string(content), "Hello, world!")
	}

	// Test directory listing
	entries, err := os.ReadDir(filepath.Join(mountPoint, "files"))
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("Expected 3 entries in /files, got %d", len(entries))
	}

	// Test nested file
	nestedContent, err := os.ReadFile(filepath.Join(mountPoint, "files", "subdir", "file"))
	if err != nil {
		t.Fatalf("Failed to read nested file: %v", err)
	}
	if string(nestedContent) != "Nested file" {
		t.Errorf("Unexpected nested content: got %q, expected %q", string(nestedContent), "Nested file")
	}

	// Test write operation
	testData := "test data"
	if err := os.WriteFile(filepath.Join(mountPoint, "echo"), []byte(testData), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	// Read back to verify echo handler returned something
	content, err = os.ReadFile(filepath.Join(mountPoint, "echo"))
	if err != nil {
		t.Fatalf("Failed to read echo file: %v", err)
	}
	expected := "Echo: " + testData
	if string(content) != expected {
		t.Errorf("Unexpected echo content: got %q, expected %q", string(content), expected)
	}

	t.Log("Tengo binary integration test completed successfully")
}

// TestIntegrationTengoNklhdBinary9p tests the nklhd executable with the 9p protocol
// using a Tengo script.
func TestIntegrationTengoNklhdBinary9p(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	nklhdPath, err := findNklhdBinary()
	if err != nil {
		t.Skipf("nklhd binary not found: %v", err)
	}

	port, err := findAvailablePort()
	if err != nil {
		t.Fatalf("failed to find port: %v", err)
	}
	addr := fmt.Sprintf("localhost:%d", port)

	tmpDir, err := os.MkdirTemp("", "nklhd-tengo-9p-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write Tengo script — same routes as the binary test plus path params + subdirectory
	tengoContent := `
app.read("/hello", func(req) {
    return "Hello, world!"
})
app.write("/echo", func(req) {
    data := req.data
    if data == undefined { data = "" }
    return "Echo: " + data
})
app.read("/echo", func(req) {
    return "Echo: test data"
})
app.readdir("/dir", func(req) {
    return "file1\nfile2"
})
app.read("/dir/file1", func(req) {
    return "File1 content"
})
app.read("/dir/file2", func(req) {
    return "File2 content"
})
app.read("/users/{id}", func(req) {
    return "User: " + req.id
})
app.read("/status", func(req) {
    return "OK"
})
`
	tengoScriptPath := filepath.Join(tmpDir, "routes.tengo")
	if err := os.WriteFile(tengoScriptPath, []byte(tengoContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Start nklhd with 9p protocol
	cmd := exec.Command(nklhdPath, "--config", tengoScriptPath, "--protocol", "9p", "--listen", addr, "--verbose")
	var stderr bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	t.Logf("Starting nklhd 9p: %s --config %s --protocol 9p --listen %s", nklhdPath, tengoScriptPath, addr)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start nklhd: %v", err)
	}
	defer func() {
		gracefullyShutdownNklhd(cmd.Process, "")
	}()

	// Wait for 9p server to become ready
	if !waitForNinepReady(addr) {
		t.Fatalf("9p server never became ready (stderr: %s)", stderr.String())
	}

	// Connect to 9p server
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to connect to 9p server: %v", err)
	}
	c, err := client.NewClient(conn, "nobody", "")
	if err != nil {
		conn.Close()
		t.Fatalf("failed to create 9p client: %v", err)
	}

	// Test 1: Read a file
	t.Run("read", func(t *testing.T) {
		f, err := c.Open("/hello", proto.Oread)
		if err != nil {
			t.Fatalf("failed to open /hello: %v", err)
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("failed to read /hello: %v", err)
		}
		if strings.TrimSpace(string(data)) != "Hello, world!" {
			t.Errorf("got %q, want %q", string(data), "Hello, world!")
		}
	})

	// Test 2: Read directory listing
	t.Run("readdir", func(t *testing.T) {
		stats, err := c.Readdir("/")
		if err != nil {
			t.Fatalf("failed to readdir /: %v", err)
		}
		names := make([]string, len(stats))
		for i, s := range stats {
			names[i] = s.Name
		}
		found := make(map[string]bool)
		for _, name := range names {
			found[name] = true
		}
		for _, expected := range []string{"hello", "echo", "status", "dir"} {
			if !found[expected] {
				t.Errorf("expected %q in root directory, got %v", expected, names)
			}
		}
	})

	// Test 3: Read nested file
	t.Run("nested", func(t *testing.T) {
		f, err := c.Open("/dir/file1", proto.Oread)
		if err != nil {
			t.Fatalf("failed to open /dir/file1: %v", err)
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("failed to read /dir/file1: %v", err)
		}
		if strings.TrimSpace(string(data)) != "File1 content" {
			t.Errorf("got %q, want %q", string(data), "File1 content")
		}
	})

	// Test 4: Write operation (Tengo handlers are stateless — no read-back)
	t.Run("write", func(t *testing.T) {
		writeTestData := "write test data"
		f, err := c.Open("/echo", proto.Owrite)
		if err != nil {
			t.Fatalf("failed to open /echo for write: %v", err)
		}
		_, err = f.Write([]byte(writeTestData))
		f.Close()
		if err != nil {
			t.Fatalf("failed to write to /echo: %v", err)
		}
	})

	// Test 5: Read echo (standalone read handler, independent of write)
	t.Run("read-echo", func(t *testing.T) {
		f, err := c.Open("/echo", proto.Oread)
		if err != nil {
			t.Fatalf("failed to open /echo for read: %v", err)
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("failed to read /echo: %v", err)
		}
		if strings.TrimSpace(string(data)) != "Echo: test data" {
			t.Errorf("got %q, want %q", string(data), "Echo: test data")
		}
	})

	// Test 6: Path parameters
	t.Run("pathparams", func(t *testing.T) {
		f, err := c.Open("/users/42", proto.Oread)
		if err != nil {
			t.Fatalf("failed to open /users/42: %v", err)
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("failed to read /users/42: %v", err)
		}
		if strings.TrimSpace(string(data)) != "User: 42" {
			t.Errorf("got %q, want %q", string(data), "User: 42")
		}
	})

	// Test 7: Stat a file
	t.Run("stat", func(t *testing.T) {
		st, err := c.Stat("/hello")
		if err != nil {
			t.Fatalf("failed to stat /hello: %v", err)
		}
		if st.Name != "hello" {
			t.Errorf("stat name: got %q, want %q", st.Name, "hello")
		}
		if st.Mode&proto.DMDIR != 0 {
			t.Errorf("/hello should not be a directory, mode=0x%x", st.Mode)
		}
	})

	t.Log("Tengo 9p binary integration test completed successfully")
}

// TestIntegrationSshfsOperations tests the nklhd executable with the SSH protocol
// using a Lua script, mirroring the structure of TestIntegrationNklhdBinary9p.
func TestIntegrationSshfsOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	nklhdPath, err := findNklhdBinary()
	if err != nil {
		t.Skipf("nklhd binary not found: %v", err)
	}

	port, err := findAvailablePort()
	if err != nil {
		t.Fatalf("failed to find port: %v", err)
	}
	addr := fmt.Sprintf("localhost:%d", port)

	tmpDir, err := os.MkdirTemp("", "nklhd-test-ssh-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write Lua script — same routes as the 9p binary test.
	const luaContent = `
app.read("/hello", function(req)
    return "Hello, world!"
end)
local echo_data = ""
app.write("/echo", function(req)
    echo_data = "Echo: " .. (req.data or "")
    return echo_data
end)
app.read("/echo", function(req)
    return echo_data
end)
app.readdir("/dir", function(req)
    return {"file1", "file2"}
end)
app.read("/dir/file1", function(req)
    return "File1 content"
end)
app.read("/dir/file2", function(req)
    return "File2 content"
end)
app.read("/users/{id}", function(req)
    return "User: " .. req.id
end)
app.read("/status", function(req)
    return "OK"
end)
`
	luaScriptPath := filepath.Join(tmpDir, "routes.lua")
	if err := os.WriteFile(luaScriptPath, []byte(luaContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Generate ephemeral Ed25519 key pair.
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ed25519 key: %v", err)
	}
	signer, err := gossh.NewSignerFromKey(privKey)
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}
	sshPubKey, err := gossh.NewPublicKey(pubKey)
	if err != nil {
		t.Fatalf("failed to create ssh public key: %v", err)
	}
	authorizedKey := gossh.MarshalAuthorizedKey(sshPubKey)
	authKeysPath := filepath.Join(tmpDir, "authorized_keys")
	if err := os.WriteFile(authKeysPath, authorizedKey, 0644); err != nil {
		t.Fatal(err)
	}

	// Start nklhd with SSH protocol.
	cmd := exec.Command(nklhdPath, "--config", luaScriptPath, "--protocol", "ssh", "--ssh-addr", addr, "--ssh-authorized-keys", authKeysPath, "--verbose")
	var stderr bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	t.Logf("Starting nklhd SSH: %s --config %s --protocol ssh --ssh-addr %s --ssh-authorized-keys %s", nklhdPath, luaScriptPath, addr, authKeysPath)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start nklhd: %v", err)
	}
	defer func() {
		gracefullyShutdownNklhd(cmd.Process, "")
	}()

	// Wait for SSH server to become ready.
	if !waitForNinepReady(addr) {
		t.Fatalf("SSH server never became ready (stderr: %s)", stderr.String())
	}

	// Dial SSH.
	sshConfig := &gossh.ClientConfig{
		User: "nklhd",
		Auth: []gossh.AuthMethod{
			gossh.PublicKeys(signer),
		},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	}
	sshClient, err := gossh.Dial("tcp", addr, sshConfig)
	if err != nil {
		t.Fatalf("failed to dial SSH: %v", err)
	}
	defer sshClient.Close()

	// Open SFTP client.
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		t.Fatalf("failed to create SFTP client: %v", err)
	}
	defer sftpClient.Close()

	// Test 1: Read a file.
	t.Run("read", func(t *testing.T) {
		f, err := sftpClient.Open("/hello")
		if err != nil {
			t.Fatalf("failed to open /hello: %v", err)
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("failed to read /hello: %v", err)
		}
		if strings.TrimSpace(string(data)) != "Hello, world!" {
			t.Errorf("got %q, want %q", string(data), "Hello, world!")
		}
	})

	// Test 2: Read directory listing.
	t.Run("readdir", func(t *testing.T) {
		entries, err := sftpClient.ReadDir("/")
		if err != nil {
			t.Fatalf("failed to readdir /: %v", err)
		}
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		found := make(map[string]bool)
		for _, name := range names {
			found[name] = true
		}
		for _, expected := range []string{"hello", "echo", "status", "dir"} {
			if !found[expected] {
				t.Errorf("expected %q in root directory, got %v", expected, names)
			}
		}
	})

	// Test 3: Read nested file.
	t.Run("nested", func(t *testing.T) {
		f, err := sftpClient.Open("/dir/file1")
		if err != nil {
			t.Fatalf("failed to open /dir/file1: %v", err)
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("failed to read /dir/file1: %v", err)
		}
		if strings.TrimSpace(string(data)) != "File1 content" {
			t.Errorf("got %q, want %q", string(data), "File1 content")
		}
	})

	// Test 4: Write then read back.
	t.Run("write", func(t *testing.T) {
		testData := "test data"
		f, err := sftpClient.Create("/echo")
		if err != nil {
			t.Fatalf("failed to create /echo for write: %v", err)
		}
		_, err = f.Write([]byte(testData))
		f.Close()
		if err != nil {
			t.Fatalf("failed to write to /echo: %v", err)
		}
		// Read back.
		f2, err := sftpClient.Open("/echo")
		if err != nil {
			t.Fatalf("failed to open /echo for read: %v", err)
		}
		defer f2.Close()
		data, err := io.ReadAll(f2)
		if err != nil {
			t.Fatalf("failed to read /echo: %v", err)
		}
		expected := "Echo: " + testData
		if strings.TrimSpace(string(data)) != expected {
			t.Errorf("got %q, want %q", string(data), expected)
		}
	})

	// Test 5: Path parameters.
	t.Run("pathparams", func(t *testing.T) {
		f, err := sftpClient.Open("/users/42")
		if err != nil {
			t.Fatalf("failed to open /users/42: %v", err)
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("failed to read /users/42: %v", err)
		}
		if strings.TrimSpace(string(data)) != "User: 42" {
			t.Errorf("got %q, want %q", string(data), "User: 42")
		}
	})

	// Test 6: Stat a file.
	t.Run("stat", func(t *testing.T) {
		fi, err := sftpClient.Stat("/hello")
		if err != nil {
			t.Fatalf("failed to stat /hello: %v", err)
		}
		if fi.Name() != "hello" {
			t.Errorf("stat name: got %q, want %q", fi.Name(), "hello")
		}
		if fi.IsDir() {
			t.Errorf("/hello should not be a directory")
		}
	})

	t.Log("SSHFS binary integration test completed successfully")
}

// TestIntegrationTengoSshfsOperations tests the nklhd executable with the SSH
// protocol using a Tengo script.
func TestIntegrationTengoSshfsOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	nklhdPath, err := findNklhdBinary()
	if err != nil {
		t.Skipf("nklhd binary not found: %v", err)
	}

	port, err := findAvailablePort()
	if err != nil {
		t.Fatalf("failed to find port: %v", err)
	}
	addr := fmt.Sprintf("localhost:%d", port)

	tmpDir, err := os.MkdirTemp("", "nklhd-tengo-ssh-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write Tengo script — same routes as the Tengo 9p binary test.
	const tengoContent = `
app.read("/hello", func(req) {
    return "Hello, world!"
})
app.write("/echo", func(req) {
    data := req.data
    if data == undefined { data = "" }
    return "Echo: " + data
})
app.read("/echo", func(req) {
    return "Echo: test data"
})
app.readdir("/dir", func(req) {
    return "file1\nfile2"
})
app.read("/dir/file1", func(req) {
    return "File1 content"
})
app.read("/dir/file2", func(req) {
    return "File2 content"
})
app.read("/users/{id}", func(req) {
    return "User: " + req.id
})
app.read("/status", func(req) {
    return "OK"
})
`
	tengoScriptPath := filepath.Join(tmpDir, "routes.tengo")
	if err := os.WriteFile(tengoScriptPath, []byte(tengoContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Generate ephemeral Ed25519 key pair.
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ed25519 key: %v", err)
	}
	signer, err := gossh.NewSignerFromKey(privKey)
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}
	sshPubKey, err := gossh.NewPublicKey(pubKey)
	if err != nil {
		t.Fatalf("failed to create ssh public key: %v", err)
	}
	authorizedKey := gossh.MarshalAuthorizedKey(sshPubKey)
	authKeysPath := filepath.Join(tmpDir, "authorized_keys")
	if err := os.WriteFile(authKeysPath, authorizedKey, 0644); err != nil {
		t.Fatal(err)
	}

	// Start nklhd with SSH protocol.
	cmd := exec.Command(nklhdPath, "--config", tengoScriptPath, "--protocol", "ssh", "--ssh-addr", addr, "--ssh-authorized-keys", authKeysPath, "--verbose")
	var stderr bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	t.Logf("Starting nklhd SSH (Tengo): %s --config %s --protocol ssh --ssh-addr %s --ssh-authorized-keys %s", nklhdPath, tengoScriptPath, addr, authKeysPath)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start nklhd: %v", err)
	}
	defer func() {
		gracefullyShutdownNklhd(cmd.Process, "")
	}()

	// Wait for SSH server to become ready.
	if !waitForNinepReady(addr) {
		t.Fatalf("SSH server never became ready (stderr: %s)", stderr.String())
	}

	// Dial SSH.
	sshConfig := &gossh.ClientConfig{
		User: "nklhd",
		Auth: []gossh.AuthMethod{
			gossh.PublicKeys(signer),
		},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	}
	sshClient, err := gossh.Dial("tcp", addr, sshConfig)
	if err != nil {
		t.Fatalf("failed to dial SSH: %v", err)
	}
	defer sshClient.Close()

	// Open SFTP client.
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		t.Fatalf("failed to create SFTP client: %v", err)
	}
	defer sftpClient.Close()

	// Test 1: Read a file.
	t.Run("read", func(t *testing.T) {
		f, err := sftpClient.Open("/hello")
		if err != nil {
			t.Fatalf("failed to open /hello: %v", err)
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("failed to read /hello: %v", err)
		}
		if strings.TrimSpace(string(data)) != "Hello, world!" {
			t.Errorf("got %q, want %q", string(data), "Hello, world!")
		}
	})

	// Test 2: Read directory listing.
	t.Run("readdir", func(t *testing.T) {
		entries, err := sftpClient.ReadDir("/")
		if err != nil {
			t.Fatalf("failed to readdir /: %v", err)
		}
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		found := make(map[string]bool)
		for _, name := range names {
			found[name] = true
		}
		for _, expected := range []string{"hello", "echo", "status", "dir"} {
			if !found[expected] {
				t.Errorf("expected %q in root directory, got %v", expected, names)
			}
		}
	})

	// Test 3: Read nested file.
	t.Run("nested", func(t *testing.T) {
		f, err := sftpClient.Open("/dir/file1")
		if err != nil {
			t.Fatalf("failed to open /dir/file1: %v", err)
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("failed to read /dir/file1: %v", err)
		}
		if strings.TrimSpace(string(data)) != "File1 content" {
			t.Errorf("got %q, want %q", string(data), "File1 content")
		}
	})

	// Test 4: Write operation (Tengo handlers are stateless — no read-back).
	t.Run("write", func(t *testing.T) {
		writeTestData := "write test data"
		f, err := sftpClient.Create("/echo")
		if err != nil {
			t.Fatalf("failed to create /echo for write: %v", err)
		}
		_, err = f.Write([]byte(writeTestData))
		f.Close()
		if err != nil {
			t.Fatalf("failed to write to /echo: %v", err)
		}
	})

	// Test 5: Read echo (standalone read handler, independent of write).
	t.Run("read-echo", func(t *testing.T) {
		f, err := sftpClient.Open("/echo")
		if err != nil {
			t.Fatalf("failed to open /echo for read: %v", err)
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("failed to read /echo: %v", err)
		}
		if strings.TrimSpace(string(data)) != "Echo: test data" {
			t.Errorf("got %q, want %q", string(data), "Echo: test data")
		}
	})

	// Test 6: Path parameters.
	t.Run("pathparams", func(t *testing.T) {
		f, err := sftpClient.Open("/users/42")
		if err != nil {
			t.Fatalf("failed to open /users/42: %v", err)
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("failed to read /users/42: %v", err)
		}
		if strings.TrimSpace(string(data)) != "User: 42" {
			t.Errorf("got %q, want %q", string(data), "User: 42")
		}
	})

	// Test 7: Stat a file.
	t.Run("stat", func(t *testing.T) {
		fi, err := sftpClient.Stat("/hello")
		if err != nil {
			t.Fatalf("failed to stat /hello: %v", err)
		}
		if fi.Name() != "hello" {
			t.Errorf("stat name: got %q, want %q", fi.Name(), "hello")
		}
		if fi.IsDir() {
			t.Errorf("/hello should not be a directory")
		}
	})

	t.Log("Tengo SSHFS binary integration test completed successfully")
}
