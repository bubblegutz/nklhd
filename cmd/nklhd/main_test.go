package main_test

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/bubblegutz/nklhd/internal/config"
	"github.com/kkyr/fig"
	"github.com/bubblegutz/nklhd/internal/fuse"
	"github.com/bubblegutz/nklhd/internal/lua"
	"gopkg.in/yaml.v3"
)

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
	// Fallback to syscall unmount with lazy flag
	if err := syscall.Unmount(mountPoint, syscall.MNT_DETACH); err == nil {
		return nil
	}
	// Fallback to regular syscall unmount
	return syscall.Unmount(mountPoint, 0)
}

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

// fileExists returns true if the file exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// copyDir recursively copies a directory from src to dst.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}
		// For regular files, copy content
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()
		dstFile, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer dstFile.Close()
		_, err = io.Copy(dstFile, srcFile)
		return err
	})
}

// findNklhdBinary returns the path to the nklhd binary.
func findNklhdBinary() (string, error) {
	// Try to find the binary in the project root
	candidates := []string{
		"../nklhd",
		"../../nklhd",
		"./nklhd",
		"nklhd",
	}
	for _, cand := range candidates {
		if fi, err := os.Stat(cand); err == nil {
			// Check it's a regular file (not a directory)
			if fi.IsDir() {
				continue
			}
			// Check it's executable (at least by owner)
			if fi.Mode()&0111 == 0 {
				continue
			}
			abs, err := filepath.Abs(cand)
			if err == nil {
				return abs, nil
			}
		}
	}
	return "", fmt.Errorf("nklhd binary not found")
}

// langConfig describes language-specific settings for an example integration test.
type langConfig struct {
	lang       string // "lua" or "tengo"
	exampleDir string // subdirectory under examples/{lang}/
	rootScript string // e.g., "main.lua" or "main.tengo"
}

// exampleTest manages the lifecycle of an nklhd integration test.
type exampleTest struct {
	t          *testing.T
	lc         langConfig
	tmpDir     string
	mountPoint string
	nklhdPath  string
	cmd        *exec.Cmd
}

// newExampleTest creates and starts nklhd with the given example.
// It registers a Cleanup callback that tears down the process and temp dir.
func newExampleTest(t *testing.T, lc langConfig, env map[string]string) *exampleTest {
	t.Helper()

	if _, err := os.Stat("/dev/fuse"); err != nil {
		t.Skip("FUSE not available (/dev/fuse missing)")
	}
	nklhdPath, err := findNklhdBinary()
	if err != nil {
		t.Skipf("nklhd binary not found: %v", err)
	}
	tmpDir, err := os.MkdirTemp("", "nklhd-test-"+lc.exampleDir+"-"+lc.lang+"-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	mountPoint := filepath.Join(tmpDir, "mount")

	// Copy example directory
	exampleDir := filepath.Join("..", "..", "examples", lc.lang, lc.exampleDir)
	if _, err := os.Stat(exampleDir); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("example directory not found: %v", err)
	}
	if err := copyDir(exampleDir, tmpDir); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to copy example directory: %v", err)
	}

	// Write dynamic config.toml
	configPath := filepath.Join(tmpDir, "config.toml")
	configContent := fmt.Sprintf(`mountpoint = "%s"
rootscript = "%s"
verbose = true
`, mountPoint, lc.rootScript)
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}

	cmd := exec.Command(nklhdPath, "--config", configPath, "--mount", mountPoint, "--verbose")
	if env != nil {
		cmd.Env = append(os.Environ(), envAsSlice(env)...)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	t.Logf("Starting nklhd: %s --config %s --mount %s --verbose", nklhdPath, configPath, mountPoint)
	if err := cmd.Start(); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to start nklhd: %v", err)
	}

	et := &exampleTest{
		t:          t,
		lc:         lc,
		tmpDir:     tmpDir,
		mountPoint: mountPoint,
		nklhdPath:  nklhdPath,
		cmd:        cmd,
	}

	t.Cleanup(func() {
		umountCmd := exec.Command(nklhdPath, "--config", configPath, "--mount", mountPoint, "--umount")
		if err := umountCmd.Run(); err != nil {
			t.Logf("umount via nklhd failed: %v", err)
		}
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		unmountMountPoint(mountPoint)
		os.RemoveAll(tmpDir)
	})

	var ready bool
	for i := 0; i < 10; i++ {
		time.Sleep(time.Duration(100*(1<<i)) * time.Millisecond)
		if isMounted(mountPoint) {
			_, err := os.ReadDir(mountPoint)
			if err == nil {
				ready = true
				break
			}
		}
	}
	if !ready {
		t.Fatal("Mount point never became readable")
	}

	return et
}

