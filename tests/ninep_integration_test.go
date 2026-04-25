package tests

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/knusbaum/go9p/client"
	"github.com/knusbaum/go9p/proto"
)

// checkNinepPrerequisites checks if the nklhd binary is available.
func checkNinepPrerequisites(t *testing.T) string {
	if testing.Short() {
		return "skipping in short mode"
	}
	if _, err := findNklhdBinary(); err != nil {
		return fmt.Sprintf("nklhd binary not found: %v", err)
	}
	return ""
}

// waitForNinepReady waits until the 9p server is accepting connections.
func waitForNinepReady(addr string) bool {
	for i := 0; i < 10; i++ {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(time.Duration(100*(1<<i)) * time.Millisecond)
	}
	return false
}

// startNinepServer starts nklhd with the 9p protocol and returns the cmd and listen address.
func startNinepServer(t *testing.T, tmpDir string, luaContent string, listenAddr string) *exec.Cmd {
	nklhdPath, err := findNklhdBinary()
	if err != nil {
		t.Fatalf("nklhd binary not found: %v", err)
	}

	luaScriptPath := filepath.Join(tmpDir, "routes.lua")
	if err := os.WriteFile(luaScriptPath, []byte(luaContent), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(nklhdPath,
		"--config", luaScriptPath,
		"--protocol", "9p",
		"--listen", listenAddr,
		"--verbose",
	)
	var stderr bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	t.Logf("Starting nklhd 9p: %s --config %s --protocol 9p --listen %s",
		nklhdPath, luaScriptPath, listenAddr)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start nklhd: %v", err)
	}
	return cmd
}

// connectNinep connects to a 9p server and returns the client.
func connectNinep(t *testing.T, addr string) *client.Client {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to connect to 9p server: %v", err)
	}
	// conn.Close() is handled by client.NewClient

	c, err := client.NewClient(conn, "nobody", "")
	if err != nil {
		conn.Close()
		t.Fatalf("failed to create 9p client: %v", err)
	}
	return c
}

