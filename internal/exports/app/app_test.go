package app

import (
	"os"
	"testing"
)

// mockBackend is a minimal implementation of Backend for testing.
type mockBackend struct {
	readHandlers    map[string]Handler
	writeHandlers   map[string]Handler
	createHandlers  map[string]Handler
	unlinkHandlers  map[string]Handler
	getAttrHandlers map[string]Handler
	lookupHandlers  map[string]Handler
	openHandlers    map[string]Handler
	readDirHandlers map[string]Handler
	logs            []string
	debugs          []string
	env             map[string]string
}

func newMockBackend() *mockBackend {
	return &mockBackend{
		readHandlers:    make(map[string]Handler),
		writeHandlers:   make(map[string]Handler),
		createHandlers:  make(map[string]Handler),
		unlinkHandlers:  make(map[string]Handler),
		getAttrHandlers: make(map[string]Handler),
		lookupHandlers:  make(map[string]Handler),
		openHandlers:    make(map[string]Handler),
		readDirHandlers: make(map[string]Handler),
		env:             make(map[string]string),
	}
}

func (m *mockBackend) RegisterRead(pathPattern string, handler Handler) error {
	m.readHandlers[pathPattern] = handler
	return nil
}

func (m *mockBackend) RegisterWrite(pathPattern string, handler Handler) error {
	m.writeHandlers[pathPattern] = handler
	return nil
}

func (m *mockBackend) RegisterCreate(pathPattern string, handler Handler) error {
	m.createHandlers[pathPattern] = handler
	return nil
}

func (m *mockBackend) RegisterUnlink(pathPattern string, handler Handler) error {
	m.unlinkHandlers[pathPattern] = handler
	return nil
}

func (m *mockBackend) RegisterGetAttr(pathPattern string, handler Handler) error {
	m.getAttrHandlers[pathPattern] = handler
	return nil
}

func (m *mockBackend) RegisterLookup(pathPattern string, handler Handler) error {
	m.lookupHandlers[pathPattern] = handler
	return nil
}

func (m *mockBackend) RegisterOpen(pathPattern string, handler Handler) error {
	m.openHandlers[pathPattern] = handler
	return nil
}

func (m *mockBackend) RegisterReadDir(pathPattern string, handler Handler) error {
	m.readDirHandlers[pathPattern] = handler
	return nil
}

func (m *mockBackend) Environment() map[string]string {
	return m.env
}

func (m *mockBackend) Log(msg string) {
	m.logs = append(m.logs, msg)
}

func (m *mockBackend) Debug(msg string) {
	m.debugs = append(m.debugs, msg)
}

func TestEnvironmentFromOS(t *testing.T) {
	// Set a test environment variable
	_ = os.Setenv("NK_TEST_ENV", "test_value")
	defer func() { _ = os.Unsetenv("NK_TEST_ENV") }()

	env := EnvironmentFromOS()

	// Should contain our test variable
	if val, ok := env["NK_TEST_ENV"]; !ok || val != "test_value" {
		t.Errorf("EnvironmentFromOS missing NK_TEST_ENV or wrong value: %v", env)
	}

	// Should contain PATH (almost always present)
	if _, ok := env["PATH"]; !ok {
		// On some systems PATH may not be set, but it's very rare.
		// We'll just log a warning.
		t.Log("PATH not found in environment (unusual)")
	}

	// Verify no empty keys
	for k := range env {
		if k == "" {
			t.Error("EnvironmentFromOS returned empty key")
		}
	}
}

func TestMockBackendImplementsInterface(t *testing.T) {
	var _ Backend = (*mockBackend)(nil)
}

func TestMockBackendRegistration(t *testing.T) {
	backend := newMockBackend()

	// Test registration of each operation type
	testHandler := "some_handler"

	if err := backend.RegisterRead("/read", testHandler); err != nil {
		t.Errorf("RegisterRead failed: %v", err)
	}
	if backend.readHandlers["/read"] != testHandler {
		t.Error("RegisterRead didn't store handler")
	}

	if err := backend.RegisterWrite("/write", testHandler); err != nil {
		t.Errorf("RegisterWrite failed: %v", err)
	}
	if backend.writeHandlers["/write"] != testHandler {
		t.Error("RegisterWrite didn't store handler")
	}

	if err := backend.RegisterCreate("/create", testHandler); err != nil {
		t.Errorf("RegisterCreate failed: %v", err)
	}
	if backend.createHandlers["/create"] != testHandler {
		t.Error("RegisterCreate didn't store handler")
	}

	if err := backend.RegisterUnlink("/unlink", testHandler); err != nil {
		t.Errorf("RegisterUnlink failed: %v", err)
	}
	if backend.unlinkHandlers["/unlink"] != testHandler {
		t.Error("RegisterUnlink didn't store handler")
	}

	if err := backend.RegisterGetAttr("/stat", testHandler); err != nil {
		t.Errorf("RegisterGetAttr failed: %v", err)
	}
	if backend.getAttrHandlers["/stat"] != testHandler {
		t.Error("RegisterGetAttr didn't store handler")
	}

	if err := backend.RegisterLookup("/lookup", testHandler); err != nil {
		t.Errorf("RegisterLookup failed: %v", err)
	}
	if backend.lookupHandlers["/lookup"] != testHandler {
		t.Error("RegisterLookup didn't store handler")
	}

	if err := backend.RegisterOpen("/open", testHandler); err != nil {
		t.Errorf("RegisterOpen failed: %v", err)
	}
	if backend.openHandlers["/open"] != testHandler {
		t.Error("RegisterOpen didn't store handler")
	}

	if err := backend.RegisterReadDir("/list", testHandler); err != nil {
		t.Errorf("RegisterReadDir failed: %v", err)
	}
	if backend.readDirHandlers["/list"] != testHandler {
		t.Error("RegisterReadDir didn't store handler")
	}
}

func TestMockBackendEnvironment(t *testing.T) {
	backend := newMockBackend()
	backend.env["CUSTOM"] = "value"

	env := backend.Environment()
	if env["CUSTOM"] != "value" {
		t.Errorf("Environment() returned wrong value: %v", env)
	}
}

func TestMockBackendLogging(t *testing.T) {
	backend := newMockBackend()

	backend.Log("info message")
	backend.Debug("debug message")

	if len(backend.logs) != 1 || backend.logs[0] != "info message" {
		t.Errorf("Log not recorded correctly: %v", backend.logs)
	}
	if len(backend.debugs) != 1 || backend.debugs[0] != "debug message" {
		t.Errorf("Debug not recorded correctly: %v", backend.debugs)
	}
}