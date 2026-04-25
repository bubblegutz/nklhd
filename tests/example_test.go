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
	"github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v3"
)

// exampleLangConfig describes language-specific settings for an example test.
type exampleLangConfig struct {
	lang       string            // "lua" or "tengo"
	exampleDir string            // subdirectory under examples/{lang}/
	rootScript string            // e.g., "main.lua" or "main.tengo"
	env        map[string]string // optional extra environment variables
}

// transport selects which server protocol to test against.
type transport int

const (
	transportFUSE transport = iota
	transport9pTCP
	transport9pUnix
	transport9pFUSE
	transport9pCLI
	transportSSH
)

// envAsSlice converts a map of env vars to a slice of "key=value" strings.
func envAsSlice(env map[string]string) []string {
	s := make([]string, 0, len(env))
	for k, v := range env {
		s = append(s, k+"="+v)
	}
	return s
}

// copyExampleDir copies all files from src to dst recursively.
func copyExampleDir(src, dst string) error {
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

// ---------------------------------------------------------------------------
// exampleBackend - wraps an existing Backend with deferred cleanup
// ---------------------------------------------------------------------------

type exampleBackend struct {
	Backend
}

// ---------------------------------------------------------------------------
// Backend factory — creates the right backend for a transport + example combo
// ---------------------------------------------------------------------------

func newExampleBackend(t *testing.T, tr transport, lc exampleLangConfig, env map[string]string) Backend {
	t.Helper()
	nklhdPath, err := findNklhdBinary()
	if err != nil {
		t.Skipf("nklhd binary not found: %v", err)
	}

	tmpDir, err := os.MkdirTemp("", "nklhd-example-"+lc.exampleDir+"-"+lc.lang+"-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Copy example files into tmpDir
	exampleDir := filepath.Join("..", "examples", lc.lang, lc.exampleDir)
	if _, err := os.Stat(exampleDir); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("example directory not found: %v", err)
	}
	if err := copyExampleDir(exampleDir, tmpDir); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to copy example directory: %v", err)
	}

	configPath := filepath.Join(tmpDir, "config.toml")
	mountPoint := filepath.Join(tmpDir, "mount")

	switch tr {
	case transportFUSE:
		if skipMsg := checkFUSEPrerequisites(); skipMsg != "" {
			os.RemoveAll(tmpDir)
			t.Skipf("FUSE not available: %s", skipMsg)
		}
		if err := os.MkdirAll(mountPoint, 0755); err != nil {
			os.RemoveAll(tmpDir)
			t.Fatal(err)
		}
		configContent := fmt.Sprintf(`mountpoint = %q
rootscript = %q
verbose = true
`, mountPoint, lc.rootScript)
		if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
			os.RemoveAll(tmpDir)
			t.Fatal(err)
		}

		cmd := exec.Command(nklhdPath, "--config", configPath, "--mount", mountPoint, "--verbose")
		if env != nil {
			cmd.Env = append(os.Environ(), envAsSlice(env)...)
		}
		var stderr bytes.Buffer
		cmd.Stdout = os.Stdout
		cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
		t.Logf("Example/FUSE: starting %s --config %s --mount %s", nklhdPath, configPath, mountPoint)
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

	case transport9pTCP:
		port, err := findAvailablePort()
		if err != nil {
			os.RemoveAll(tmpDir)
			t.Fatalf("failed to find port: %v", err)
		}
		addr := fmt.Sprintf("localhost:%d", port)
		configContent := fmt.Sprintf(`rootscript = %q
verbose = true
`, lc.rootScript)
		if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
			os.RemoveAll(tmpDir)
			t.Fatal(err)
		}

		cmd := exec.Command(nklhdPath, "--config", configPath, "--protocol", "9p", "--listen", addr, "--verbose")
		if env != nil {
			cmd.Env = append(os.Environ(), envAsSlice(env)...)
		}
		var stderr bytes.Buffer
		cmd.Stdout = os.Stdout
		cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
		t.Logf("Example/9pTCP: starting %s --config %s --protocol 9p --listen %s", nklhdPath, configPath, addr)
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

	case transport9pUnix:
		configContent := fmt.Sprintf(`rootscript = %q
verbose = true
`, lc.rootScript)
		if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
			os.RemoveAll(tmpDir)
			t.Fatal(err)
		}

		socketPath := filepath.Join(tmpDir, "ninep.sock")
		listenAddr := "unix!" + socketPath

		cmd := exec.Command(nklhdPath, "--config", configPath, "--protocol", "9p", "--listen", listenAddr, "--verbose")
		if env != nil {
			cmd.Env = append(os.Environ(), envAsSlice(env)...)
		}
		var stderr bytes.Buffer
		cmd.Stdout = os.Stdout
		cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
		t.Logf("Example/9pUnix: starting %s --config %s --protocol 9p --listen %s", nklhdPath, configPath, listenAddr)
		if err := cmd.Start(); err != nil {
			os.RemoveAll(tmpDir)
			t.Fatalf("failed to start nklhd: %v", err)
		}

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

	case transport9pFUSE:
		if skipMsg := checkFUSEPrerequisites(); skipMsg != "" {
			os.RemoveAll(tmpDir)
			t.Skipf("FUSE not available: %s", skipMsg)
		}
		if _, err := exec.LookPath("9pfuse"); err != nil {
			os.RemoveAll(tmpDir)
			t.Skipf("9pfuse not found: %v", err)
		}

		port, err := findAvailablePort()
		if err != nil {
			os.RemoveAll(tmpDir)
			t.Fatalf("failed to find port: %v", err)
		}
		addr := fmt.Sprintf("localhost:%d", port)
		configContent := fmt.Sprintf(`rootscript = %q
verbose = true
`, lc.rootScript)
		if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
			os.RemoveAll(tmpDir)
			t.Fatal(err)
		}

		nklhdCmd := exec.Command(nklhdPath, "--config", configPath, "--protocol", "9p", "--listen", addr, "--verbose")
		if env != nil {
			nklhdCmd.Env = append(os.Environ(), envAsSlice(env)...)
		}
		var nklhdStderr bytes.Buffer
		nklhdCmd.Stdout = os.Stdout
		nklhdCmd.Stderr = io.MultiWriter(os.Stderr, &nklhdStderr)
		t.Logf("Example/9pFUSE: starting nklhd: %s --config %s --protocol 9p --listen %s", nklhdPath, configPath, addr)
		if err := nklhdCmd.Start(); err != nil {
			os.RemoveAll(tmpDir)
			t.Fatalf("failed to start nklhd: %v", err)
		}

		if !waitForNinepReady(addr) {
			gracefullyShutdownNklhd(nklhdCmd.Process, "")
			os.RemoveAll(tmpDir)
			t.Fatalf("9p server never became ready (stderr: %s)", nklhdStderr.String())
		}

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
		t.Logf("Example/9pFUSE: mounting 9pfuse %s %s", plan9Addr, mountPoint)
		if err := fuseCmd.Start(); err != nil {
			gracefullyShutdownNklhd(nklhdCmd.Process, "")
			os.RemoveAll(tmpDir)
			t.Fatalf("failed to start 9pfuse: %v", err)
		}

		var fuseReady bool
		for i := 0; i < 10; i++ {
			time.Sleep(time.Duration(100*(1<<i)) * time.Millisecond)
			if isMounted(mountPoint) {
				if _, err := os.ReadDir(mountPoint); err == nil {
					fuseReady = true
					break
				}
			}
		}
		if !fuseReady {
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

	case transport9pCLI:
		if _, err := exec.LookPath("9p"); err != nil {
			os.RemoveAll(tmpDir)
			t.Skipf("9p command not found: %v", err)
		}
		port, err := findAvailablePort()
		if err != nil {
			os.RemoveAll(tmpDir)
			t.Fatalf("failed to find port: %v", err)
		}
		addr := fmt.Sprintf("localhost:%d", port)
		configContent := fmt.Sprintf(`rootscript = %q
verbose = true
`, lc.rootScript)
		if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
			os.RemoveAll(tmpDir)
			t.Fatal(err)
		}

		cmd := exec.Command(nklhdPath, "--config", configPath, "--protocol", "9p", "--listen", addr, "--verbose")
		if env != nil {
			cmd.Env = append(os.Environ(), envAsSlice(env)...)
		}
		var stderr bytes.Buffer
		cmd.Stdout = os.Stdout
		cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
		t.Logf("Example/9pCLI: starting %s --config %s --protocol 9p --listen %s", nklhdPath, configPath, addr)
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

	case transportSSH:
		port, err := findAvailablePort()
		if err != nil {
			os.RemoveAll(tmpDir)
			t.Fatalf("failed to find port: %v", err)
		}
		addr := fmt.Sprintf("localhost:%d", port)
		configContent := fmt.Sprintf(`rootscript = %q
verbose = true
`, lc.rootScript)
		if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
			os.RemoveAll(tmpDir)
			t.Fatal(err)
		}

		// Generate ephemeral Ed25519 key pair for client authentication.
		pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			os.RemoveAll(tmpDir)
			t.Fatalf("failed to generate ed25519 key: %v", err)
		}
		signer, err := gossh.NewSignerFromKey(privKey)
		if err != nil {
			os.RemoveAll(tmpDir)
			t.Fatalf("failed to create signer: %v", err)
		}
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

		cmd := exec.Command(nklhdPath, "--config", configPath, "--protocol", "ssh", "--ssh-addr", addr, "--ssh-authorized-keys", authKeysPath, "--verbose")
		if env != nil {
			cmd.Env = append(os.Environ(), envAsSlice(env)...)
		}
		var stderr bytes.Buffer
		cmd.Stdout = os.Stdout
		cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
		t.Logf("Example/SSH: starting %s --config %s --protocol ssh --ssh-addr %s --ssh-authorized-keys %s", nklhdPath, configPath, addr, authKeysPath)
		if err := cmd.Start(); err != nil {
			os.RemoveAll(tmpDir)
			t.Fatalf("failed to start nklhd: %v", err)
		}

		if !waitForNinepReady(addr) {
			gracefullyShutdownNklhd(cmd.Process, "")
			os.RemoveAll(tmpDir)
			t.Fatalf("SSH server never became ready (stderr: %s)", stderr.String())
		}

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

	default:
		os.RemoveAll(tmpDir)
		t.Fatalf("unknown transport: %v", tr)
		return nil
	}
}