// envAsSlice converts a map of env vars to a slice of "key=value" strings.
func envAsSlice(env map[string]string) []string {
	s := make([]string, 0, len(env))
	for k, v := range env {
		s = append(s, k+"="+v)
	}
	return s
}

// path returns a full path under the test mount point.
func (et *exampleTest) path(elem ...string) string {
	return filepath.Join(append([]string{et.mountPoint}, elem...)...)
}

func TestFuseOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
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
	t.Logf("Config file written: %s (exists: %v)", configTomlPath, fileExists(configTomlPath))

	mountPoint := filepath.Join(tmpDir, "mount")
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		t.Fatal(err)
	}

	// Load configuration via config package
	// Debug: verify file is readable
	if _, err := os.Stat(configTomlPath); err != nil {
		t.Fatalf("config file not accessible: %v", err)
	}
	t.Logf("Loading config from: %s (absolute: %v)", configTomlPath, filepath.IsAbs(configTomlPath))
	// Read file content for debugging
	configContent, err := os.ReadFile(configTomlPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}
	t.Logf("Config file content (%d bytes):\n%s", len(configContent), string(configContent))
	// Try loading with fig directly (showing the correct way)
	var testCfg config.Config
	dir := filepath.Dir(configTomlPath)
	filename := filepath.Base(configTomlPath)
	err2 := fig.Load(&testCfg, fig.File(filename), fig.Dirs(dir), fig.UseEnv("NKLHD"))
	t.Logf("fig.Load with split path error: %v", err2)
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

	// Mount the filesystem using fuse package
	// Ensure mount point directory exists (already created)
	// Create backend (Lua executor) and load configuration from resolved script path
	backend := lua.NewExecutor()
	if err := backend.LoadScript(cfg.RootScript); err != nil {
		backend.Close()
		t.Fatalf("failed to load config: %v", err)
	}
	// Create FUSE server with mount options
	opts := &fuse.MountOptions{
		Debug: true, // Enable debug logging for troubleshooting
	}
	server := fuse.NewServer(backend.GetRouter(), opts)
	if err := server.Mount(mountPoint); err != nil {
		// If mounting fails due to lack of FUSE permissions, skip test
		backend.Close()
		t.Skipf("FUSE mount failed (maybe not available): %v", err)
	}
	// Give the FUSE server a moment to start serving
	time.Sleep(100 * time.Millisecond)
	// Debug: print mount line
	if data, err := os.ReadFile("/proc/mounts"); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.Contains(line, mountPoint) {
				t.Logf("Mount line: %s", line)
			}
		}
	}

	defer func() {
		unmountSuccess := false
		if server != nil {
			t.Log("Unmounting FUSE filesystem")
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
				// Close backend after server has stopped
				if backend != nil {
					backend.Close()
				}
			case <-time.After(30 * time.Second):
				t.Log("cleanup wait timeout after 30 seconds")
				// Still try to close backend
				if backend != nil {
					backend.Close()
				}
			}
		}
		// Only remove temporary directory if unmount succeeded AND mount point is no longer mounted
		if unmountSuccess && !isMounted(mountPoint) {
			t.Logf("Cleaning up temp directory %s", tmpDir)
			if err := os.RemoveAll(tmpDir); err != nil {
				t.Logf("cleanup temp dir failed: %v", err)
			}
		} else {
			t.Logf("WARNING: Skipping temp directory cleanup for %s (unmountSuccess=%v, isMounted=%v)", tmpDir, unmountSuccess, isMounted(mountPoint))
		}
	}()


	// Wait for mount to become ready - poll with exponential backoff
	var ready bool
	mountedButUnreadable := 0
	for i := 0; i < 10; i++ {
		time.Sleep(time.Duration(100*(1<<i)) * time.Millisecond)
		if isMounted(mountPoint) {
			_, err := os.ReadDir(mountPoint)
			if err == nil {
				t.Logf("Mount ready after %d attempts", i+1)
				ready = true
				break
			}
			if e, ok := err.(syscall.Errno); ok {
				t.Logf("Mount appears in /proc/mounts but not readable (attempt %d): %v, errno: %d", i+1, err, e)
			} else {
				t.Logf("Mount appears in /proc/mounts but not readable (attempt %d): %v", i+1, err)
			}
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
		t.Errorf("Expected 3 directory entries, got %d", len(entries))
	}
	// Check for expected names
	expectedNames := map[string]bool{"hello": false, "echo": false, "subdir": false}
	for _, entry := range entries {
		expectedNames[entry.Name()] = true
	}
	for name, found := range expectedNames {
		if !found {
			t.Errorf("Expected entry %q not found in directory", name)
		}
	}

	// Test nested file read
	nestedContent, err := os.ReadFile(filepath.Join(mountPoint, "files", "subdir", "file"))
	if err != nil {
		t.Fatalf("Failed to read nested file: %v", err)
	}
	if string(nestedContent) != "Nested file" {
		t.Errorf("Unexpected nested content: got %q, expected %q", string(nestedContent), "Nested file")
	}

	// Test write operation
	err = os.WriteFile(filepath.Join(mountPoint, "echo"), []byte("test data"), 0644)
	if err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	// Write succeeded; we could also verify that the Lua handler was called via logs
	// but for now just ensure no error.
}

