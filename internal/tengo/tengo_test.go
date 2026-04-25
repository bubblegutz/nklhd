package tengo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bubblegutz/nklhd/internal/script"
	"github.com/d5/tengo/v2"
)

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && (s[0:len(substr)] == substr || contains(s[1:], substr))))
}

func TestMissingHandler(t *testing.T) {
	scriptContent := `
app.read("/foo", func(req) {
    return "ok"
})
`
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.tengo")
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0644); err != nil {
		t.Fatal(err)
	}

	exec := NewExecutor()
	defer exec.Close()
	if err := exec.LoadScript(scriptPath); err != nil {
		t.Fatal(err)
	}

	// Try to execute a write handler on same path (not registered)
	_, err := exec.ExecuteHandler("write", "/foo", nil, []byte("data"), 0)
	if err == nil {
		t.Error("expected error for missing handler")
	}
	if err != nil && !contains(err.Error(), "no handler registered") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInvalidScriptSyntax(t *testing.T) {
	scriptContent := `invalid tengo syntax @@@`
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.tengo")
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0644); err != nil {
		t.Fatal(err)
	}

	exec := NewExecutor()
	defer exec.Close()
	err := exec.LoadScript(scriptPath)
	if err == nil {
		t.Error("expected error for invalid Tengo syntax")
	}
	if err != nil && !contains(err.Error(), "failed to load script") && !contains(err.Error(), "failed to extract constants") && !contains(err.Error(), "failed to compile script") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHandlerReturnsNonString(t *testing.T) {
	scriptContent := `
app.read("/foo", func(req) {
    return 42  // number instead of string
})
`
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.tengo")
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0644); err != nil {
		t.Fatal(err)
	}

	exec := NewExecutor()
	defer exec.Close()
	if err := exec.LoadScript(scriptPath); err != nil {
		t.Fatal(err)
	}

	result, err := exec.ExecuteHandler("read", "/foo", nil, nil, 0)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// Tengo converts non-string returns to their string representation
	if result != "42" {
		t.Errorf("expected '42', got %q", result)
	}
}

func TestListHandler(t *testing.T) {
	registry := script.NewRegistry()

	scriptSrc := []byte(`
		app.readdir("/dir", func(req) {
			return "a.txt\nb.txt\nsubdir/"
		})
	`)

	// Extract constants so RouteHandler can pass them to handler VMs
	constants, err := extractConstants(scriptSrc, nil)
	if err != nil {
		t.Fatalf("Failed to extract constants: %v", err)
	}

	L := tengo.NewScript(scriptSrc)

	// Manually register handlers by directly using TengoApp
	tengoApp := NewTengoApp(registry, constants, nil, nil)
	appMap := make(map[string]tengo.Object)
	appMap["readdir"] = &tengo.UserFunction{
		Name: "readdir",
		Value: func(args ...tengo.Object) (tengo.Object, error) {
			path := args[0].(*tengo.String).Value
			fn := args[1].(*tengo.CompiledFunction)
			tengoApp.RegisterList(path, fn)
			return tengo.UndefinedValue, nil
		},
	}
	L.Add("app", &tengo.ImmutableMap{Value: appMap})
	_, err = L.Run()
	if err != nil {
		t.Fatalf("Failed to load script: %v", err)
	}

	// Execute list handler via ExecuteListHandler
	exec := &Executor{registry: registry}
	entries, err := exec.ExecuteListHandler("/dir", nil)
	if err != nil {
		t.Fatalf("ExecuteListHandler failed: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("Expected 3 entries, got %d: %v", len(entries), entries)
	}
	expected := []string{"a.txt", "b.txt", "subdir/"}
	for i, exp := range expected {
		if entries[i] != exp {
			t.Errorf("Entry %d mismatch: got %q, expected %q", i, entries[i], exp)
		}
	}
}

func TestListHandlerEmpty(t *testing.T) {
	scriptContent := `
app.readdir("/empty", func(req) {
    return ""
})
`
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.tengo")
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0644); err != nil {
		t.Fatal(err)
	}

	exec := NewExecutor()
	defer exec.Close()
	if err := exec.LoadScript(scriptPath); err != nil {
		t.Fatal(err)
	}

	entries, err := exec.ExecuteListHandler("/empty", nil)
	if err != nil {
		t.Fatalf("ExecuteListHandler failed: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("Expected 0 entries, got %d: %v", len(entries), entries)
	}
}

func TestAppUtilities(t *testing.T) {
	scriptContent := `
app.log("test log message")
app.debug("test debug message")

app.read("/test", func(req) {
    return "OK"
})
`
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.tengo")
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0644); err != nil {
		t.Fatal(err)
	}

	exec := NewExecutor()
	defer exec.Close()
	SetDebug(true)
	if err := exec.LoadScript(scriptPath); err != nil {
		t.Fatalf("failed to load script: %v", err)
	}
	// Verify route was registered
	content, err := exec.ExecuteHandler("read", "/test", nil, nil, 0)
	if err != nil {
		t.Fatalf("failed to execute handler: %v", err)
	}
	if content != "OK" {
		t.Errorf("expected OK, got %q", content)
	}
}

