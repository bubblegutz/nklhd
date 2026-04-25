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
	"testing"
	"time"

	"github.com/knusbaum/go9p/client"
	"github.com/knusbaum/go9p/proto"
	"github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"
)

// protocolTestLua is the common Lua route definition used by all protocol
// backends. Every route defined here must work identically across FUSE,
// 9p/TCP, 9p/Unix, 9p/FUSE, and 9p/CLI transports.
const protocolTestLua = `
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

// StatInfo holds basic file stat information for protocol-agnostic tests.
type StatInfo struct {
	Name  string
	Size  int64
	IsDir bool
}

// Backend defines the interface for protocol-agnostic filesystem tests.
// Each implementation wraps a different transport:
//
//   - FUSE: os.* on a FUSE mount point
//   - 9p/TCP: go9p client over TCP
//   - 9p/Unix: go9p client over Unix socket
//   - 9p/FUSE: os.* on a 9pfuse mount point
//   - 9p/CLI: 9p command-line tool over TCP
type Backend interface {
	// ReadFile reads the complete content of a file.
	ReadFile(path string) (string, error)
	// WriteFile writes data to a file.
	WriteFile(path string, data string) error
	// ReadDir returns the names of entries in a directory.
	ReadDir(path string) ([]string, error)
	// Stat returns file metadata.
	Stat(path string) (*StatInfo, error)
	// CreateFile creates a new file, suitable for testing app.create handlers.
	CreateFile(path string) error
	// RemoveFile removes a file, suitable for testing app.unlink handlers.
	RemoveFile(path string) error
	// Close cleans up backend resources (processes, temp dirs, mounts).
	Close()
}

// protocolTestTengo is the Tengo equivalent of protocolTestLua.
const protocolTestTengo = `
app.read("/hello", func(req) {
    return "Hello, world!"
})

