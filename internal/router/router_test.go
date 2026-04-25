package router

import (
	"testing"
)

func TestComponentMatches(t *testing.T) {
	tests := []struct {
		pattern   string
		component string
		expected  bool
	}{
		// Exact matches
		{"file", "file", true},
		{"file", "other", false},
		// Prefix wildcard
		{"prefix*", "prefix123", true},
		{"prefix*", "prefi", false},
		{"prefix*", "otherprefix", false},
		// Suffix wildcard
		{"*suffix", "123suffix", true},
		{"*suffix", "suffix", true},
		{"*suffix", "suffixextra", false},
		// Contains wildcard
		{"*middle*", "abmiddlecd", true},
		{"*middle*", "middle", true},
		{"*middle*", "nomiddle", true},
		// Simple glob with single *
		{"prefix*suffix", "prefix123suffix", true},
		{"prefix*suffix", "prefixsuffix", true},
		{"prefix*suffix", "prefiXsuffix", false},
		// Question mark
		{"file?", "file1", true},
		{"file?", "file", false},
		{"file?", "file12", false},
		// Edge cases
		{"*", "*", true},
		{"*", "anything", true},
		{"", "", true},
		{"", "x", false},
	}

	for _, tt := range tests {
		result := ComponentMatches(tt.pattern, tt.component)
		if result != tt.expected {
			t.Errorf("ComponentMatches(%q, %q) = %v, expected %v", tt.pattern, tt.component, result, tt.expected)
		}
	}
}

func TestConvertGlobPattern(t *testing.T) {
	tests := []struct {
		pattern  string
		expected string
	}{
		// No glob characters
		{"/api/users", "/api/users"},
		{"/static/file.txt", "/static/file.txt"},
		// Simple wildcard
		{"/api/*", "/api/*"},
		{"/api/*/config", "/api/{_wildcard0:.*}/config"},
		{"/files/*.*", "/files/{_glob0:^.*\\..*$}"},
		// Glob patterns
		{"/api/file*.txt", "/api/{_glob0:^file.*\\.txt$}"},
		{"/logs/*.log", "/logs/{_glob0:^.*\\.log$}"},
		{"/data/*/*.json", "/data/{_wildcard0:.*}/{_glob0:^.*\\.json$}"},
		// Already parameterized patterns
		{"/api/{id}", "/api/{id}"},
		{"/api/{id:[0-9]+}", "/api/{id:[0-9]+}"},
		// Mixed
		{"/api/{id}/file*.txt", "/api/{id}/{_glob0:^file.*\\.txt$}"},
	}

	for _, tt := range tests {
		converted, _ := ConvertGlobPattern(tt.pattern)
		if converted != tt.expected {
			t.Errorf("ConvertGlobPattern(%q) = %q, expected %q", tt.pattern, converted, tt.expected)
		}
	}
}

func TestRouterRegisterAndFind(t *testing.T) {
	r := NewRouter()

	// Register some handlers
	r.Register("read", "/api/users", "readHandler")
	r.Register("write", "/api/users", "writeHandler")
	r.Register("read", "/api/users/{id}", "readUserHandler")
	r.Register("read", "/data/{category}/{item}", "dataHandler")

	// Test exact match
	handler, params, pattern := r.Find("read", "/api/users")
	if handler != "readHandler" {
		t.Errorf("Find read /api/users: handler = %v, expected readHandler", handler)
	}
	if len(params) != 0 {
		t.Errorf("Find read /api/users: params = %v, expected empty", params)
	}
	if pattern != "/api/users" {
		t.Errorf("Find read /api/users: pattern = %q, expected /api/users", pattern)
	}

	// Test param match
	handler, params, pattern = r.Find("read", "/api/users/123")
	if handler != "readUserHandler" {
		t.Errorf("Find read /api/users/123: handler = %v, expected readUserHandler", handler)
	}
	if params["id"] != "123" {
		t.Errorf("Find read /api/users/123: params[id] = %v, expected 123", params["id"])
	}
	if pattern != "/api/users/{id}" {
		t.Errorf("Find read /api/users/123: pattern = %q, expected /api/users/{id}", pattern)
	}

	// Test nested params
	handler, params, pattern = r.Find("read", "/data/books/title")
	if handler != "dataHandler" {
		t.Errorf("Find read /data/books/title: handler = %v, expected dataHandler", handler)
	}
	if params["category"] != "books" || params["item"] != "title" {
		t.Errorf("Find read /data/books/title: params = %v, expected category=books, item=title", params)
	}
	if pattern != "/data/{category}/{item}" {
		t.Errorf("Find read /data/books/title: pattern = %q, expected /data/{category}/{item}", pattern)
	}

	// Test operation mismatch
	handler, _, _ = r.Find("write", "/api/users/123")
	if handler != nil {
		t.Errorf("Find write /api/users/123: handler = %v, expected nil (no write handler)", handler)
	}

	// Test not found
	handler, _, _ = r.Find("read", "/nonexistent")
	if handler != nil {
		t.Errorf("Find read /nonexistent: handler = %v, expected nil", handler)
	}
}