// ---------------------------------------------------------------------------
// Example test suite — runs all example tests against a single backend for
// a specific (transport, lang, exampleDir) combination.
// ---------------------------------------------------------------------------

func runExampleTest(t *testing.T, b Backend, lc exampleLangConfig) {
	switch lc.exampleDir {
	case "simple":
		testExampleSimple(t, b)
	case "simpler":
		testExampleSimpler(t, b)
	case "utilities":
		testExampleUtilities(t, b)
	case "serialization":
		testExampleSerialization(t, b, lc.lang)
	case "conversion":
		testExampleConversion(t, b, lc.lang)
	case "http":
		testExampleHttp(t, b, lc.lang)
	case "jq":
		testExampleJq(t, b, lc.lang)
	case "oauth":
		testExampleOauth(t, b, lc.lang)
	case "wikipedia":
		testExampleWikipedia(t, b)
	}
}

// ---------------------------------------------------------------------------
// Shared test functions — one per example
// ---------------------------------------------------------------------------

func testExampleSimple(t *testing.T, b Backend) {
	entries, err := b.ReadDir("/")
	if err != nil {
		t.Fatalf("ReadDir /: %v", err)
	}
	expectedRoot := map[string]bool{"api": false, "docs": false, "status.txt": false, "config.txt": false}
	for _, e := range entries {
		expectedRoot[e] = true
	}
	for name, found := range expectedRoot {
		if !found {
			t.Errorf("expected root entry %q not found in %v", name, entries)
		}
	}

	docContent, err := b.ReadFile("/docs/api.md")
	if err != nil {
		t.Fatalf("ReadFile /docs/api.md: %v", err)
	}
	if !strings.Contains(docContent, "nklhd API Documentation") {
		t.Errorf("API doc missing expected text: %q", docContent)
	}

	statusContent, err := b.ReadFile("/status.txt")
	if err != nil {
		t.Fatalf("ReadFile /status.txt: %v", err)
	}
	if !strings.Contains(statusContent, "System Status:") {
		t.Errorf("status.txt missing expected text: %q", statusContent)
	}

	configContent, err := b.ReadFile("/config.txt")
	if err != nil {
		t.Fatalf("ReadFile /config.txt: %v", err)
	}
	if !strings.Contains(configContent, "Configuration:") {
		t.Errorf("config.txt missing expected text: %q", configContent)
	}

	userEntries, err := b.ReadDir("/api/users")
	if err != nil {
		t.Fatalf("ReadDir /api/users: %v", err)
	}
	found1, found2, found3 := false, false, false
	for _, entry := range userEntries {
		if strings.Contains(entry, "1") {
			found1 = true
		}
		if strings.Contains(entry, "2") {
			found2 = true
		}
		if strings.Contains(entry, "3") {
			found3 = true
		}
	}
	if !found1 || !found2 || !found3 {
		t.Errorf("Expected user entries with IDs 1, 2, 3 in /api/users, got %v", userEntries)
	}

	userContent, err := b.ReadFile("/api/users/1")
	if err != nil {
		t.Fatalf("ReadFile /api/users/1: %v", err)
	}
	if !strings.Contains(userContent, "Alice") {
		t.Errorf("User 1 missing Alice: %q", userContent)
	}
}