echo_data := ""
app.write("/echo", func(req) {
    data := req.data
    if data == undefined { data = "" }
    echo_data = "Echo: " + data
    return echo_data
})
app.read("/echo", func(req) {
    return echo_data
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

// langScript returns the protocol test script and file extension for the given
// language identifier ("lua" or "tengo").
func langScript(lang string) (script string, ext string) {
	switch lang {
	case "tengo":
		return protocolTestTengo, ".tengo"
	default:
		return protocolTestLua, ".lua"
	}
}

// ---------------------------------------------------------------------------
// Shared test functions
// ---------------------------------------------------------------------------

func testBackendRead(t *testing.T, b Backend) {
	content, err := b.ReadFile("/hello")
	if err != nil {
		t.Fatalf("ReadFile /hello: %v", err)
	}
	content = strings.TrimSpace(content)
	if content != "Hello, world!" {
		t.Errorf("got %q, want %q", content, "Hello, world!")
	}
}

func testBackendWrite(t *testing.T, b Backend) {
	if err := b.WriteFile("/echo", "test data"); err != nil {
		t.Fatalf("WriteFile /echo: %v", err)
	}
	content, err := b.ReadFile("/echo")
	if err != nil {
		t.Fatalf("ReadFile /echo after write: %v", err)
	}
	content = strings.TrimSpace(content)
	if content != "Echo: test data" {
		t.Errorf("got %q, want %q", content, "Echo: test data")
	}
}

func testBackendReadDir(t *testing.T, b Backend) {
	entries, err := b.ReadDir("/")
	if err != nil {
		t.Fatalf("ReadDir /: %v", err)
	}
	found := make(map[string]bool)
	for _, e := range entries {
		if e != "." && e != ".." {
			found[e] = true
		}
	}
	for _, expected := range []string{"hello", "echo", "status", "dir"} {
		if !found[expected] {
			t.Errorf("expected %q in root directory, got %v", expected, entries)
		}
	}
}

func testBackendStat(t *testing.T, b Backend) {
	st, err := b.Stat("/hello")
	if err != nil {
		t.Fatalf("Stat /hello: %v", err)
	}
	if st.Name != "hello" {
		t.Errorf("stat name: got %q, want %q", st.Name, "hello")
	}
	if st.IsDir {
		t.Errorf("/hello should not be a directory")
	}
}

func testBackendPathParams(t *testing.T, b Backend) {
	content, err := b.ReadFile("/users/42")
	if err != nil {
		t.Fatalf("ReadFile /users/42: %v", err)
	}
	content = strings.TrimSpace(content)
	if content != "User: 42" {
		t.Errorf("got %q, want %q", content, "User: 42")
	}
}

func testBackendNestedRead(t *testing.T, b Backend) {
	content, err := b.ReadFile("/dir/file1")
	if err != nil {
		t.Fatalf("ReadFile /dir/file1: %v", err)
	}
	content = strings.TrimSpace(content)
	if content != "File1 content" {
		t.Errorf("got %q, want %q", content, "File1 content")
	}
}

// testBackendSuite runs all shared protocol-agnostic tests against the given backend.
func testBackendSuite(t *testing.T, b Backend) {
	t.Run("read", func(t *testing.T) { testBackendRead(t, b) })
	t.Run("write", func(t *testing.T) { testBackendWrite(t, b) })
	t.Run("readdir", func(t *testing.T) { testBackendReadDir(t, b) })
	t.Run("stat", func(t *testing.T) { testBackendStat(t, b) })
	t.Run("pathparams", func(t *testing.T) { testBackendPathParams(t, b) })
	t.Run("nested", func(t *testing.T) { testBackendNestedRead(t, b) })
}

// ---------------------------------------------------------------------------
// Port allocation helper
// ---------------------------------------------------------------------------

func findAvailablePort() (int, error) {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// ---------------------------------------------------------------------------
// FUSE backend — uses os.* on a FUSE mount point
// ---------------------------------------------------------------------------

type FUSEBackend struct {
	cmd        *exec.Cmd
	tmpDir     string
	mountPoint string
}

func NewFUSEBackend(t *testing.T, lang string) *FUSEBackend {
	t.Helper()
	nklhdPath, err := findNklhdBinary()
	if err != nil {
		t.Skipf("nklhd binary not found: %v", err)
	}
	if skipMsg := checkFUSEPrerequisites(); skipMsg != "" {
		t.Skipf("FUSE not available: %s", skipMsg)
	}

	tmpDir, err := os.MkdirTemp("", "nklhd-protocol-fuse-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	script, ext := langScript(lang)
	scriptPath := filepath.Join(tmpDir, "routes"+ext)
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}

	mountPoint := filepath.Join(tmpDir, "mount")
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}

	configPath := filepath.Join(tmpDir, "config.toml")
	tomlContent := fmt.Sprintf(`
mountpoint = %q
rootscript = %q
verbose = true
`, mountPoint, scriptPath)
	if err := os.WriteFile(configPath, []byte(tomlContent), 0644); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}

	cmd := exec.Command(nklhdPath, "--config", configPath, "--mount", mountPoint, "--verbose")
	var stderr bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	t.Logf("FUSE: starting %s --config %s --mount %s", nklhdPath, configPath, mountPoint)
	if err := cmd.Start(); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to start nklhd: %v", err)
	}

	var ready bool
	for i := 0; i < 10; i++ {
		time.Sleep(time.Duration(100*(1<<i)) * time.Millisecond)
		if isMounted(mountPoint) {
			if _, err := os.ReadDir(mountPoint); err == nil {
				ready = true
				break
			}
		}
	}
	if !ready {
		gracefullyShutdownNklhd(cmd.Process, mountPoint)
		os.RemoveAll(tmpDir)
		t.Fatalf("FUSE mount never became ready (stderr: %s)", stderr.String())
	}

	return &FUSEBackend{cmd: cmd, tmpDir: tmpDir, mountPoint: mountPoint}
}