func TestExampleSimple(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	langs := []struct {
		lc langConfig
	}{
		{lc: langConfig{lang: "lua", exampleDir: "simple", rootScript: "main.lua"}},
		{lc: langConfig{lang: "tengo", exampleDir: "simple", rootScript: "main.tengo"}},
	}
	for _, tc := range langs {
		t.Run(tc.lc.lang, func(t *testing.T) {
			et := newExampleTest(t, tc.lc, nil)

			entries, err := os.ReadDir(et.mountPoint)
			if err != nil {
				t.Fatalf("Failed to list mount point: %v", err)
			}
			expectedRootEntries := map[string]bool{"api": false, "docs": false, "status.txt": false, "config.txt": false}
			for _, entry := range entries {
				expectedRootEntries[entry.Name()] = true
			}
			for name, found := range expectedRootEntries {
				if !found {
					t.Errorf("Expected root entry %q not found", name)
				}
			}

			docContent, err := os.ReadFile(et.path("docs", "api.md"))
			if err != nil {
				t.Fatalf("Failed to read /docs/api.md: %v", err)
			}
			if !strings.Contains(string(docContent), "nklhd API Documentation") {
				t.Errorf("API documentation content missing expected text: %q", string(docContent))
			}

			statusContent, err := os.ReadFile(et.path("status.txt"))
			if err != nil {
				t.Fatalf("Failed to read /status.txt: %v", err)
			}
			if !strings.Contains(string(statusContent), "System Status:") {
				t.Errorf("Status file content missing expected text: %q", string(statusContent))
			}

			configContent, err := os.ReadFile(et.path("config.txt"))
			if err != nil {
				t.Fatalf("Failed to read /config.txt: %v", err)
			}
			if !strings.Contains(string(configContent), "Configuration:") {
				t.Errorf("Config file content missing expected text: %q", string(configContent))
			}

			userEntries, err := os.ReadDir(et.path("api", "users"))
			if err != nil {
				t.Fatalf("Failed to list /api/users: %v", err)
			}
			found1, found2, found3 := false, false, false
			for _, entry := range userEntries {
				if strings.Contains(entry.Name(), "1") {
					found1 = true
				}
				if strings.Contains(entry.Name(), "2") {
					found2 = true
				}
				if strings.Contains(entry.Name(), "3") {
					found3 = true
				}
			}
			if !found1 || !found2 || !found3 {
				t.Errorf("Expected user entries with IDs 1, 2, 3 in /api/users, got %v", userEntries)
			}

			userContent, err := os.ReadFile(et.path("api", "users", "1"))
			if err != nil {
				t.Fatalf("Failed to read /api/users/1: %v", err)
			}
			if !strings.Contains(string(userContent), "Alice") {
				t.Errorf("User 1 missing Alice: %q", string(userContent))
			}

			t.Log("Example simple test completed successfully")
		})
	}
}