func TestBuildTree(t *testing.T) {
	handlers := map[string]map[string]any{
		"read": {
			"/api/users":        "readUsers",
			"/api/users/{id}":   "readUser",
			"/data/{cat}/{item}": "readData",
		},
		"write": {
			"/api/users": "writeUsers",
		},
	}

	tree := BuildTree(handlers)

	// Check root
	if tree.Name != "" {
		t.Errorf("root.Name = %q, expected empty", tree.Name)
	}
	if tree.Type != NodeTypeDir {
		t.Errorf("root.Type = %v, expected NodeTypeDir", tree.Type)
	}

	// Check api node
	apiNode, ok := tree.Children["api"]
	if !ok {
		t.Fatal("missing api node")
	}
	if apiNode.Type != NodeTypeDir {
		t.Errorf("api.Type = %v, expected NodeTypeDir", apiNode.Type)
	}

	// Check api/users node
	usersNode, ok := apiNode.Children["users"]
	if !ok {
		t.Fatal("missing users node")
	}
	if usersNode.Pattern != "/api/users" {
		t.Errorf("users.Pattern = %q, expected /api/users", usersNode.Pattern)
	}
	if len(usersNode.Handlers) != 2 {
		t.Errorf("users.Handlers count = %d, expected 2", len(usersNode.Handlers))
	}
	if usersNode.Handlers["read"] != "readUsers" {
		t.Errorf("users.Handlers[read] = %v, expected readUsers", usersNode.Handlers["read"])
	}
	if usersNode.Handlers["write"] != "writeUsers" {
		t.Errorf("users.Handlers[write] = %v, expected writeUsers", usersNode.Handlers["write"])
	}

	// Check param node
	paramNode, ok := apiNode.Children["users"].Children["{id}"]
	if !ok {
		t.Fatal("missing {id} param node")
	}
	if paramNode.Type != NodeTypeParam {
		t.Errorf("paramNode.Type = %v, expected NodeTypeParam", paramNode.Type)
	}
	if paramNode.Pattern != "/api/users/{id}" {
		t.Errorf("paramNode.Pattern = %q, expected /api/users/{id}", paramNode.Pattern)
	}
	if paramNode.Handlers["read"] != "readUser" {
		t.Errorf("paramNode.Handlers[read] = %v, expected readUser", paramNode.Handlers["read"])
	}

	// Test FindNode
	components := []string{"api", "users", "123"}
	node, params := tree.FindNode(components)
	if node == nil {
		t.Fatal("FindNode returned nil")
	}
	if node.GetPattern() != "/api/users/{id}" {
		t.Errorf("FindNode pattern = %q, expected /api/users/{id}", node.GetPattern())
	}
	if params["id"] != "123" {
		t.Errorf("FindNode params[id] = %q, expected 123", params["id"])
	}
}

func TestNodeListChildren(t *testing.T) {
	handlers := map[string]map[string]any{
		"read": {
			"/a/b": "handler1",
			"/a/c": "handler2",
			"/a/{id}": "handler3",
		},
	}
	tree := BuildTree(handlers)
	aNode := tree.Children["a"]
	children := aNode.ListChildren()
	expected := map[string]bool{"b": true, "c": true, "{id}": true}
	if len(children) != len(expected) {
		t.Errorf("ListChildren returned %v, expected %v", children, expected)
	}
	for _, child := range children {
		if !expected[child] {
			t.Errorf("Unexpected child %q", child)
		}
	}
}
func TestGlobPatternTree(t *testing.T) {
	handlers := map[string]map[string]any{
		"read": {
			"/api/file*.txt": "handler1",
			"/api/*/config": "handler2",
		},
	}
	tree := BuildTree(handlers)

	// Check glob node exists
	apiNode := tree.Children["api"]
	if apiNode == nil {
		t.Fatal("missing api node")
	}
	// Should have children: "file*.txt" and "*"
	if len(apiNode.Children) != 2 {
		t.Errorf("api should have 2 children, got %d", len(apiNode.Children))
	}
	// Check file*.txt node type
	globNode, ok := apiNode.Children["file*.txt"]
	if !ok {
		t.Fatal("missing file*.txt child")
	}
	if globNode.Type != NodeTypeGlob {
		t.Errorf("file*.txt type should be Glob, got %v", globNode.Type)
	}
	// Check wildcard node
	wildNode, ok := apiNode.Children["*"]
	if !ok {
		t.Fatal("missing * child")
	}
	if wildNode.Type != NodeTypeWildcard {
		t.Errorf("* type should be Wildcard, got %v", wildNode.Type)
	}
	// Wildcard should have child "config"
	configNode, ok := wildNode.Children["config"]
	if !ok {
		t.Fatal("missing config child under *")
	}
	if configNode.Type != NodeTypeFile {
		t.Errorf("config type should be File, got %v", configNode.Type)
	}
}