func testExampleSimpler(t *testing.T, b Backend) {
	entries, err := b.ReadDir("/")
	if err != nil {
		t.Fatalf("ReadDir /: %v", err)
	}
	expectedRoot := map[string]bool{"hello.txt": false, "counter.txt": false, "echo": false, "files": false, "uploads": false}
	for _, e := range entries {
		expectedRoot[e] = true
	}
	for name, found := range expectedRoot {
		if !found {
			t.Errorf("expected root entry %q not found in %v", name, entries)
		}
	}

	helloContent, err := b.ReadFile("/hello.txt")
	if err != nil {
		t.Fatalf("ReadFile /hello.txt: %v", err)
	}
	if !strings.Contains(helloContent, "Hello from nklhd FUSE filesystem!") {
		t.Errorf("hello.txt missing expected text: %q", helloContent)
	}

	counter1, err := b.ReadFile("/counter.txt")
	if err != nil {
		t.Fatalf("ReadFile /counter.txt: %v", err)
	}
	counter2, _ := b.ReadFile("/counter.txt")
	if counter1 == counter2 {
		t.Logf("Counter may have incremented but strings equal: %q", counter1)
	}

	testData := "test echo data"
	if err := b.WriteFile("/echo", testData); err != nil {
		t.Fatalf("WriteFile /echo: %v", err)
	}
	echoContent, err := b.ReadFile("/echo")
	if err != nil {
		t.Fatalf("ReadFile /echo after write: %v", err)
	}
	if !strings.Contains(echoContent, "Echo:") {
		t.Errorf("Echo content missing 'Echo:' prefix: got %q", echoContent)
	}
	if !strings.Contains(echoContent, testData) {
		t.Errorf("Echo content missing test data: got %q", echoContent)
	}

	filesEntries, err := b.ReadDir("/files")
	if err != nil {
		t.Fatalf("ReadDir /files: %v", err)
	}
	expectedFiles := map[string]bool{"readme.txt": false, "nested": false}
	for _, e := range filesEntries {
		expectedFiles[e] = true
	}
	for name, found := range expectedFiles {
		if !found {
			t.Errorf("expected file entry %q not found in /files, got %v", name, filesEntries)
		}
	}

	nestedContent, err := b.ReadFile("/files/nested/deep.txt")
	if err != nil {
		t.Fatalf("ReadFile /files/nested/deep.txt: %v", err)
	}
	if !strings.Contains(nestedContent, "Deeply nested file content.") {
		t.Errorf("nested content mismatch: got %q", nestedContent)
	}

	if err := b.CreateFile("/uploads/testfile.txt"); err != nil {
		t.Errorf("CreateFile /uploads/testfile.txt failed: %v", err)
	}
}

