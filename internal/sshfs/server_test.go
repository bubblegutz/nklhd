package sshfs

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bubblegutz/nklhd/internal/router"
	"github.com/charmbracelet/ssh"
)

// testHandler implements router.ServeHandler for testing.
type testHandler struct{}

func (t *testHandler) Serve(req *router.Request) (*router.Response, error) {
	return &router.Response{Data: "ok"}, nil
}

// TestNewServer verifies that the server can be created with a router.
func TestNewServer(t *testing.T) {
	r := router.NewRouter()
	s := NewServer(r, "", "", "", false)
	if s == nil {
		t.Fatal("NewServer returned nil")
	}
	if s.router != r {
		t.Error("NewServer did not store the router")
	}
}

// TestNewServerWithOptions verifies all options are stored correctly.
func TestNewServerWithOptions(t *testing.T) {
	r := router.NewRouter()
	s := NewServer(r, "localhost:2222", "/tmp/keys", "/tmp/hostkey", true)
	if s == nil {
		t.Fatal("NewServer returned nil")
	}
	if s.addr != "localhost:2222" {
		t.Errorf("addr = %q, want %q", s.addr, "localhost:2222")
	}
	if s.authorizedKeys != "/tmp/keys" {
		t.Errorf("authorizedKeys = %q, want %q", s.authorizedKeys, "/tmp/keys")
	}
	if s.hostKeyPath != "/tmp/hostkey" {
		t.Errorf("hostKeyPath = %q, want %q", s.hostKeyPath, "/tmp/hostkey")
	}
	if !s.debug {
		t.Error("debug should be true")
	}
}

// TestGenerateHostKey verifies that generateHostKey returns valid PEM-encoded Ed25519 key.
func TestGenerateHostKey(t *testing.T) {
	pemBytes, err := generateHostKey()
	if err != nil {
		t.Fatalf("generateHostKey failed: %v", err)
	}
	if len(pemBytes) == 0 {
		t.Fatal("generateHostKey returned empty bytes")
	}
	// Verify PEM format
	block, rest := pem.Decode(pemBytes)
	if block == nil {
		t.Fatal("generateHostKey did not return valid PEM data")
	}
	if len(rest) > 0 {
		t.Error("generateHostKey returned trailing data after PEM block")
	}
	if block.Type != "PRIVATE KEY" {
		t.Errorf("PEM type = %q, want %q", block.Type, "PRIVATE KEY")
	}
	// Verify it can be parsed as Ed25519
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("failed to parse generated private key: %v", err)
	}
	if _, ok := key.(ed25519.PrivateKey); !ok {
		t.Fatalf("generated key is not Ed25519, got %T", key)
	}
}

// TestGenerateHostKeyUnique verifies two calls produce different keys.
func TestGenerateHostKeyUnique(t *testing.T) {
	k1, err := generateHostKey()
	if err != nil {
		t.Fatal(err)
	}
	k2, err := generateHostKey()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(k1, k2) {
		t.Error("generateHostKey produced identical keys on consecutive calls")
	}
}

// TestRouterFileInfo verifies the routerFileInfo implements os.FileInfo correctly.
func TestRouterFileInfo(t *testing.T) {
	now := time.Now()
	fi := &routerFileInfo{
		name:  "test.txt",
		size:  1024,
		mode:  0644,
		mod:   now,
		isDir: false,
	}
	if fi.Name() != "test.txt" {
		t.Errorf("Name = %q, want %q", fi.Name(), "test.txt")
	}
	if fi.Size() != 1024 {
		t.Errorf("Size = %d, want %d", fi.Size(), 1024)
	}
	if fi.Mode() != 0644 {
		t.Errorf("Mode = %o, want %o", fi.Mode(), 0644)
	}
	if !fi.ModTime().Equal(now) {
		t.Errorf("ModTime = %v, want %v", fi.ModTime(), now)
	}
	if fi.IsDir() {
		t.Error("IsDir should be false")
	}
	if fi.Sys() != nil {
		t.Error("Sys should be nil")
	}
}