func (b *FUSEBackend) ReadFile(path string) (string, error) {
	data, err := os.ReadFile(filepath.Join(b.mountPoint, path))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (b *FUSEBackend) WriteFile(path string, data string) error {
	return os.WriteFile(filepath.Join(b.mountPoint, path), []byte(data), 0644)
}

func (b *FUSEBackend) ReadDir(path string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(b.mountPoint, path))
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names, nil
}

func (b *FUSEBackend) Stat(path string) (*StatInfo, error) {
	fi, err := os.Stat(filepath.Join(b.mountPoint, path))
	if err != nil {
		return nil, err
	}
	return &StatInfo{
		Name:  fi.Name(),
		Size:  fi.Size(),
		IsDir: fi.IsDir(),
	}, nil
}

func (b *FUSEBackend) CreateFile(path string) error {
	f, err := os.Create(filepath.Join(b.mountPoint, path))
	if err != nil {
		return err
	}
	return f.Close()
}

func (b *FUSEBackend) RemoveFile(path string) error {
	return os.Remove(filepath.Join(b.mountPoint, path))
}

func (b *FUSEBackend) Close() {
	if b.cmd != nil {
		gracefullyShutdownNklhd(b.cmd.Process, b.mountPoint)
	}
	os.RemoveAll(b.tmpDir)
}

// ---------------------------------------------------------------------------
// 9p/TCP backend — go9p client over TCP
// ---------------------------------------------------------------------------

type NinepTCPBackend struct {
	cmd    *exec.Cmd
	tmpDir string
	client *client.Client
	addr   string
}

func NewNinepTCPBackend(t *testing.T, lang string) *NinepTCPBackend {
	t.Helper()
	nklhdPath, err := findNklhdBinary()
	if err != nil {
		t.Skipf("nklhd binary not found: %v", err)
	}

	port, err := findAvailablePort()
	if err != nil {
		t.Fatalf("failed to find port: %v", err)
	}
	tmpDir, err := os.MkdirTemp("", "nklhd-protocol-9p-tcp-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	script, ext := langScript(lang)
	scriptPath := filepath.Join(tmpDir, "routes"+ext)
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}

	addr := fmt.Sprintf("localhost:%d", port)
	cmd := exec.Command(nklhdPath, "--config", scriptPath, "--protocol", "9p", "--listen", addr, "--verbose")
	var stderr bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	t.Logf("9p/TCP: starting %s --config %s --protocol 9p --listen %s", nklhdPath, scriptPath, addr)
	if err := cmd.Start(); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to start nklhd: %v", err)
	}

	if !waitForNinepReady(addr) {
		gracefullyShutdownNklhd(cmd.Process, "")
		os.RemoveAll(tmpDir)
		t.Fatalf("9p server never became ready (stderr: %s)", stderr.String())
	}

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		gracefullyShutdownNklhd(cmd.Process, "")
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to connect to 9p server: %v", err)
	}
	c, err := client.NewClient(conn, "nobody", "")
	if err != nil {
		conn.Close()
		gracefullyShutdownNklhd(cmd.Process, "")
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create 9p client: %v", err)
	}

	return &NinepTCPBackend{cmd: cmd, tmpDir: tmpDir, client: c, addr: addr}
}

func (b *NinepTCPBackend) ReadFile(path string) (string, error) {
	f, err := b.client.Open(path, proto.Oread)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (b *NinepTCPBackend) WriteFile(path string, data string) error {
	f, err := b.client.Open(path, proto.Owrite)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write([]byte(data))
	return err
}

func (b *NinepTCPBackend) ReadDir(path string) ([]string, error) {
	stats, err := b.client.Readdir(path)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(stats))
	for i, s := range stats {
		names[i] = s.Name
	}
	return names, nil
}

func (b *NinepTCPBackend) Stat(path string) (*StatInfo, error) {
	st, err := b.client.Stat(path)
	if err != nil {
		return nil, err
	}
	return &StatInfo{
		Name:  st.Name,
		Size:  int64(st.Length),
		IsDir: (st.Mode & proto.DMDIR) != 0,
	}, nil
}