func testExampleUtilities(t *testing.T, b Backend) {
	infoContent, err := b.ReadFile("/info")
	if err != nil {
		t.Fatalf("ReadFile /info: %v", err)
	}
	if !strings.Contains(infoContent, "User: testuser") {
		t.Errorf("/info missing User: testuser, got: %s", infoContent)
	}
	if !strings.Contains(infoContent, "Home: /home/testuser") {
		t.Errorf("/info missing Home: /home/testuser, got: %s", infoContent)
	}
	if !strings.Contains(infoContent, "Max items: 25") {
		t.Errorf("/info missing Max items: 25, got: %s", infoContent)
	}
	if !strings.Contains(infoContent, "Default greeting: Welcome") {
		t.Errorf("/info missing Default greeting: Welcome, got: %s", infoContent)
	}

	counter1, err := b.ReadFile("/counter")
	if err != nil {
		t.Fatalf("ReadFile /counter: %v", err)
	}
	counter2, _ := b.ReadFile("/counter")
	if counter1 == counter2 {
		t.Logf("Counter may have incremented but strings equal: %q", counter1)
	}

	debugContent, err := b.ReadFile("/debug-test")
	if err != nil {
		t.Fatalf("ReadFile /debug-test: %v", err)
	}
	if !strings.Contains(debugContent, "Debug mode is ON") {
		t.Errorf("/debug-test expected 'Debug mode is ON', got: %q", debugContent)
	}
}