func TestFuseOperationHandlers(t *testing.T) {
	scriptContent := `
app.create("/newfile", func(req) {
    return "Created: unknown"
})

app.unlink("/delete", func(req) {
    return "Deleted: unknown"
})

app.getattr("/stat", func(req) {
    return "size=1024\nmode=0644\nuid=1000\ngid=1000"
})

app.open("/openfile", func(req) {
    return "Opened: flags=0"
})
`
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.tengo")
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0644); err != nil {
		t.Fatal(err)
	}

	exec := NewExecutor()
	defer exec.Close()
	if err := exec.LoadScript(scriptPath); err != nil {
		t.Fatalf("failed to load script: %v", err)
	}

	// Test create handler
	createResp, err := exec.ExecuteHandler("create", "/newfile", nil, nil, 0)
	if err != nil {
		t.Fatalf("ExecuteHandler for create failed: %v", err)
	}
	if !contains(createResp, "Created") {
		t.Errorf("Create response missing 'Created': %q", createResp)
	}

	// Test unlink handler
	unlinkResp, err := exec.ExecuteHandler("unlink", "/delete", nil, nil, 0)
	if err != nil {
		t.Fatalf("ExecuteHandler for unlink failed: %v", err)
	}
	if !contains(unlinkResp, "Deleted") {
		t.Errorf("Unlink response missing 'Deleted': %q", unlinkResp)
	}

	// Test getattr handler
	attrResp, err := exec.ExecuteHandler("getattr", "/stat", nil, nil, 0)
	if err != nil {
		t.Fatalf("ExecuteHandler for getattr failed: %v", err)
	}
	if !contains(attrResp, "size=1024") || !contains(attrResp, "mode=0644") {
		t.Errorf("Getattr response missing expected attributes: %q", attrResp)
	}

	// Test open handler
	openResp, err := exec.ExecuteHandler("open", "/openfile", nil, nil, 0)
	if err != nil {
		t.Fatalf("ExecuteHandler for open failed: %v", err)
	}
	if !contains(openResp, "Opened") {
		t.Errorf("Open response missing 'Opened': %q", openResp)
	}
}