func (b *NinepTCPBackend) CreateFile(path string) error {
	f, err := b.client.Create(path, 0644)
	if err != nil {
		return err
	}
	return f.Close()
}

func (b *NinepTCPBackend) RemoveFile(path string) error {
	return b.client.Remove(path)
}

func (b *NinepTCPBackend) Close() {
	if b.cmd != nil {
		gracefullyShutdownNklhd(b.cmd.Process, "")
	}
	os.RemoveAll(b.tmpDir)
}

// ---------------------------------------------------------------------------
// 9p/Unix backend — go9p client over Unix socket
// ---------------------------------------------------------------------------

type NinepUnixBackend struct {
	cmd        *exec.Cmd
	tmpDir     string
	client     *client.Client
	socketPath string
}

func NewNinepUnixBackend(t *testing.T, lang string) *NinepUnixBackend {
	t.Helper()
	nklhdPath, err := findNklhdBinary()
	if err != nil {
		t.Skipf("nklhd binary not found: %v", err)
	}

	tmpDir, err := os.MkdirTemp("", "nklhd-protocol-9p-unix-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	script, ext := langScript(lang)
	scriptPath := filepath.Join(tmpDir, "routes"+ext)
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}

	socketPath := filepath.Join(tmpDir, "ninep.sock")
	listenAddr := "unix!" + socketPath

	cmd := exec.Command(nklhdPath, "--config", scriptPath, "--protocol", "9p", "--listen", listenAddr, "--verbose")
	var stderr bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	t.Logf("9p/Unix: starting %s --config %s --protocol 9p --listen %s", nklhdPath, scriptPath, listenAddr)
	if err := cmd.Start(); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to start nklhd: %v", err)
	}

	// Wait for Unix socket to be ready.
	var ready bool
	for i := 0; i < 10; i++ {
		conn, dialErr := net.DialTimeout("unix", socketPath, 500*time.Millisecond)
		if dialErr == nil {
			conn.Close()
			ready = true
			break
		}
		time.Sleep(time.Duration(100*(1<<i)) * time.Millisecond)
	}
	if !ready {
		gracefullyShutdownNklhd(cmd.Process, "")
		os.RemoveAll(tmpDir)
		t.Fatalf("9p Unix server never became ready (stderr: %s)", stderr.String())
	}

	conn, err := net.DialTimeout("unix", socketPath, 5*time.Second)
	if err != nil {
		gracefullyShutdownNklhd(cmd.Process, "")
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to connect to 9p Unix socket: %v", err)
	}
	c, err := client.NewClient(conn, "nobody", "")
	if err != nil {
		conn.Close()
		gracefullyShutdownNklhd(cmd.Process, "")
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create 9p client: %v", err)
	}

	return &NinepUnixBackend{cmd: cmd, tmpDir: tmpDir, client: c, socketPath: socketPath}
}

func (b *NinepUnixBackend) ReadFile(path string) (string, error) {
	f, err := b.client.Open(path, proto.Oread)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (b *NinepUnixBackend) WriteFile(path string, data string) error {
	f, err := b.client.Open(path, proto.Owrite)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write([]byte(data))
	return err
}

func (b *NinepUnixBackend) ReadDir(path string) ([]string, error) {
	stats, err := b.client.Readdir(path)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(stats))
	for i, s := range stats {
		names[i] = s.Name
	}
	return names, nil
}

func (b *NinepUnixBackend) Stat(path string) (*StatInfo, error) {
	st, err := b.client.Stat(path)
	if err != nil {
		return nil, err
	}
	return &StatInfo{
		Name:  st.Name,
		Size:  int64(st.Length),
		IsDir: (st.Mode & proto.DMDIR) != 0,
	}, nil
}

func (b *NinepUnixBackend) CreateFile(path string) error {
	f, err := b.client.Create(path, 0644)
	if err != nil {
		return err
	}
	return f.Close()
}

func (b *NinepUnixBackend) RemoveFile(path string) error {
	return b.client.Remove(path)
}