// TestRouterFileInfoIsDir verifies IsDir reports true when mode has dir bit.
func TestRouterFileInfoIsDir(t *testing.T) {
	fi := &routerFileInfo{
		name:  "mydir",
		mode:  os.ModeDir | 0755,
		isDir: false,
	}
	if !fi.IsDir() {
		t.Error("IsDir should be true when mode has ModeDir")
	}
}

// TestNamedFileInfo verifies the namedFileInfo implements os.FileInfo correctly.
func TestNamedFileInfo(t *testing.T) {
	fi := &namedFileInfo{name: "file.txt", isDir: false}
	if fi.Name() != "file.txt" {
		t.Errorf("Name = %q, want %q", fi.Name(), "file.txt")
	}
	if fi.Size() != 0 {
		t.Errorf("Size = %d, want 0", fi.Size())
	}
	if fi.Mode() != 0644 {
		t.Errorf("Mode = %o, want %o", fi.Mode(), 0644)
	}
	if fi.ModTime().IsZero() {
		t.Error("ModTime should not be zero")
	}
	if fi.IsDir() {
		t.Error("IsDir should be false")
	}
	if fi.Sys() != nil {
		t.Error("Sys should be nil")
	}

	dirFi := &namedFileInfo{name: "dir", isDir: true}
	if !dirFi.IsDir() {
		t.Error("IsDir should be true for directory entry")
	}
}

// TestListerAtAdapter verifies ListAt behavior with offset and EOF.
func TestListerAtAdapter(t *testing.T) {
	items := []os.FileInfo{
		&namedFileInfo{name: "a.txt"},
		&namedFileInfo{name: "b.txt"},
		&namedFileInfo{name: "c.txt"},
	}
	la := &listerAtAdapter{items: items}

	// Test: read all at offset 0
	buf := make([]os.FileInfo, 3)
	n, err := la.ListAt(buf, 0)
	if err != nil || n != 3 {
		t.Fatalf("ListAt(0) = %d, %v; want 3, nil", n, err)
	}
	for i, fi := range buf[:n] {
		if fi.Name() != items[i].Name() {
			t.Errorf("buf[%d].Name = %q, want %q", i, fi.Name(), items[i].Name())
		}
	}

	// Test: partial read from middle offset
	buf2 := make([]os.FileInfo, 2)
	n, err = la.ListAt(buf2, 1)
	if err != io.EOF || n != 2 {
		t.Fatalf("ListAt(1) = %d, %v; want 2, io.EOF", n, err)
	}
	if buf2[0].Name() != "b.txt" {
		t.Errorf("buf2[0].Name = %q, want %q", buf2[0].Name(), "b.txt")
	}
	if buf2[1].Name() != "c.txt" {
		t.Errorf("buf2[1].Name = %q, want %q", buf2[1].Name(), "c.txt")
	}

	// Test: offset beyond end
	buf3 := make([]os.FileInfo, 1)
	n, err = la.ListAt(buf3, 10)
	if err != io.EOF || n != 0 {
		t.Fatalf("ListAt(10) = %d, %v; want 0, io.EOF", n, err)
	}
}

// TestListerAtAdapterEmpty verifies ListAt on empty list returns EOF.
func TestListerAtAdapterEmpty(t *testing.T) {
	la := &listerAtAdapter{items: []os.FileInfo{}}
	buf := make([]os.FileInfo, 1)
	n, err := la.ListAt(buf, 0)
	if err != io.EOF || n != 0 {
		t.Fatalf("ListAt on empty = %d, %v; want 0, io.EOF", n, err)
	}
}

// TestWriteBuffer verifies WriteAt stores data and sends to router.
func TestWriteBuffer(t *testing.T) {
	r := router.NewRouter()
	handler := &testHandler{}
	r.Register("write", "/test", handler)

	wb := &writeBuffer{r: r, path: "/test"}
	n, err := wb.WriteAt([]byte("hello"), 0)
	if err != nil {
		t.Fatalf("WriteAt failed: %v", err)
	}
	if n != 5 {
		t.Errorf("WriteAt returned %d, want 5", n)
	}

	// Write more data at an offset to test growth
	n, err = wb.WriteAt([]byte(" world"), 5)
	if err != nil {
		t.Fatalf("WriteAt (second) failed: %v", err)
	}
	if n != 6 {
		t.Errorf("WriteAt (second) returned %d, want 6", n)
	}
}