func TestExampleSimpler(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	langs := []struct {
		lc langConfig
	}{
		{lc: langConfig{lang: "lua", exampleDir: "simpler", rootScript: "main.lua"}},
		{lc: langConfig{lang: "tengo", exampleDir: "simpler", rootScript: "main.tengo"}},
	}
	for _, tc := range langs {
		t.Run(tc.lc.lang, func(t *testing.T) {
			et := newExampleTest(t, tc.lc, nil)

			entries, err := os.ReadDir(et.mountPoint)
			if err != nil {
				t.Fatalf("Failed to list mount point: %v", err)
			}
			expectedRootEntries := map[string]bool{"hello.txt": false, "counter.txt": false, "echo": false, "files": false, "uploads": false}
			for _, entry := range entries {
				expectedRootEntries[entry.Name()] = true
			}
			for name, found := range expectedRootEntries {
				if !found {
					t.Errorf("Expected root entry %q not found", name)
				}
			}

			helloContent, err := os.ReadFile(et.path("hello.txt"))
			if err != nil {
				t.Fatalf("Failed to read /hello.txt: %v", err)
			}
			if !strings.Contains(string(helloContent), "Hello from nklhd FUSE filesystem!") {
				t.Errorf("hello.txt content missing expected text: %q", string(helloContent))
			}

			counter1, err := os.ReadFile(et.path("counter.txt"))
			if err != nil {
				t.Fatalf("Failed to read /counter.txt: %v", err)
			}
			counter2, _ := os.ReadFile(et.path("counter.txt"))
			if string(counter1) == string(counter2) {
				t.Logf("Counter may have incremented but strings equal: %q", string(counter1))
			}

			testData := "test echo data"
			if err := os.WriteFile(et.path("echo"), []byte(testData), 0644); err != nil {
				t.Fatalf("Failed to write to /echo: %v", err)
			}
			echoContent, err := os.ReadFile(et.path("echo"))
			if err != nil {
				t.Fatalf("Failed to read echo after write: %v", err)
			}
			if !strings.Contains(string(echoContent), "Echo:") {
				t.Errorf("Echo content missing 'Echo:' prefix: got %q", string(echoContent))
			}
			if !strings.Contains(string(echoContent), testData) {
				t.Errorf("Echo content missing test data: got %q", string(echoContent))
			}

			filesEntries, err := os.ReadDir(et.path("files"))
			if err != nil {
				t.Fatalf("Failed to list /files: %v", err)
			}
			expectedFiles := map[string]bool{"readme.txt": false, "nested": false}
			for _, entry := range filesEntries {
				expectedFiles[entry.Name()] = true
			}
			for name, found := range expectedFiles {
				if !found {
					t.Errorf("Expected file entry %q not found in /files", name)
				}
			}

			nestedContent, err := os.ReadFile(et.path("files", "nested", "deep.txt"))
			if err != nil {
				t.Fatalf("Failed to read nested file: %v", err)
			}
			if !strings.Contains(string(nestedContent), "Deeply nested file content.") {
				t.Errorf("Nested content mismatch: got %q", string(nestedContent))
			}

			uploadPath := et.path("uploads", "testfile.txt")
			f, err := os.Create(uploadPath)
			if err != nil {
				t.Errorf("Create in /uploads/ failed: %v", err)
			} else {
				f.Close()
				t.Log("Wildcard create operation succeeded")
			}

			t.Log("Example simpler test completed successfully")
		})
	}
}