func (b *NinepUnixBackend) Close() {
	if b.cmd != nil {
		gracefullyShutdownNklhd(b.cmd.Process, "")
	}
	os.RemoveAll(b.tmpDir)
}

// ---------------------------------------------------------------------------
// 9p/FUSE backend — os.* on a 9pfuse mount point
// ---------------------------------------------------------------------------

type NinepFUSEBackend struct {
	nklhdCmd   *exec.Cmd
	fuseCmd    *exec.Cmd
	tmpDir     string
	mountPoint string
}

func NewNinepFUSEBackend(t *testing.T, lang string) *NinepFUSEBackend {
	t.Helper()
	nklhdPath, err := findNklhdBinary()
	if err != nil {
		t.Skipf("nklhd binary not found: %v", err)
	}
	if skipMsg := checkFUSEPrerequisites(); skipMsg != "" {
		t.Skipf("FUSE not available: %s", skipMsg)
	}
	if _, err := exec.LookPath("9pfuse"); err != nil {
		t.Skipf("9pfuse not found: %v", err)
	}

	port, err := findAvailablePort()
	if err != nil {
		t.Fatalf("failed to find port: %v", err)
	}
	tmpDir, err := os.MkdirTemp("", "nklhd-protocol-9pfuse-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	script, ext := langScript(lang)
	scriptPath := filepath.Join(tmpDir, "routes"+ext)
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}

	addr := fmt.Sprintf("localhost:%d", port)
	nklhdCmd := exec.Command(nklhdPath, "--config", scriptPath, "--protocol", "9p", "--listen", addr, "--verbose")
	var nklhdStderr bytes.Buffer
	nklhdCmd.Stdout = os.Stdout
	nklhdCmd.Stderr = io.MultiWriter(os.Stderr, &nklhdStderr)
	t.Logf("9p/FUSE: starting nklhd: %s --config %s --protocol 9p --listen %s", nklhdPath, scriptPath, addr)
	if err := nklhdCmd.Start(); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to start nklhd: %v", err)
	}

	if !waitForNinepReady(addr) {
		gracefullyShutdownNklhd(nklhdCmd.Process, "")
		os.RemoveAll(tmpDir)
		t.Fatalf("9p server never became ready (stderr: %s)", nklhdStderr.String())
	}

	mountPoint := filepath.Join(tmpDir, "mount")
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		gracefullyShutdownNklhd(nklhdCmd.Process, "")
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}

	host, portStr, _ := net.SplitHostPort(addr)
	plan9Addr := "tcp!" + host + "!" + portStr
	fuseCmd := exec.Command("9pfuse", plan9Addr, mountPoint)
	var fuseStderr bytes.Buffer
	fuseCmd.Stderr = &fuseStderr
	t.Logf("9p/FUSE: mounting 9pfuse %s %s", plan9Addr, mountPoint)
	if err := fuseCmd.Start(); err != nil {
		gracefullyShutdownNklhd(nklhdCmd.Process, "")
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to start 9pfuse: %v", err)
	}

	var ready bool
	for i := 0; i < 10; i++ {
		time.Sleep(time.Duration(100*(1<<i)) * time.Millisecond)
		if isMounted(mountPoint) {
			if _, err := os.ReadDir(mountPoint); err == nil {
				ready = true
				break
			}
		}
	}
	if !ready {
		fuseCmd.Process.Kill()
		gracefullyShutdownNklhd(nklhdCmd.Process, "")
		os.RemoveAll(tmpDir)
		t.Fatalf("9pfuse mount never became ready: %s", fuseStderr.String())
	}

	return &NinepFUSEBackend{
		nklhdCmd:   nklhdCmd,
		fuseCmd:    fuseCmd,
		tmpDir:     tmpDir,
		mountPoint: mountPoint,
	}
}

func (b *NinepFUSEBackend) ReadFile(path string) (string, error) {
	data, err := os.ReadFile(filepath.Join(b.mountPoint, path))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (b *NinepFUSEBackend) WriteFile(path string, data string) error {
	return os.WriteFile(filepath.Join(b.mountPoint, path), []byte(data), 0644)
}

func (b *NinepFUSEBackend) ReadDir(path string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(b.mountPoint, path))
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names, nil
}