func TestRouteRegistrationWithScript(t *testing.T) {
	scriptContent := `
app.read("/api/users", func(req) {
    return "User list"
})

app.read("/api/users/{id}", func(req) {
    return "User: " + req.id
})

app.write("/api/users", func(req) {
    return "User created"
})
`
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.tengo")
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0644); err != nil {
		t.Fatal(err)
	}

	exec := NewExecutor()
	defer exec.Close()
	if err := exec.LoadScript(scriptPath); err != nil {
		t.Fatalf("failed to load script: %v", err)
	}

	// Test read handler without params
	resp, err := exec.ExecuteHandler("read", "/api/users", nil, nil, 0)
	if err != nil {
		t.Fatalf("ExecuteHandler failed: %v", err)
	}
	if resp != "User list" {
		t.Errorf("expected 'User list', got %q", resp)
	}

	// Test read handler with path param
	resp, err = exec.ExecuteHandler("read", "/api/users/123", map[string]string{"id": "123"}, nil, 0)
	if err != nil {
		t.Fatalf("ExecuteHandler with param failed: %v", err)
	}
	if resp != "User: 123" {
		t.Errorf("expected 'User: 123', got %q", resp)
	}

	// Test write handler
	resp, err = exec.ExecuteHandler("write", "/api/users", nil, []byte("data"), 0)
	if err != nil {
		t.Fatalf("ExecuteHandler for write failed: %v", err)
	}
	if resp != "User created" {
		t.Errorf("expected 'User created', got %q", resp)
	}
}

func BenchmarkRouteMatching(b *testing.B) {
	registry := script.NewRegistry()

	benchSrc := []byte(`
		app.read("/api/users", func(req) { return "" })
		app.read("/api/users/{id}", func(req) { return "" })
		app.read("/api/users/{id}/posts", func(req) { return "" })
		app.read("/api/users/{id}/posts/{postId}", func(req) { return "" })
		app.read("/static/*", func(req) { return "" })
		app.read("/files/{category}/{file}", func(req) { return "" })
	`)

	// Extract constants so RouteHandler can pass them to handler VMs
	constants, err := extractConstants(benchSrc, nil)
	if err != nil {
		b.Fatalf("Failed to extract constants: %v", err)
	}

	benchScript := tengo.NewScript(benchSrc)

	tengoApp := NewTengoApp(registry, constants, nil, nil)
	appMap := make(map[string]tengo.Object)
	appMap["read"] = &tengo.UserFunction{
		Name: "read",
		Value: func(args ...tengo.Object) (tengo.Object, error) {
			path := args[0].(*tengo.String).Value
			fn := args[1].(*tengo.CompiledFunction)
			tengoApp.RegisterRead(path, fn)
			return tengo.UndefinedValue, nil
		},
	}
	benchScript.Add("app", &tengo.ImmutableMap{Value: appMap})
	_, err = benchScript.Run()
	if err != nil {
		b.Fatalf("Failed to load benchmark routes: %v", err)
	}

	testPaths := []string{
		"/api/users",
		"/api/users/123",
		"/api/users/456/posts",
		"/api/users/789/posts/999",
		"/static/css/style.css",
		"/files/images/photo.jpg",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := testPaths[i%len(testPaths)]
		registry.FindHandler("read", path)
	}
}

func TestJsonModuleInHandler(t *testing.T) {
	scriptContent := `
app.read("/encode", func(req) {
    data := {name: "alice", age: 30}
    return string(req.json.encode(data))
})

app.read("/decode", func(req) {
    decoded := req.json.decode(req.data)
    return decoded.name
})
`
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.tengo")
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0644); err != nil {
		t.Fatal(err)
	}

	exec := NewExecutor()
	defer exec.Close()
	if err := exec.LoadScript(scriptPath); err != nil {
		t.Fatalf("failed to load script: %v", err)
	}

	// Test json.encode in handler via req
	result, err := exec.ExecuteHandler("read", "/encode", nil, nil, 0)
	if err != nil {
		t.Fatalf("ExecuteHandler for /encode failed: %v", err)
	}
	if !contains(result, "alice") || !contains(result, "30") {
		t.Errorf("encode result should contain name and age, got: %q", result)
	}

	// Test json.decode in handler via req
	result, err = exec.ExecuteHandler("read", "/decode", nil, []byte(`{"name":"bob","age":25}`), 0)
	if err != nil {
		t.Fatalf("ExecuteHandler for /decode failed: %v", err)
	}
	if result != "bob" {
		t.Errorf("expected 'bob', got %q", result)
	}
}