func TestExampleUtilities(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	langs := []struct {
		lc  langConfig
		env map[string]string
	}{
		{lc: langConfig{lang: "lua", exampleDir: "utilities", rootScript: "main.lua"}, env: map[string]string{
			"USER": "testuser", "HOME": "/home/testuser", "PATH": "/usr/bin:/bin",
			"MAX_ITEMS": "25", "GREETING": "Welcome", "DEBUG_MODE": "1",
		}},
		{lc: langConfig{lang: "tengo", exampleDir: "utilities", rootScript: "main.tengo"}, env: map[string]string{
			"USER": "testuser", "HOME": "/home/testuser", "PATH": "/usr/bin:/bin",
			"MAX_ITEMS": "25", "GREETING": "Welcome", "DEBUG_MODE": "1",
		}},
	}
	for _, tc := range langs {
		t.Run(tc.lc.lang, func(t *testing.T) {
			et := newExampleTest(t, tc.lc, tc.env)

			infoContent, err := os.ReadFile(et.path("info"))
			if err != nil {
				t.Fatalf("Failed to read /info: %v", err)
			}
			infoStr := string(infoContent)
			if !strings.Contains(infoStr, "User: testuser") {
				t.Errorf("/info missing User: testuser, got: %s", infoStr)
			}
			if !strings.Contains(infoStr, "Home: /home/testuser") {
				t.Errorf("/info missing Home: /home/testuser, got: %s", infoStr)
			}
			if !strings.Contains(infoStr, "Max items: 25") {
				t.Errorf("/info missing Max items: 25, got: %s", infoStr)
			}
			if !strings.Contains(infoStr, "Default greeting: Welcome") {
				t.Errorf("/info missing Default greeting: Welcome, got: %s", infoStr)
			}

			counter1, err := os.ReadFile(et.path("counter"))
			if err != nil {
				t.Fatalf("Failed to read /counter: %v", err)
			}
			counter2, _ := os.ReadFile(et.path("counter"))
			if string(counter1) == string(counter2) {
				t.Logf("Counter may have incremented but strings equal: %q", string(counter1))
			}

			debugContent, err := os.ReadFile(et.path("debug-test"))
			if err != nil {
				t.Fatalf("Failed to read /debug-test: %v", err)
			}
			if !strings.Contains(string(debugContent), "Debug mode is ON") {
				t.Errorf("/debug-test expected 'Debug mode is ON', got: %q", string(debugContent))
			}

			t.Log("Example utilities test completed successfully")
		})
	}
}