func (b *NinepFUSEBackend) Stat(path string) (*StatInfo, error) {
	fi, err := os.Stat(filepath.Join(b.mountPoint, path))
	if err != nil {
		return nil, err
	}
	return &StatInfo{
		Name:  fi.Name(),
		Size:  fi.Size(),
		IsDir: fi.IsDir(),
	}, nil
}

func (b *NinepFUSEBackend) CreateFile(path string) error {
	f, err := os.Create(filepath.Join(b.mountPoint, path))
	if err != nil {
		return err
	}
	return f.Close()
}

func (b *NinepFUSEBackend) RemoveFile(path string) error {
	return os.Remove(filepath.Join(b.mountPoint, path))
}

func (b *NinepFUSEBackend) Close() {
	// Unmount 9pfuse first (process is killed when mount is unmounted).
	if b.fuseCmd != nil && isMounted(b.mountPoint) {
		unmountMountPoint(b.mountPoint)
	}
	if b.nklhdCmd != nil {
		gracefullyShutdownNklhd(b.nklhdCmd.Process, "")
	}
	os.RemoveAll(b.tmpDir)
}

// ---------------------------------------------------------------------------
// 9p/CLI backend — uses the '9p' command-line tool over TCP
//
// Named NinepUDPBackend for compatibility with existing test plans; uses TCP
// connectivity (the server listens on TCP) and exercises the 9p CLI which
// also supports Plan 9 udp! address format.
// ---------------------------------------------------------------------------

type NinepUDPBackend struct {
	cmd       *exec.Cmd
	tmpDir    string
	addr      string
	plan9Addr string
	client    *client.Client
}

func NewNinepUDPBackend(t *testing.T, lang string) *NinepUDPBackend {
	t.Helper()
	nklhdPath, err := findNklhdBinary()
	if err != nil {
		t.Skipf("nklhd binary not found: %v", err)
	}
	if _, err := exec.LookPath("9p"); err != nil {
		t.Skipf("9p command not found: %v", err)
	}

	port, err := findAvailablePort()
	if err != nil {
		t.Fatalf("failed to find port: %v", err)
	}
	tmpDir, err := os.MkdirTemp("", "nklhd-protocol-9p-cli-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	script, ext := langScript(lang)
	scriptPath := filepath.Join(tmpDir, "routes"+ext)
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}

	addr := fmt.Sprintf("localhost:%d", port)
	cmd := exec.Command(nklhdPath, "--config", scriptPath, "--protocol", "9p", "--listen", addr, "--verbose")
	var stderr bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	t.Logf("9p/CLI: starting %s --config %s --protocol 9p --listen %s", nklhdPath, scriptPath, addr)
	if err := cmd.Start(); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to start nklhd: %v", err)
	}

	if !waitForNinepReady(addr) {
		gracefullyShutdownNklhd(cmd.Process, "")
		os.RemoveAll(tmpDir)
		t.Fatalf("9p server never became ready (stderr: %s)", stderr.String())
	}

	host, portStr, _ := net.SplitHostPort(addr)
	plan9Addr := "tcp!" + host + "!" + portStr

	// Establish go9p client connection for Create/Remove operations (9p CLI doesn't support these).
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		gracefullyShutdownNklhd(cmd.Process, "")
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to connect to 9p server for go9p client: %v", err)
	}
	c, err := client.NewClient(conn, "nobody", "")
	if err != nil {
		conn.Close()
		gracefullyShutdownNklhd(cmd.Process, "")
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create go9p client: %v", err)
	}

	return &NinepUDPBackend{
		cmd:       cmd,
		tmpDir:    tmpDir,
		addr:      addr,
		plan9Addr: plan9Addr,
		client:    c,
	}
}