// TestWriteBufferConcurrent verifies writeBuffer handles concurrent writes safely.
func TestWriteBufferConcurrent(t *testing.T) {
	r := router.NewRouter()
	handler := &testHandler{}
	r.Register("write", "/concurrent", handler)

	wb := &writeBuffer{r: r, path: "/concurrent"}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			wb.WriteAt([]byte("x"), int64(i))
		}(i)
	}
	wg.Wait()
}

// TestReadSCPLine verifies readSCPLine reads newline-terminated lines.
func TestReadSCPLine(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello\n", "hello"},
		{"C0644 123 file.txt\n", "C0644 123 file.txt"},
		{"E\n", "E"},
		{"\n", ""},
		{"multi\nline\n", "multi"},
	}

	for _, tt := range tests {
		session := &mockSession{reader: strings.NewReader(tt.input)}
		got, err := readSCPLine(session)
		if err != nil {
			t.Fatalf("readSCPLine(%q) error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("readSCPLine(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestReadSCPLineEOF verifies readSCPLine returns error on unexpected EOF.
func TestReadSCPLineEOF(t *testing.T) {
	session := &mockSession{reader: strings.NewReader("no newline here")}
	_, err := readSCPLine(session)
	if err == nil {
		t.Error("readSCPLine should return error on missing newline")
	}
}

// TestSendSCPError verifies error message format.
func TestSendSCPError(t *testing.T) {
	var buf bytes.Buffer
	session := &mockSession{writer: &buf}
	sendSCPError(session, "test error")
	expected := "\x01test error\n\x02"
	if buf.String() != expected {
		t.Errorf("sendSCPError = %q, want %q", buf.String(), expected)
	}
}

// mockSession implements a minimal ssh.Session for testing SCP functions.
type mockSession struct {
	reader io.Reader
	writer io.Writer
}

func (m *mockSession) Read(p []byte) (int, error) {
	if m.reader != nil {
		return m.reader.Read(p)
	}
	return 0, io.EOF
}

func (m *mockSession) Write(p []byte) (int, error) {
	if m.writer != nil {
		return m.writer.Write(p)
	}
	return len(p), nil
}

func (m *mockSession) Close() error                                  { return nil }
func (m *mockSession) CloseWrite() error                                 { return nil }
func (m *mockSession) Stderr() io.ReadWriter                           { return nil }
func (m *mockSession) SendRequest(name string, wantReply bool, payload []byte) (bool, error) { return false, nil }
func (m *mockSession) RawCommand() string                              { return "" }
func (m *mockSession) PublicKey() ssh.PublicKey                        { return nil }
func (m *mockSession) Context() ssh.Context                            { return nil }
func (m *mockSession) Exit(code int) error                             { return nil }
func (m *mockSession) Command() []string                               { return nil }
func (m *mockSession) Pty() (ssh.Pty, <-chan ssh.Window, bool)         { return ssh.Pty{}, nil, false }
func (m *mockSession) Signals(c chan<- ssh.Signal)                     {}
func (m *mockSession) Break(c chan<- bool)                             {}
func (m *mockSession) Subsystem() string                               { return "" }
func (m *mockSession) User() string                                    { return "" }
func (m *mockSession) RemoteAddr() net.Addr                            { return nil }
func (m *mockSession) LocalAddr() net.Addr                             { return nil }
func (m *mockSession) Environ() []string                               { return nil }
func (m *mockSession) EmulatedPty() bool                               { return false }
func (m *mockSession) Permissions() ssh.Permissions                    { return ssh.Permissions{} }
func (m *mockSession) Environment() []string                           { return nil }
func (m *mockSession) DisablePTY()                                     {}
func (m *mockSession) Unsignaled() bool                                { return false }
func (m *mockSession) SendSignal(signal any) error                     { return nil }