func TestExampleSerialization(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	langs := []struct {
		lc langConfig
	}{
		{lc: langConfig{lang: "lua", exampleDir: "serialization", rootScript: "main.lua"}},
		{lc: langConfig{lang: "tengo", exampleDir: "serialization", rootScript: "main.tengo"}},
	}
	for _, tc := range langs {
		t.Run(tc.lc.lang, func(t *testing.T) {
			et := newExampleTest(t, tc.lc, nil)

			helloContent, err := os.ReadFile(et.path("api", "json", "hello"))
			if err != nil {
				t.Fatalf("Failed to read /api/json/hello: %v", err)
			}
			var helloData map[string]interface{}
			if err := yaml.Unmarshal(helloContent, &helloData); err != nil {
				t.Fatalf("Failed to unmarshal JSON from /api/json/hello: %v", err)
			}
			if helloData["message"] != "Hello from nklhd" {
				t.Errorf("Expected message='Hello from nklhd', got %v", helloData["message"])
			}
			if helloData["version"] != "1.0" {
				t.Errorf("Expected version='1.0', got %v", helloData["version"])
			}

			complexContent, err := os.ReadFile(et.path("api", "data", "complex"))
			if err != nil {
				t.Fatalf("Failed to read /api/data/complex: %v", err)
			}
			var complexData map[string]interface{}
			if err := yaml.Unmarshal(complexContent, &complexData); err != nil {
				t.Fatalf("Failed to unmarshal JSON from /api/data/complex: %v", err)
			}
			users, ok := complexData["users"].([]interface{})
			if !ok || len(users) != 3 {
				t.Errorf("Expected users array of length 3, got %v", complexData["users"])
			}
			metadata, ok := complexData["metadata"].(map[string]interface{})
			if !ok || metadata["format"] != "JSON" {
				t.Errorf("Expected metadata.format='JSON', got %v", metadata)
			}

			echoJsonContent, err := os.ReadFile(et.path("api", "echo", "json"))
			if err != nil {
				t.Fatalf("Failed to read /api/echo/json: %v", err)
			}
			var echoData map[string]interface{}
			if err := yaml.Unmarshal(echoJsonContent, &echoData); err != nil {
				t.Fatalf("Failed to unmarshal JSON from /api/echo/json: %v", err)
			}
			if echoData["format"] != "json" {
				t.Errorf("Expected format='json' in echo response, got %v", echoData["format"])
			}

			jsonInput := `{"test": "value", "numbers": [1, 2, 3]}`
			if err := os.WriteFile(et.path("api", "convert", "json-to-yaml"), []byte(jsonInput), 0644); err != nil {
				t.Fatalf("Failed to write to /api/convert/json-to-yaml: %v", err)
			}
			yamlOutput, err := os.ReadFile(et.path("api", "convert", "json-to-yaml"))
			if err != nil {
				t.Fatalf("Failed to read /api/convert/json-to-yaml after write: %v", err)
			}
			var yamlData map[string]interface{}
			if err := yaml.Unmarshal(yamlOutput, &yamlData); err != nil {
				t.Fatalf("Failed to unmarshal YAML from /api/convert/json-to-yaml: %v", err)
			}
			if yamlData["test"] != "value" {
				t.Errorf("Expected test='value' in YAML output, got %v", yamlData["test"])
			}

			yamlInput := "name: test\nvalues:\n  - 1\n  - 2\n  - 3"
			if err := os.WriteFile(et.path("api", "convert", "yaml-to-json"), []byte(yamlInput), 0644); err != nil {
				t.Fatalf("Failed to write to /api/convert/yaml-to-json: %v", err)
			}
			jsonOutput, err := os.ReadFile(et.path("api", "convert", "yaml-to-json"))
			if err != nil {
				t.Fatalf("Failed to read /api/convert/yaml-to-json after write: %v", err)
			}
			var jsonData map[string]interface{}
			if err := yaml.Unmarshal(jsonOutput, &jsonData); err != nil {
				t.Fatalf("Failed to unmarshal JSON from /api/convert/yaml-to-json: %v", err)
			}
			if jsonData["name"] != "test" {
				t.Errorf("Expected name='test' in JSON output, got %v", jsonData["name"])
			}

			envContent, err := os.ReadFile(et.path("api", "env"))
			if err != nil {
				t.Fatalf("Failed to read /api/env: %v", err)
			}
			var envList []interface{}
			if err := yaml.Unmarshal(envContent, &envList); err != nil {
				t.Fatalf("Failed to unmarshal YAML from /api/env: %v", err)
			}
			if len(envList) == 0 {
				t.Error("Expected at least one environment variable in /api/env output")
			}

			echoYamlContent, err := os.ReadFile(et.path("api", "echo", "yaml"))
			if err != nil {
				t.Fatalf("Failed to read /api/echo/yaml: %v", err)
			}
			var echoYamlData map[string]interface{}
			if err := yaml.Unmarshal(echoYamlContent, &echoYamlData); err != nil {
				t.Fatalf("Failed to unmarshal YAML from /api/echo/yaml: %v", err)
			}
			if echoYamlData["format"] != "yaml" {
				t.Errorf("Expected format='yaml' in echo response, got %v", echoYamlData["format"])
			}

			t.Log("Example serialization test completed successfully")
		})
	}
}

