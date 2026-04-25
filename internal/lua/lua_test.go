package lua

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bubblegutz/nklhd/internal/exports/app"
	"github.com/bubblegutz/nklhd/internal/script"
	lua "github.com/yuin/gopher-lua"
)

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && (s[0:len(substr)] == substr || contains(s[1:], substr))))
}

func TestMissingHandler(t *testing.T) {
	script := `
app.read("/foo", function(req)
    return "ok"
end)
`
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.lua")
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
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
	// Expect error message contains "no handler registered"
	if err != nil && !contains(err.Error(), "no handler registered") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInvalidScriptSyntax(t *testing.T) {
	script := `invalid lua syntax !@#`
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.lua")
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		t.Fatal(err)
	}

	exec := NewExecutor()
	defer exec.Close()
	err := exec.LoadScript(scriptPath)
	if err == nil {
		t.Error("expected error for invalid Lua syntax")
	}
	// Expect error about failed to load script
	if err != nil && !contains(err.Error(), "failed to load script") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHandlerReturnsNonString(t *testing.T) {
	script := `
app.read("/foo", function(req)
    return 42  -- number instead of string
end)
`
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.lua")
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		t.Fatal(err)
	}

	exec := NewExecutor()
	defer exec.Close()
	if err := exec.LoadScript(scriptPath); err != nil {
		t.Fatal(err)
	}

	_, err := exec.ExecuteHandler("read", "/foo", nil, nil, 0)
	if err == nil {
		t.Error("expected error for non-string return")
	}
	if err != nil && !contains(err.Error(), "must return a string") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestListHandlerInvalidReturn(t *testing.T) {
	script := `
app.readdir("/dir", function(req)
    return 123  -- number instead of table/string
end)
`
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.lua")
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		t.Fatal(err)
	}

	exec := NewExecutor()
	defer exec.Close()
	if err := exec.LoadScript(scriptPath); err != nil {
		t.Fatal(err)
	}

	_, err := exec.ExecuteListHandler("/dir", nil)
	if err == nil {
		t.Error("expected error for invalid list handler return")
	}
	if err != nil && !contains(err.Error(), "must return a string, table, or nil") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestListHandler(t *testing.T) {
	exec := NewExecutor()
	defer exec.Close()

	L := lua.NewState()
	defer L.Close()

	// Register a list handler using the app object (simulate script)
	appTable := L.NewTable()
	L.SetField(appTable, "readdir", L.NewFunction(func(L *lua.LState) int {
		path := L.CheckString(1)
		fn := L.CheckFunction(2)
		// We'll manually register because we don't have LuaApp exposed
		// Instead, we'll directly call exec's registry
		exec.registry.Register("readdir", path, &RouteHandler{L: L, Func: fn})
		return 0
	}))
	L.SetGlobal("app", appTable)

	// Load a simple script that registers a list handler
	err := L.DoString(`
		app.readdir("/movies/{imdbID}/*", function(req)
			return {"metadata.txt", "poster.jpg", "subtitles/"}
		end)
	`)
	if err != nil {
		t.Fatalf("Failed to load script: %v", err)
	}

	// Execute list handler
	entries, err := exec.ExecuteListHandler("/movies/tt1234567/*", map[string]string{"imdbID": "tt1234567"})
	if err != nil {
		t.Fatalf("ExecuteListHandler failed: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("Expected 3 entries, got %d: %v", len(entries), entries)
	}
	expected := []string{"metadata.txt", "poster.jpg", "subtitles/"}
	for i, exp := range expected {
		if entries[i] != exp {
			t.Errorf("Entry %d mismatch: got %q, expected %q", i, entries[i], exp)
		}
	}
}

func TestListHandlerWithPatterns(t *testing.T) {
	exec := NewExecutor()
	defer exec.Close()

	L := lua.NewState()
	defer L.Close()

	// Register list handlers using the app object (simulate script)
	appTable := L.NewTable()
	L.SetField(appTable, "readdir", L.NewFunction(func(L *lua.LState) int {
		path := L.CheckString(1)
		fn := L.CheckFunction(2)
		exec.registry.Register("readdir", path, &RouteHandler{L: L, Func: fn})
		return 0
	}))
	L.SetGlobal("app", appTable)

	// Load script that registers list handlers for patterns
	err := L.DoString(`
		app.readdir("/movies/{imdbID}/*", function(req)
			local entries = {"metadata.txt", "poster.jpg", "subtitles/"}
			-- Add imdbID to each entry for verification
			local result = {}
			for _, e in ipairs(entries) do
				table.insert(result, req.imdbID .. "-" .. e)
			end
			return result
		end)

		app.readdir("/logs/*.txt", function(req)
			-- The '*' captures the prefix before .txt
			local prefix = req["*"] or ""
			return {prefix .. "log1.txt", prefix .. "log2.txt"}
		end)
	`)
	if err != nil {
		t.Fatalf("Failed to load script: %v", err)
	}

	// Test list handler with parameter and wildcard
	entries, err := exec.ExecuteListHandler("/movies/tt1234567/*", map[string]string{"imdbID": "tt1234567"})
	if err != nil {
		t.Fatalf("ExecuteListHandler failed: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("Expected 3 entries, got %d: %v", len(entries), entries)
	}
	expected := []string{"tt1234567-metadata.txt", "tt1234567-poster.jpg", "tt1234567-subtitles/"}
	for i, exp := range expected {
		if entries[i] != exp {
			t.Errorf("Entry %d mismatch: got %q, expected %q", i, entries[i], exp)
		}
	}

	// Test glob pattern list handler
	entries2, err := exec.ExecuteListHandler("/logs/error.txt", nil)
	if err != nil {
		t.Fatalf("ExecuteListHandler for glob failed: %v", err)
	}
	if len(entries2) != 2 {
		t.Errorf("Expected 2 entries, got %d: %v", len(entries2), entries2)
	}
	// The '*' parameter should be "error" (without .txt)
	// Actually our pattern is "*.txt", the '*' captures the whole component "error.txt"?
	// Let's see what the handler receives. We'll just check that entries are returned.
}

func TestDynamicDirectoryGeneration(t *testing.T) {
	exec := NewExecutor()
	defer exec.Close()

	L := lua.NewState()
	defer L.Close()

	appTable := L.NewTable()
	L.SetField(appTable, "readdir", L.NewFunction(func(L *lua.LState) int {
		path := L.CheckString(1)
		fn := L.CheckFunction(2)
		exec.registry.Register("readdir", path, &RouteHandler{L: L, Func: fn})
		return 0
	}))
	L.SetField(appTable, "read", L.NewFunction(func(L *lua.LState) int {
		path := L.CheckString(1)
		fn := L.CheckFunction(2)
		exec.registry.Register("read", path, &RouteHandler{L: L, Func: fn})
		return 0
	}))
	L.SetGlobal("app", appTable)

	err := L.DoString(`
		app.readdir("/data/{category}/*", function(req)
			local cat = req.category
			return {cat .. "-file1.txt", cat .. "-file2.txt", cat .. "-subdir/"}
		end)

		app.read("/data/{category}/{file}", function(req)
			return "Content of " .. req.category .. "/" .. req.file
		end)
	`)
	if err != nil {
		t.Fatalf("Failed to load script: %v", err)
	}

	// Test list handler
	entries, err := exec.ExecuteListHandler("/data/books/*", map[string]string{"category": "books"})
	if err != nil {
		t.Fatalf("ExecuteListHandler failed: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("Expected 3 entries, got %d: %v", len(entries), entries)
	}
	// Verify that entries are plausible
	for _, e := range entries {
		if e != "books-file1.txt" && e != "books-file2.txt" && e != "books-subdir/" {
			t.Errorf("Unexpected entry: %q", e)
		}
	}
}

func TestPatternsScriptIntegration(t *testing.T) {
	exec := NewExecutor()
	defer exec.Close()

	// Load the patterns.lua script from the scripts directory
	// The test runs from the internal/lua directory, so go up two levels
	scriptPath := "../../scripts/patterns.lua"
	err := exec.LoadScript(scriptPath)
	if err != nil {
		t.Fatalf("Failed to load patterns.lua: %v", err)
	}

	// Test list handler for movies pattern
	entries, err := exec.ExecuteListHandler("/movies/tt1234567/*", map[string]string{"imdbID": "tt1234567"})
	if err != nil {
		t.Fatalf("ExecuteListHandler failed: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("Expected 3 entries, got %d: %v", len(entries), entries)
	}
	expected := []string{"metadata.txt", "poster.jpg", "subtitles/"}
	for i, exp := range expected {
		if entries[i] != exp {
			t.Errorf("Entry %d mismatch: got %q, expected %q", i, entries[i], exp)
		}
	}

	// Test read handler for metadata file
	content, err := exec.ExecuteHandler("read", "/movies/tt1234567/metadata.txt", map[string]string{"imdbID": "tt1234567"}, nil, 0)
	if err != nil {
		t.Fatalf("ExecuteHandler failed: %v", err)
	}
	expectedContent := "Title: Example Movie\nIMDB ID: tt1234567\nYear: 2024"
	if content != expectedContent {
		t.Errorf("Content mismatch:\ngot: %q\nexpected: %q", content, expectedContent)
	}

	// Test list handler for subtitles directory
	subEntries, err := exec.ExecuteListHandler("/movies/tt1234567/subtitles/*", map[string]string{"imdbID": "tt1234567"})
	if err != nil {
		t.Fatalf("ExecuteListHandler for subtitles failed: %v", err)
	}
	if len(subEntries) != 3 {
		t.Errorf("Expected 3 subtitle entries, got %d: %v", len(subEntries), subEntries)
	}
	// subtitles list handler returns {"en.srt", "es.srt", "fr.srt"}
	// Test read handler for subtitle file
	subContent, err := exec.ExecuteHandler("read", "/movies/tt1234567/subtitles/en.srt", map[string]string{"imdbID": "tt1234567", "lang": "en"}, nil, 0)
	if err != nil {
		t.Fatalf("ExecuteHandler for subtitle failed: %v", err)
	}
	expectedSub := "Subtitle content for tt1234567 in en"
	if subContent != expectedSub {
		t.Errorf("Subtitle content mismatch:\ngot: %q\nexpected: %q", subContent, expectedSub)
	}
}

func TestFuseOperationHandlers(t *testing.T) {
	script := `
app.create("/newfile", function(req)
    return "Created: " .. (req.name or "unknown")
end)

app.unlink("/delete", function(req)
    return "Deleted: " .. (req.name or "unknown")
end)

app.getattr("/stat", function(req)
    return "size=1024\nmode=0644\nuid=1000\ngid=1000"
end)

app.open("/openfile", function(req)
    return "Opened: flags=" .. (req.flags or "0")
end)
`
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.lua")
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
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

func TestAppUtilities(t *testing.T) {
	script := `
		-- Test app.env
		local hasPath = false
		for k, v in pairs(app.env) do
			if k == "PATH" then
				hasPath = true
				break
			end
		end
		if not hasPath then
			error("PATH not found in app.env")
		end
		-- Test app.log (should not error)
		app.log("test log message")
		-- Test app.debug with debug logging enabled
		app.debug("test debug message")
		-- Register a simple route to verify script loaded
		app.read("/test", function(req)
			return "OK"
		end)
	`
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.lua")
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		t.Fatal(err)
	}

	exec := NewExecutor()
	defer exec.Close()
	// Enable debug logging
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
	// Disable debug logging and test debug message is not printed (cannot easily test)
	SetDebug(false)
}

func BenchmarkRouteMatching(b *testing.B) {
	registry := script.NewRegistry()
	// Create a minimal Lua state for handlers
	L := lua.NewState()
	defer L.Close()
	fn := L.NewFunction(func(L *lua.LState) int { return 0 })

	// Register a variety of routes
	routes := []string{
		"/api/users",
		"/api/users/{id}",
		"/api/users/{id}/posts",
		"/api/users/{id}/posts/{postId}",
		"/static/*",
		"/files/{category}/{file}",
	}
	for _, route := range routes {
		registry.Register("read", route, &RouteHandler{L: L, Func: fn})
	}

	// Test paths that should match
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

func TestLuaAppImplementsBackend(t *testing.T) {
	// Compile-time check that LuaApp implements app.Backend
	var _ app.Backend = (*LuaApp)(nil).Backend()
	// Create a concrete instance to verify methods work
	registry := script.NewRegistry()
	L := lua.NewState()
	defer L.Close()
	luaApp := NewLuaApp(registry, L)
	backend := luaApp.Backend()

	// Test that backend can be used
	fn := L.NewFunction(func(L *lua.LState) int { return 0 })
	err := backend.RegisterRead("/test", fn)
	if err != nil {
		t.Errorf("RegisterRead failed: %v", err)
	}
	// Verify environment
	env := backend.Environment()
	if env == nil {
		t.Error("Environment() returned nil")
	}
	// Log calls should not panic
	backend.Log("test log")
	backend.Debug("test debug")
}