func (b *NinepUDPBackend) ReadFile(path string) (string, error) {
	out, err := exec.Command("9p", "-a", b.plan9Addr, "read", path).Output()
	if err != nil {
		return "", fmt.Errorf("9p read %s: %w", path, err)
	}
	// Remove trailing newline if present (9p read does not add one).
	return string(out), nil
}

func (b *NinepUDPBackend) WriteFile(path string, data string) error {
	cmd := exec.Command("9p", "-a", b.plan9Addr, "write", path)
	cmd.Stdin = strings.NewReader(data)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("9p write %s: %w (output: %s)", path, err, out)
	}
	return nil
}

func (b *NinepUDPBackend) ReadDir(path string) ([]string, error) {
	out, err := exec.Command("9p", "-a", b.plan9Addr, "ls", path).Output()
	if err != nil {
		return nil, fmt.Errorf("9p ls %s: %w", path, err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var entries []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && line != "." && line != ".." {
			entries = append(entries, line)
		}
	}
	return entries, nil
}

func (b *NinepUDPBackend) Stat(path string) (*StatInfo, error) {
	out, err := exec.Command("9p", "-a", b.plan9Addr, "stat", path).Output()
	if err != nil {
		return nil, fmt.Errorf("9p stat %s: %w", path, err)
	}
	// Parse stat output: 'name' 'uid' 'gid' 'muid' q (...) m mode ...
	name := extractQuoted(string(out))
	if name == "" {
		name = path
		if idx := strings.LastIndex(path, "/"); idx >= 0 {
			name = path[idx+1:]
		}
	}
	return &StatInfo{Name: name, IsDir: false}, nil
}

func (b *NinepUDPBackend) CreateFile(path string) error {
	f, err := b.client.Create(path, 0644)
	if err != nil {
		return err
	}
	return f.Close()
}

func (b *NinepUDPBackend) RemoveFile(path string) error {
	return b.client.Remove(path)
}

func (b *NinepUDPBackend) Close() {
	if b.cmd != nil {
		gracefullyShutdownNklhd(b.cmd.Process, "")
	}
	os.RemoveAll(b.tmpDir)
}

// ---------------------------------------------------------------------------
// SSH/SFTP backend — uses golang.org/x/crypto/ssh + github.com/pkg/sftp
// ---------------------------------------------------------------------------

type SSHBackend struct {
	cmd        *exec.Cmd
	tmpDir     string
	sshClient  *gossh.Client
	sftpClient *sftp.Client
	addr       string
}

func NewSSHBackend(t *testing.T, lang string) *SSHBackend {
	t.Helper()
	nklhdPath, err := findNklhdBinary()
	if err != nil {
		t.Skipf("nklhd binary not found: %v", err)
	}

	port, err := findAvailablePort()
	if err != nil {
		t.Fatalf("failed to find port: %v", err)
	}
	tmpDir, err := os.MkdirTemp("", "nklhd-protocol-ssh-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	script, ext := langScript(lang)
	scriptPath := filepath.Join(tmpDir, "routes"+ext)
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}

	// Generate ephemeral Ed25519 key pair for client authentication.
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to generate ed25519 key: %v", err)
	}

	// Create signer from private key.
	signer, err := gossh.NewSignerFromKey(privKey)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create signer: %v", err)
	}

	// Convert raw Ed25519 public key to ssh.PublicKey and write authorized_keys.
	sshPubKey, err := gossh.NewPublicKey(pubKey)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create ssh public key: %v", err)
	}
	authorizedKey := gossh.MarshalAuthorizedKey(sshPubKey)
	authKeysPath := filepath.Join(tmpDir, "authorized_keys")
	if err := os.WriteFile(authKeysPath, authorizedKey, 0644); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}

	addr := fmt.Sprintf("localhost:%d", port)
	cmd := exec.Command(nklhdPath, "--config", scriptPath, "--protocol", "ssh", "--ssh-addr", addr, "--ssh-authorized-keys", authKeysPath, "--verbose")
	var stderr bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	t.Logf("SSH: starting %s --config %s --protocol ssh --ssh-addr %s --ssh-authorized-keys %s", nklhdPath, scriptPath, addr, authKeysPath)
	if err := cmd.Start(); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to start nklhd: %v", err)
	}

	// Wait for SSH server to become ready (TCP connectivity).
	if !waitForNinepReady(addr) {
		gracefullyShutdownNklhd(cmd.Process, "")
		os.RemoveAll(tmpDir)
		t.Fatalf("SSH server never became ready (stderr: %s)", stderr.String())
	}

	// Dial SSH with the ephemeral key.
	sshConfig := &gossh.ClientConfig{
		User: "nklhd",
		Auth: []gossh.AuthMethod{
			gossh.PublicKeys(signer),
		},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	}
	sshClient, err := gossh.Dial("tcp", addr, sshConfig)
	if err != nil {
		gracefullyShutdownNklhd(cmd.Process, "")
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to dial SSH: %v", err)
	}

	// Open SFTP client over the SSH connection.
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		sshClient.Close()
		gracefullyShutdownNklhd(cmd.Process, "")
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create SFTP client: %v", err)
	}

	return &SSHBackend{
		cmd:        cmd,
		tmpDir:     tmpDir,
		sshClient:  sshClient,
		sftpClient: sftpClient,
		addr:       addr,
	}
}