func testExampleSerialization(t *testing.T, b Backend, lang string) {
	helloContent, err := b.ReadFile("/api/json/hello")
	if err != nil {
		t.Fatalf("ReadFile /api/json/hello: %v", err)
	}
	var helloData map[string]interface{}
	if err := yaml.Unmarshal([]byte(helloContent), &helloData); err != nil {
		t.Fatalf("Failed to unmarshal JSON from /api/json/hello: %v (content: %q)", err, helloContent)
	}
	if helloData["message"] != "Hello from nklhd" {
		t.Errorf("Expected message='Hello from nklhd', got %v", helloData["message"])
	}
	if helloData["version"] != "1.0" {
		t.Errorf("Expected version='1.0', got %v", helloData["version"])
	}

	complexContent, err := b.ReadFile("/api/data/complex")
	if err != nil {
		t.Fatalf("ReadFile /api/data/complex: %v", err)
	}
	var complexData map[string]interface{}
	if err := yaml.Unmarshal([]byte(complexContent), &complexData); err != nil {
		t.Fatalf("Failed to unmarshal JSON from /api/data/complex: %v (content: %q)", err, complexContent)
	}
	users, ok := complexData["users"].([]interface{})
	if !ok || len(users) != 3 {
		t.Errorf("Expected users array of length 3, got %v", complexData["users"])
	}
	metadata, ok := complexData["metadata"].(map[string]interface{})
	if !ok || metadata["format"] != "JSON" {
		t.Errorf("Expected metadata.format='JSON', got %v", metadata)
	}

	echoJsonContent, err := b.ReadFile("/api/echo/json")
	if err != nil {
		t.Fatalf("ReadFile /api/echo/json: %v", err)
	}
	var echoData map[string]interface{}
	if err := yaml.Unmarshal([]byte(echoJsonContent), &echoData); err != nil {
		t.Fatalf("Failed to unmarshal JSON from /api/echo/json: %v (content: %q)", err, echoJsonContent)
	}
	if echoData["format"] != "json" {
		t.Errorf("Expected format='json' in echo response, got %v", echoData["format"])
	}

	// Conversion routes: write JSON, read YAML
	jsonInput := `{"test": "value", "numbers": [1, 2, 3]}`
	if err := b.WriteFile("/api/convert/json-to-yaml", jsonInput); err != nil {
		t.Fatalf("WriteFile /api/convert/json-to-yaml: %v", err)
	}
	yamlOutput, err := b.ReadFile("/api/convert/json-to-yaml")
	if err != nil {
		t.Fatalf("ReadFile /api/convert/json-to-yaml after write: %v", err)
	}
	var yamlData map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlOutput), &yamlData); err != nil {
		t.Fatalf("Failed to unmarshal YAML from /api/convert/json-to-yaml: %v (content: %q)", err, yamlOutput)
	}
	if yamlData["test"] != "value" {
		t.Errorf("Expected test='value' in YAML output, got %v", yamlData["test"])
	}

	// Conversion routes: write YAML, read JSON
	yamlInput := "name: test\nvalues:\n  - 1\n  - 2\n  - 3"
	if err := b.WriteFile("/api/convert/yaml-to-json", yamlInput); err != nil {
		t.Fatalf("WriteFile /api/convert/yaml-to-json: %v", err)
	}
	jsonOutput, err := b.ReadFile("/api/convert/yaml-to-json")
	if err != nil {
		t.Fatalf("ReadFile /api/convert/yaml-to-json after write: %v", err)
	}
	var jsonData map[string]interface{}
	if err := yaml.Unmarshal([]byte(jsonOutput), &jsonData); err != nil {
		t.Fatalf("Failed to unmarshal JSON from /api/convert/yaml-to-json: %v (content: %q)", err, jsonOutput)
	}
	if jsonData["name"] != "test" {
		t.Errorf("Expected name='test' in JSON output, got %v", jsonData["name"])
	}

	// Environment variables route
	envContent, err := b.ReadFile("/api/env")
	if err != nil {
		t.Fatalf("ReadFile /api/env: %v", err)
	}
	var envList []interface{}
	if err := yaml.Unmarshal([]byte(envContent), &envList); err != nil {
		t.Fatalf("Failed to unmarshal YAML from /api/env: %v (content: %q)", err, envContent)
	}
	if len(envList) == 0 {
		t.Error("Expected at least one environment variable in /api/env output")
	}

	// Echo YAML route
	echoYamlContent, err := b.ReadFile("/api/echo/yaml")
	if err != nil {
		t.Fatalf("ReadFile /api/echo/yaml: %v", err)
	}
	var echoYamlData map[string]interface{}
	if err := yaml.Unmarshal([]byte(echoYamlContent), &echoYamlData); err != nil {
		t.Fatalf("Failed to unmarshal YAML from /api/echo/yaml: %v (content: %q)", err, echoYamlContent)
	}
	if echoYamlData["format"] != "yaml" {
		t.Errorf("Expected format='yaml' in echo response, got %v", echoYamlData["format"])
	}
}