// TestNinepReadWrite tests basic read and write operations via the 9p protocol.
func TestNinepReadWrite(t *testing.T) {
	if skipMsg := checkNinepPrerequisites(t); skipMsg != "" {
		t.Skip(skipMsg)
	}

	tmpDir, err := os.MkdirTemp("", "nklhd-ninep-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	luaContent := `
app.read("/hello", function(req)
    return "Hello, world!"
end)
app.write("/echo", function(req)
    return "Echo: " .. (req.data or "")
end)
app.read("/status", function(req)
    return "OK"
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
`
	listenAddr := "localhost:15640"
	cmd := startNinepServer(t, tmpDir, luaContent, listenAddr)
	defer func() {
		gracefullyShutdownNklhd(cmd.Process, "")
	}()

	if !waitForNinepReady(listenAddr) {
		t.Fatal("9p server never became ready")
	}

	c := connectNinep(t, listenAddr)

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
			t.Errorf("unexpected content: got %q, want %q", string(data), "Hello, world!")
		}
	})

	// Test 2: Read another file
	t.Run("status", func(t *testing.T) {
		f, err := c.Open("/status", proto.Oread)
		if err != nil {
			t.Fatalf("failed to open /status: %v", err)
		}
		defer f.Close()

		data, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("failed to read /status: %v", err)
		}
		if strings.TrimSpace(string(data)) != "OK" {
			t.Errorf("unexpected content: got %q, want %q", string(data), "OK")
		}
	})

	// Test 3: Stat a file
	t.Run("stat", func(t *testing.T) {
		st, err := c.Stat("/hello")
		if err != nil {
			t.Fatalf("failed to stat /hello: %v", err)
		}
		if st.Name != "hello" {
			t.Errorf("stat name: got %q, want %q", st.Name, "hello")
		}
	})

	// Test 4: Read root directory listing (static tree children)
	t.Run("root readdir", func(t *testing.T) {
		stats, err := c.Readdir("/")
		if err != nil {
			t.Fatalf("failed to readdir /: %v", err)
		}
		names := make([]string, len(stats))
		for i, s := range stats {
			names[i] = s.Name
		}
		t.Logf("root entries: %v", names)

		// Root should contain hello, echo, status, dir
		found := make(map[string]bool)
		for _, name := range names {
			found[name] = true
		}
		for _, expected := range []string{"hello", "echo", "status"} {
			if !found[expected] {
				t.Errorf("expected %q in root directory", expected)
			}
		}
	})

	// Test 5: Read subdirectory with static children
	t.Run("dir readdir", func(t *testing.T) {
		stats, err := c.Readdir("/dir")
		if err != nil {
			t.Fatalf("failed to readdir /dir: %v", err)
		}
		names := make([]string, len(stats))
		for i, s := range stats {
			names[i] = s.Name
		}
		t.Logf("dir entries: %v", names)

		// dir should have file1 and file2
		found := make(map[string]bool)
		for _, name := range names {
			found[name] = true
		}
		if !found["file1"] {
			t.Errorf("expected 'file1' in dir directory")
		}
		if !found["file2"] {
			t.Errorf("expected 'file2' in dir directory")
		}
	})

	// Test 6: Read nested file
	t.Run("nested read", func(t *testing.T) {
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
			t.Errorf("unexpected content: got %q, want %q", string(data), "File1 content")
		}
	})
}

// TestNinepServerStartup tests that the 9p server starts and shuts down cleanly.
func TestNinepServerStartup(t *testing.T) {
	if skipMsg := checkNinepPrerequisites(t); skipMsg != "" {
		t.Skip(skipMsg)
	}

	tmpDir, err := os.MkdirTemp("", "nklhd-ninep-startup-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	luaContent := `app.read("/ping", function(req) return "pong" end)`
	listenAddr := "localhost:15642"

	cmd := startNinepServer(t, tmpDir, luaContent, listenAddr)

	if !waitForNinepReady(listenAddr) {
		gracefullyShutdownNklhd(cmd.Process, "")
		t.Fatal("9p server never became ready")
	}

	c := connectNinep(t, listenAddr)

	f, err := c.Open("/ping", proto.Oread)
	if err != nil {
		t.Fatalf("failed to open /ping: %v", err)
	}

	data, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		t.Fatalf("failed to read /ping: %v", err)
	}

	if strings.TrimSpace(string(data)) != "pong" {
		t.Fatalf("unexpected response: got %q, want %q", string(data), "pong")
	}

	// Graceful shutdown
	if err := gracefullyShutdownNklhd(cmd.Process, ""); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
}

// TestNinepWrite tests write operations - note that routerFile uses 0444
// permissions by default, so writes require the file to have write permission
// at the 9p protocol level. This test verifies the expected behavior.
func TestNinepWrite(t *testing.T) {
	if skipMsg := checkNinepPrerequisites(t); skipMsg != "" {
		t.Skip(skipMsg)
	}

	tmpDir, err := os.MkdirTemp("", "nklhd-ninep-write-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	luaContent := `
app.write("/write-test", function(req)
    return "Wrote: " .. (req.data or "")
end)
`
	listenAddr := "localhost:15643"
	cmd := startNinepServer(t, tmpDir, luaContent, listenAddr)
	defer func() {
		gracefullyShutdownNklhd(cmd.Process, "")
	}()

	if !waitForNinepReady(listenAddr) {
		t.Fatal("9p server never became ready")
	}

	c := connectNinep(t, listenAddr)

	// Opening with Owrite should succeed (routerFile now uses 0666)
	f, err := c.Open("/write-test", proto.Owrite)
	if err != nil {
		t.Fatalf("failed to open /write-test for writing: %v", err)
	}
	defer f.Close()

	n, err := f.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("failed to write to /write-test: %v", err)
	}
	if n == 0 {
		t.Error("write returned 0 bytes")
	}
	t.Logf("Wrote %d bytes to /write-test", n)
}

// TestNinepPathParameters tests route path parameter extraction via 9p.
func TestNinepPathParameters(t *testing.T) {
	if skipMsg := checkNinepPrerequisites(t); skipMsg != "" {
		t.Skip(skipMsg)
	}

	tmpDir, err := os.MkdirTemp("", "nklhd-ninep-params-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	luaContent := `
app.read("/users/{id}", function(req)
    return "User: " .. req.id
end)
`
	listenAddr := "localhost:15644"
	cmd := startNinepServer(t, tmpDir, luaContent, listenAddr)
	defer func() {
		gracefullyShutdownNklhd(cmd.Process, "")
	}()

	if !waitForNinepReady(listenAddr) {
		t.Fatal("9p server never became ready")
	}

	c := connectNinep(t, listenAddr)

	// WalkFail should handle parameterized paths
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
		t.Errorf("unexpected content: got %q, want %q", string(data), "User: 42")
	}
}