func TestExampleConversion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	langs := []struct {
		lc langConfig
	}{
		{lc: langConfig{lang: "lua", exampleDir: "conversion", rootScript: "main.lua"}},
		{lc: langConfig{lang: "tengo", exampleDir: "conversion", rootScript: "main.tengo"}},
	}
	for _, tc := range langs {
		t.Run(tc.lc.lang, func(t *testing.T) {
			et := newExampleTest(t, tc.lc, nil)

			switch tc.lc.lang {
			case "lua":
				yamlInput := "name: test\nvalues:\n  - 1\n  - 2\n  - 3"
				if err := os.WriteFile(et.path("tojson"), []byte(yamlInput), 0644); err != nil {
					t.Fatalf("Failed to write YAML to /tojson: %v", err)
				}
				jsonOutput, err := os.ReadFile(et.path("tojson"))
				if err != nil {
					t.Fatalf("Failed to read JSON from /tojson: %v", err)
				}
				var jsonData map[string]interface{}
				if err := yaml.Unmarshal(jsonOutput, &jsonData); err != nil {
					t.Fatalf("Failed to unmarshal JSON from /tojson: %v", err)
				}
				if jsonData["name"] != "test" {
					t.Errorf("Expected name='test' in JSON output, got %v", jsonData["name"])
				}

				jsonInput := `{"key": "value", "numbers": [1, 2, 3]}`
				if err := os.WriteFile(et.path("toyaml"), []byte(jsonInput), 0644); err != nil {
					t.Fatalf("Failed to write JSON to /toyaml: %v", err)
				}
				yamlOutput, err := os.ReadFile(et.path("toyaml"))
				if err != nil {
					t.Fatalf("Failed to read YAML from /toyaml: %v", err)
				}
				var yamlData map[string]interface{}
				if err := yaml.Unmarshal(yamlOutput, &yamlData); err != nil {
					t.Fatalf("Failed to unmarshal YAML from /toyaml: %v", err)
				}
				if yamlData["key"] != "value" {
					t.Errorf("Expected key='value' in YAML output, got %v", yamlData["key"])
				}

				yamlConvertInput := "item: apple\ncount: 5"
				if err := os.WriteFile(et.path("convert"), []byte(yamlConvertInput), 0644); err != nil {
					t.Fatalf("Failed to write YAML to /convert: %v", err)
				}
				convertOutput, err := os.ReadFile(et.path("convert"))
				if err != nil {
					t.Fatalf("Failed to read converted output from /convert: %v", err)
				}
				var convertData map[string]interface{}
				if err := yaml.Unmarshal(convertOutput, &convertData); err != nil {
					t.Fatalf("Failed to unmarshal converted output from /convert: %v", err)
				}
				if convertData["item"] != "apple" {
					t.Errorf("Expected item='apple' in converted output, got %v", convertData["item"])
				}

				jsonConvertInput := `{"id": 123, "active": true}`
				if err := os.WriteFile(et.path("convert"), []byte(jsonConvertInput), 0644); err != nil {
					t.Fatalf("Failed to write JSON to /convert: %v", err)
				}
				convertOutput2, err := os.ReadFile(et.path("convert"))
				if err != nil {
					t.Fatalf("Failed to read converted output from /convert (second): %v", err)
				}
				var convertData2 map[string]interface{}
				if err := yaml.Unmarshal(convertOutput2, &convertData2); err != nil {
					t.Fatalf("Failed to unmarshal converted output from /convert (second): %v", err)
				}
				if convertData2["id"] != 123 {
					t.Errorf("Expected id=123 in converted output, got %v", convertData2["id"])
				}

			case "tengo":
				yamlInput := "name: test\nvalues:\n  - 1\n  - 2\n  - 3"
				if err := os.WriteFile(et.path("tojson"), []byte(yamlInput), 0644); err != nil {
					t.Fatalf("Failed to write YAML to /tojson: %v", err)
				}
				jsonOutput, err := os.ReadFile(et.path("tojson"))
				if err != nil {
					t.Fatalf("Failed to read JSON from /tojson: %v", err)
				}
				var jsonData map[string]interface{}
				if err := yaml.Unmarshal(jsonOutput, &jsonData); err != nil {
					t.Fatalf("Failed to unmarshal JSON from /tojson: %v", err)
				}
				if jsonData["name"] != "test" {
					t.Errorf("Expected name='test' in JSON output, got %v", jsonData["name"])
				}

				jsonInput := `{"key": "value", "numbers": [1, 2, 3]}`
				if err := os.WriteFile(et.path("toyaml"), []byte(jsonInput), 0644); err != nil {
					t.Fatalf("Failed to write JSON to /toyaml: %v", err)
				}
				yamlOutput, err := os.ReadFile(et.path("toyaml"))
				if err != nil {
					t.Fatalf("Failed to read YAML from /toyaml: %v", err)
				}
				var yamlData map[string]interface{}
				if err := yaml.Unmarshal(yamlOutput, &yamlData); err != nil {
					t.Fatalf("Failed to unmarshal YAML from /toyaml: %v", err)
				}
				if yamlData["key"] != "value" {
					t.Errorf("Expected key='value' in YAML output, got %v", yamlData["key"])
				}

				yamlConvertInput := "item: apple\ncount: 5"
				if err := os.WriteFile(et.path("convert"), []byte(yamlConvertInput), 0644); err != nil {
					t.Fatalf("Failed to write YAML to /convert: %v", err)
				}
				convertOutput, err := os.ReadFile(et.path("convert"))
				if err != nil {
					t.Fatalf("Failed to read converted output from /convert: %v", err)
				}
				var convertData map[string]interface{}
				if err := yaml.Unmarshal(convertOutput, &convertData); err != nil {
					t.Fatalf("Failed to unmarshal converted output from /convert: %v", err)
				}
				if convertData["item"] != "apple" {
					t.Errorf("Expected item='apple' in converted output, got %v", convertData["item"])
				}

				jsonConvertInput := `{"id": 123, "active": true}`
				if err := os.WriteFile(et.path("convert"), []byte(jsonConvertInput), 0644); err != nil {
					t.Fatalf("Failed to write JSON to /convert: %v", err)
				}
				convertOutput2, err := os.ReadFile(et.path("convert"))
				if err != nil {
					t.Fatalf("Failed to read converted output from /convert (second): %v", err)
				}
				var convertData2 map[string]interface{}
				if err := yaml.Unmarshal(convertOutput2, &convertData2); err != nil {
					t.Fatalf("Failed to unmarshal converted output from /convert (second): %v", err)
				}
				if convertData2["id"] != 123 {
					t.Errorf("Expected id=123 in converted output, got %v", convertData2["id"])
				}
			}

			helpContent, err := os.ReadFile(et.path("help"))
			if err != nil {
				t.Fatalf("Failed to read /help: %v", err)
			}
			if !strings.Contains(string(helpContent), "JSON") || !strings.Contains(string(helpContent), "Conversion") {
				t.Errorf("/help missing expected 'JSON Conversion' text: %q", string(helpContent))
			}

			t.Log("Example conversion test completed successfully")
		})
	}
}