func testExampleConversion(t *testing.T, b Backend, lang string) {
	yamlInput := "name: test\nvalues:\n  - 1\n  - 2\n  - 3"
	if err := b.WriteFile("/tojson", yamlInput); err != nil {
		t.Fatalf("WriteFile /tojson: %v", err)
	}
	jsonOutput, err := b.ReadFile("/tojson")
	if err != nil {
		t.Fatalf("ReadFile /tojson: %v", err)
	}
	var jsonData map[string]interface{}
	if err := yaml.Unmarshal([]byte(jsonOutput), &jsonData); err != nil {
		t.Fatalf("Failed to unmarshal JSON from /tojson: %v (content: %q)", err, jsonOutput)
	}
	if jsonData["name"] != "test" {
		t.Errorf("Expected name='test' in JSON output, got %v", jsonData["name"])
	}

	jsonInput := `{"key": "value", "numbers": [1, 2, 3]}`
	if err := b.WriteFile("/toyaml", jsonInput); err != nil {
		t.Fatalf("WriteFile /toyaml: %v", err)
	}
	yamlOutput, err := b.ReadFile("/toyaml")
	if err != nil {
		t.Fatalf("ReadFile /toyaml: %v", err)
	}
	var yamlData map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlOutput), &yamlData); err != nil {
		t.Fatalf("Failed to unmarshal YAML from /toyaml: %v (content: %q)", err, yamlOutput)
	}
	if yamlData["key"] != "value" {
		t.Errorf("Expected key='value' in YAML output, got %v", yamlData["key"])
	}

	yamlConvertInput := "item: apple\ncount: 5"
	if err := b.WriteFile("/convert", yamlConvertInput); err != nil {
		t.Fatalf("WriteFile /convert: %v", err)
	}
	convertOutput, err := b.ReadFile("/convert")
	if err != nil {
		t.Fatalf("ReadFile /convert: %v", err)
	}
	var convertData map[string]interface{}
	if err := yaml.Unmarshal([]byte(convertOutput), &convertData); err != nil {
		t.Fatalf("Failed to unmarshal converted output from /convert: %v (content: %q)", err, convertOutput)
	}
	if convertData["item"] != "apple" {
		t.Errorf("Expected item='apple' in converted output, got %v", convertData["item"])
	}

	jsonConvertInput := `{"id": 123, "active": true}`
	if err := b.WriteFile("/convert", jsonConvertInput); err != nil {
		t.Fatalf("WriteFile /convert (second): %v", err)
	}
	convertOutput2, err := b.ReadFile("/convert")
	if err != nil {
		t.Fatalf("ReadFile /convert (second): %v", err)
	}
	var convertData2 map[string]interface{}
	if err := yaml.Unmarshal([]byte(convertOutput2), &convertData2); err != nil {
		t.Fatalf("Failed to unmarshal converted output from /convert (second): %v (content: %q)", err, convertOutput2)
	}
	if convertData2["id"] != 123 {
		t.Errorf("Expected id=123 in converted output, got %v", convertData2["id"])
	}
}