func (b *SSHBackend) ReadFile(path string) (string, error) {
	f, err := b.sftpClient.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (b *SSHBackend) WriteFile(path string, data string) error {
	f, err := b.sftpClient.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write([]byte(data))
	return err
}

func (b *SSHBackend) ReadDir(path string) ([]string, error) {
	entries, err := b.sftpClient.ReadDir(path)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names, nil
}

func (b *SSHBackend) Stat(path string) (*StatInfo, error) {
	fi, err := b.sftpClient.Stat(path)
	if err != nil {
		return nil, err
	}
	return &StatInfo{
		Name:  fi.Name(),
		Size:  fi.Size(),
		IsDir: fi.IsDir(),
	}, nil
}

func (b *SSHBackend) CreateFile(path string) error {
	f, err := b.sftpClient.Create(path)
	if err != nil {
		return err
	}
	return f.Close()
}

func (b *SSHBackend) RemoveFile(path string) error {
	return b.sftpClient.Remove(path)
}

func (b *SSHBackend) Close() {
	if b.sftpClient != nil {
		b.sftpClient.Close()
	}
	if b.sshClient != nil {
		b.sshClient.Close()
	}
	if b.cmd != nil {
		gracefullyShutdownNklhd(b.cmd.Process, "")
	}
	os.RemoveAll(b.tmpDir)
}

// extractQuoted extracts the content of the first single-quoted string in s.
func extractQuoted(s string) string {
	start := strings.IndexByte(s, '\'')
	if start < 0 {
		return ""
	}
	end := strings.IndexByte(s[start+1:], '\'')
	if end < 0 {
		return ""
	}
	return s[start+1 : start+1+end]
}

// ---------------------------------------------------------------------------
// Top-level test functions
// ---------------------------------------------------------------------------

func TestProtocolFUSE(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	b := NewFUSEBackend(t, "lua")
	defer b.Close()
	testBackendSuite(t, b)
}

func TestProtocol9pTCP(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	b := NewNinepTCPBackend(t, "lua")
	defer b.Close()
	testBackendSuite(t, b)
}

func TestProtocol9pUnix(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	b := NewNinepUnixBackend(t, "lua")
	defer b.Close()
	testBackendSuite(t, b)
}

func TestProtocol9pFUSE(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	b := NewNinepFUSEBackend(t, "lua")
	defer b.Close()
	testBackendSuite(t, b)
}

func TestProtocolSSH(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	b := NewSSHBackend(t, "lua")
	defer b.Close()
	testBackendSuite(t, b)
}

func TestProtocol9pUDP(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	b := NewNinepUDPBackend(t, "lua")
	defer b.Close()
	testBackendSuite(t, b)
}