func TestBuildTreeComprehensive(t *testing.T) {
	handlers := map[string]map[string]any{
		"read": {
			"/api/users":        "readUsers",
			"/api/users/{id}":   "readUser",
			"/data/files/{name}/content": "readContent",
		},
		"write": {
			"/api/users": "writeUsers",
		},
	}
	tree := BuildTree(handlers)

	// Check root node
	if tree.Name != "" {
		t.Errorf("root name expected empty, got %s", tree.Name)
	}
	if tree.Type != NodeTypeDir {
		t.Errorf("root type expected dir, got %v", tree.Type)
	}
	if len(tree.Children) != 2 {
		t.Errorf("root should have 2 children (api, data), got %d", len(tree.Children))
	}

	// Check api child
	api, ok := tree.Children["api"]
	if !ok {
		t.Fatal("missing 'api' child")
	}
	if api.Type != NodeTypeDir {
		t.Errorf("api type should be dir, got %v", api.Type)
	}
	if len(api.Children) != 1 {
		t.Errorf("api should have 1 child (users), got %d", len(api.Children))
	}

	// Check api/users child
	users, ok := api.Children["users"]
	if !ok {
		t.Fatal("missing 'users' child under api")
	}
	if users.Type != NodeTypeDir {
		t.Errorf("users type should be dir (has {id} child), got %v", users.Type)
	}
	if users.Pattern != "/api/users" {
		t.Errorf("users pattern mismatch: %s", users.Pattern)
	}
	if len(users.Handlers) != 2 {
		t.Errorf("users should have 2 handlers (read, write), got %d", len(users.Handlers))
	}
	if users.Handlers["read"] != "readUsers" || users.Handlers["write"] != "writeUsers" {
		t.Error("missing read or write handler")
	}

	// Check api/users/{id} parameter child
	paramChild, ok := api.Children["users"].Children["{id}"]
	if !ok {
		t.Fatal("missing '{id}' child under users")
	}
	if paramChild.Type != NodeTypeParam {
		t.Errorf("{id} type should be param, got %v", paramChild.Type)
	}
	if paramChild.Pattern != "/api/users/{id}" {
		t.Errorf("pattern mismatch: %s", paramChild.Pattern)
	}
	if len(paramChild.Handlers) != 1 || paramChild.Handlers["read"] != "readUser" {
		t.Error("missing read handler for {id}")
	}

	// Check data/files/{name}/content
	data, ok := tree.Children["data"]
	if !ok {
		t.Fatal("missing 'data' child")
	}
	files, ok := data.Children["files"]
	if !ok {
		t.Fatal("missing 'files' child under data")
	}
	nameParam, ok := files.Children["{name}"]
	if !ok {
		t.Fatal("missing '{name}' child under files")
	}
	if nameParam.Type != NodeTypeParam {
		t.Errorf("{name} type should be param, got %v", nameParam.Type)
	}
	content, ok := nameParam.Children["content"]
	if !ok {
		t.Fatal("missing 'content' child under {name}")
	}
	if content.Type != NodeTypeFile {
		t.Errorf("content type should be file, got %v", content.Type)
	}
	if content.Pattern != "/data/files/{name}/content" {
		t.Errorf("pattern mismatch: %s", content.Pattern)
	}
}

func TestFindNodeComprehensive(t *testing.T) {
	handlers := map[string]map[string]any{
		"read": {
			"/api/users/{id}": "readUser",
			"/data/{category}/{item}": "readItem",
		},
	}
	tree := BuildTree(handlers)

	// Test exact match (static)
	components := []string{"api", "users", "123"}
	node, params := tree.FindNode(components)
	if node == nil {
		t.Fatal("FindNode returned nil for /api/users/123")
	}
	if node.GetPattern() != "/api/users/{id}" {
		t.Errorf("pattern mismatch: %s", node.GetPattern())
	}
	if params["id"] != "123" {
		t.Errorf("param id expected 123, got %s", params["id"])
	}
	if len(params) != 1 {
		t.Errorf("expected 1 param, got %d", len(params))
	}

	// Test nested parameter
	components2 := []string{"data", "books", "title"}
	node2, params2 := tree.FindNode(components2)
	if node2 == nil {
		t.Fatal("FindNode returned nil for /data/books/title")
	}
	if node2.GetPattern() != "/data/{category}/{item}" {
		t.Errorf("pattern mismatch: %s", node2.GetPattern())
	}
	if params2["category"] != "books" || params2["item"] != "title" {
		t.Errorf("params mismatch: %v", params2)
	}

	// Test not found
	components3 := []string{"nonexistent"}
	node3, _ := tree.FindNode(components3)
	if node3 != nil {
		t.Error("FindNode should return nil for nonexistent path")
	}
}