func testExampleHttp(t *testing.T, b Backend, lang string) {
	// HTTP module routes: mostly documentation and external API calls.
	// Just verify they load and return something reasonable.
	helpContent, err := b.ReadFile("/http/help")
	if err != nil {
		t.Fatalf("ReadFile /http: %v", err)
	}
	if len(helpContent) == 0 {
		t.Error("/http returned empty content")
	}

	_, err = b.ReadFile("/http/headers")
	if err != nil {
		t.Logf("Warning: /http/headers failed (network?): %v", err)
	}
}

func testExampleJq(t *testing.T, b Backend, lang string) {
	if _, err := b.ReadFile("/jq/help"); err != nil {
		t.Fatalf("ReadFile /jq: %v", err)
	}

	usersContent, err := b.ReadFile("/jq/users")
	if err != nil {
		t.Fatalf("ReadFile /jq/users: %v", err)
	}
	if !strings.Contains(strings.ToLower(usersContent), "alice") {
		t.Errorf("/jq/users missing alice, got: %q", usersContent)
	}

	productsContent, err := b.ReadFile("/jq/products")
	if err != nil {
		t.Fatalf("ReadFile /jq/products: %v", err)
	}
	if !strings.Contains(strings.ToLower(productsContent), "laptop") {
		t.Errorf("/jq/products missing laptop, got: %q", productsContent)
	}

	statsContent, err := b.ReadFile("/jq/stats")
	if err != nil {
		t.Fatalf("ReadFile /jq/stats: %v", err)
	}
	if !strings.Contains(statsContent, "user_count") && !strings.Contains(statsContent, "oldest_user") {
		t.Errorf("/jq/stats missing expected stats, got: %q", statsContent)
	}

	namesContent, err := b.ReadFile("/jq/users-names")
	if err != nil {
		t.Fatalf("ReadFile /jq/users-names: %v", err)
	}
	if !strings.Contains(strings.ToLower(namesContent), "alice") && !strings.Contains(strings.ToLower(namesContent), "bob") {
		t.Errorf("/jq/users-names missing expected names, got: %q", namesContent)
	}
}

func testExampleOauth(t *testing.T, b Backend, lang string) {
	entries, err := b.ReadDir("/")
	if err != nil {
		t.Fatalf("ReadDir /: %v", err)
	}
	found := false
	for _, e := range entries {
		if e == "oauth" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'oauth' in root directory, got %v", entries)
	}

	helpContent, err := b.ReadFile("/oauth/help")
	if err != nil {
		t.Fatalf("ReadFile /oauth/help: %v", err)
	}
	if len(helpContent) == 0 {
		t.Error("/oauth/help returned empty content")
	}

	clientContent, err := b.ReadFile("/oauth/client")
	if err != nil {
		t.Fatalf("ReadFile /oauth/client: %v", err)
	}
	if len(clientContent) == 0 {
		t.Error("/oauth/client returned empty content")
	}

	deviceContent, err := b.ReadFile("/oauth/device")
	if err != nil {
		t.Fatalf("ReadFile /oauth/device: %v", err)
	}
	if len(deviceContent) == 0 {
		t.Error("/oauth/device returned empty content")
	}
}

func testExampleWikipedia(t *testing.T, b Backend) {
	entries, err := b.ReadDir("/")
	if err != nil {
		t.Fatalf("ReadDir /: %v", err)
	}
	expectedRoot := map[string]bool{"search": false, "result": false, "article": false, "bookmarks": false, "README.md": false}
	for _, e := range entries {
		expectedRoot[e] = true
	}
	for name, found := range expectedRoot {
		if !found {
			t.Errorf("expected root entry %q not found in %v", name, entries)
		}
	}

	readmeContent, err := b.ReadFile("/README.md")
	if err != nil {
		t.Fatalf("ReadFile /README.md: %v", err)
	}
	if len(readmeContent) == 0 {
		t.Error("README.md is empty")
	}

	// Search and bookmark operations — log non-fatal warnings if they fail
	searchData := "Go programming language"
	if err := b.WriteFile("/search", searchData); err != nil {
		t.Logf("Warning: WriteFile /search failed (expected if network unavailable): %v", err)
	} else {
		searchResult, err := b.ReadFile("/search")
		if err != nil {
			t.Logf("Warning: ReadFile /search after write failed: %v", err)
		} else {
			t.Logf("Search response: %s", searchResult)
		}
	}

	bookmarkData := "Test bookmark content"
	if err := b.WriteFile("/bookmarks/test.txt", bookmarkData); err != nil {
		t.Logf("Warning: WriteFile /bookmarks/test.txt failed (expected for some configs): %v", err)
	} else {
		bookmarkResult, err := b.ReadFile("/bookmarks/test.txt")
		if err != nil {
			t.Logf("Warning: ReadFile /bookmarks/test.txt failed: %v", err)
		} else if bookmarkResult != bookmarkData {
			t.Logf("Bookmark content mismatch: got %q, want %q", bookmarkResult, bookmarkData)
		} else {
			t.Log("Bookmark write/read successful")
		}
	}
}

// ---------------------------------------------------------------------------
// Transport list for parameterization
// ---------------------------------------------------------------------------

var allTransports = []struct {
	name string
	tr   transport
}{
	{"FUSE", transportFUSE},
	{"9pTCP", transport9pTCP},
	{"9pUnix", transport9pUnix},
	{"9pFUSE", transport9pFUSE},
	{"9pCLI", transport9pCLI},
	{"SSH", transportSSH},
}

// runExampleSuite runs the given example test against every transport and
// language variant.  lcList must have exactly 2 entries (lua + tengo).
func runExampleSuite(t *testing.T, lcList []exampleLangConfig) {
	t.Helper()
	for _, ec := range lcList {
		ec := ec
		for _, tp := range allTransports {
			tp := tp
			t.Run(ec.lang+"/"+tp.name, func(t *testing.T) {
				b := newExampleBackend(t, tp.tr, ec, ec.env)
				defer b.Close()
				runExampleTest(t, b, ec)
			})
		}
	}
}

// ---------------------------------------------------------------------------
// Top-level test functions — one per example
// ---------------------------------------------------------------------------

func TestExampleSimple(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	runExampleSuite(t, []exampleLangConfig{
		{lang: "lua", exampleDir: "simple", rootScript: "main.lua"},
		{lang: "tengo", exampleDir: "simple", rootScript: "main.tengo"},
	})
}

func TestExampleSimpler(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	runExampleSuite(t, []exampleLangConfig{
		{lang: "lua", exampleDir: "simpler", rootScript: "main.lua"},
		{lang: "tengo", exampleDir: "simpler", rootScript: "main.tengo"},
	})
}

func TestExampleUtilities(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	utilitiesEnv := map[string]string{
		"USER": "testuser", "HOME": "/home/testuser", "PATH": "/usr/bin:/bin",
		"MAX_ITEMS": "25", "GREETING": "Welcome", "DEBUG_MODE": "1",
	}
	runExampleSuite(t, []exampleLangConfig{
		{lang: "lua", exampleDir: "utilities", rootScript: "main.lua", env: utilitiesEnv},
		{lang: "tengo", exampleDir: "utilities", rootScript: "main.tengo", env: utilitiesEnv},
	})
}

func TestExampleSerialization(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	runExampleSuite(t, []exampleLangConfig{
		{lang: "lua", exampleDir: "serialization", rootScript: "main.lua"},
		{lang: "tengo", exampleDir: "serialization", rootScript: "main.tengo"},
	})
}

func TestExampleConversion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	runExampleSuite(t, []exampleLangConfig{
		{lang: "lua", exampleDir: "conversion", rootScript: "main.lua"},
		{lang: "tengo", exampleDir: "conversion", rootScript: "main.tengo"},
	})
}

func TestExampleHttp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	runExampleSuite(t, []exampleLangConfig{
		{lang: "lua", exampleDir: "http", rootScript: "main.lua"},
		{lang: "tengo", exampleDir: "http", rootScript: "main.tengo"},
	})
}

func TestExampleJq(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	runExampleSuite(t, []exampleLangConfig{
		{lang: "lua", exampleDir: "jq", rootScript: "main.lua"},
		{lang: "tengo", exampleDir: "jq", rootScript: "main.tengo"},
	})
}

func TestExampleOauth(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	runExampleSuite(t, []exampleLangConfig{
		{lang: "lua", exampleDir: "oauth", rootScript: "main.lua"},
		{lang: "tengo", exampleDir: "oauth", rootScript: "main.tengo"},
	})
}

func TestExampleWikipedia(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	runExampleSuite(t, []exampleLangConfig{
		{lang: "lua", exampleDir: "wikipedia", rootScript: "main.lua"},
		{lang: "tengo", exampleDir: "wikipedia", rootScript: "main.tengo"},
	})
}
