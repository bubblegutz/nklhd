package ninep

import (
	"testing"

	"github.com/bubblegutz/nklhd/internal/router"
	"github.com/knusbaum/go9p/fs"
)

// testHandler implements router.ServeHandler for testing.
type testHandler struct{}

func (t *testHandler) Serve(req *router.Request) (*router.Response, error) {
	return &router.Response{Data: "ok"}, nil
}

// TestNewServer verifies that the server can be created with a router.
func TestNewServer(t *testing.T) {
	r := router.NewRouter()
	s := NewServer(r, false)
	if s == nil {
		t.Fatal("NewServer returned nil")
	}
	if s.router != r {
		t.Error("NewServer did not store the router")
	}
	if s.ninepFS == nil {
		t.Error("NewServer did not create a 9p FS")
	}
	if s.root == nil {
		t.Error("NewServer did not create a root directory")
	}
}

// TestNewServerDebug verifies debug mode is passed through.
func TestNewServerDebug(t *testing.T) {
	r := router.NewRouter()
	s := NewServer(r, true)
	if s == nil {
		t.Fatal("NewServer returned nil")
	}
	if !s.debug {
		t.Error("NewServer did not enable debug mode")
	}
}

// mockTreeNode implements router.TreeNode for testing findChild.
type mockTreeNode struct {
	name     string
	typ      router.NodeType
	pattern  string
	children map[string]router.TreeNode
	handlers map[string]any
}

func (m *mockTreeNode) GetName() string                    { return m.name }
func (m *mockTreeNode) GetType() router.NodeType           { return m.typ }
func (m *mockTreeNode) GetPattern() string                 { return m.pattern }
func (m *mockTreeNode) GetChildren() map[string]router.TreeNode { return m.children }
func (m *mockTreeNode) GetHandlers() map[string]any        { return m.handlers }
func (m *mockTreeNode) FindNode(components []string) (router.TreeNode, map[string]string) {
	return nil, nil
}
func (m *mockTreeNode) ListChildren() []string {
	names := make([]string, 0, len(m.children))
	for name := range m.children {
		names = append(names, name)
	}
	return names
}
func (m *mockTreeNode) GetHandler(operation string) any {
	if m.handlers != nil {
		return m.handlers[operation]
	}
	return nil
}

func TestFindChildExactMatch(t *testing.T) {
	fileChild := &mockTreeNode{name: "hello", typ: router.NodeTypeFile}
	parent := &mockTreeNode{
		name:     "",
		children: map[string]router.TreeNode{"hello": fileChild},
	}

	child, params, found := findChild(parent, "hello")
	if !found {
		t.Fatal("findChild did not find exact match")
	}
	if child != fileChild {
		t.Error("findChild returned wrong child for exact match")
	}
	if params != nil {
		t.Errorf("findChild returned unexpected params for exact match: %v", params)
	}
}

func TestFindChildParamMatch(t *testing.T) {
	paramChild := &mockTreeNode{name: "{id}", typ: router.NodeTypeParam}
	parent := &mockTreeNode{
		name:     "",
		children: map[string]router.TreeNode{"{id}": paramChild},
	}

	child, params, found := findChild(parent, "42")
	if !found {
		t.Fatal("findChild did not find param match")
	}
	if child != paramChild {
		t.Error("findChild returned wrong child for param match")
	}
	if params == nil || params["id"] != "42" {
		t.Errorf("findChild returned wrong params for param match: %v", params)
	}
}

func TestFindChildWildcardMatch(t *testing.T) {
	wildcardChild := &mockTreeNode{name: "*", typ: router.NodeTypeWildcard}
	parent := &mockTreeNode{
		name:     "",
		children: map[string]router.TreeNode{"*": wildcardChild},
	}

	child, params, found := findChild(parent, "anything")
	if !found {
		t.Fatal("findChild did not find wildcard match")
	}
	if child != wildcardChild {
		t.Error("findChild returned wrong child for wildcard match")
	}
	if params == nil || params["*"] != "anything" {
		t.Errorf("findChild returned wrong params for wildcard match: %v", params)
	}
}

func TestFindChildGlobMatch(t *testing.T) {
	globChild := &mockTreeNode{name: "prefix*", typ: router.NodeTypeGlob}
	parent := &mockTreeNode{
		name:     "",
		children: map[string]router.TreeNode{"prefix*": globChild},
	}

	child, _, found := findChild(parent, "prefix123")
	if !found {
		t.Fatal("findChild did not find glob match")
	}
	if child != globChild {
		t.Error("findChild returned wrong child for glob match")
	}
}

func TestFindChildNoMatch(t *testing.T) {
	parent := &mockTreeNode{
		name:     "",
		children: map[string]router.TreeNode{},
	}

	_, _, found := findChild(parent, "nonexistent")
	if found {
		t.Fatal("findChild should not find nonexistent name")
	}
}

func TestFindChildExactPrecedesParam(t *testing.T) {
	exactChild := &mockTreeNode{name: "status", typ: router.NodeTypeFile}
	paramChild := &mockTreeNode{name: "{id}", typ: router.NodeTypeParam}
	parent := &mockTreeNode{
		name: "",
		children: map[string]router.TreeNode{
			"status": exactChild,
			"{id}":   paramChild,
		},
	}

	// Exact match should take priority
	child, _, found := findChild(parent, "status")
	if !found {
		t.Fatal("findChild did not find exact match when param also matches")
	}
	if child != exactChild {
		t.Error("findChild did not prefer exact match over param")
	}

	// Non-exact should match param
	child, params, found := findChild(parent, "42")
	if !found {
		t.Fatal("findChild did not find param match")
	}
	if child != paramChild {
		t.Error("findChild should match param node")
	}
	if params == nil || params["id"] != "42" {
		t.Errorf("findChild returned wrong params: %v", params)
	}
}

func TestBuildDir(t *testing.T) {
	r := router.NewRouter()
	handler := &testHandler{}
	r.Register("read", "/users/list", handler)
	r.Register("read", "/users/create", handler)
	r.Register("read", "/status/health", handler)
	r.Register("read", "/files/hello", handler)
	r.Register("read", "/files/world", handler)
	r.Register("write", "/data/config", handler)

	s := NewServer(r, false)

	// Verify root was created
	if s.root == nil {
		t.Fatal("root directory is nil")
	}

	// The tree should have an "api" child directory
	children := s.root.Children()
	if len(children) == 0 {
		t.Fatal("root has no children")
	}

	// The tree should have top-level children
	children = s.root.Children()
	if len(children) == 0 {
		t.Fatal("root has no children")
	}

	// users should be a directory (it has children)
	usersNode, ok := children["users"]
	if !ok {
		t.Fatal("root does not have 'users' child")
	}
	usersDir, ok := usersNode.(fs.Dir)
	if !ok {
		t.Fatal("users child is not a Dir")
	}
	if len(usersDir.Children()) < 2 {
		t.Fatal("users directory should have at least 2 children")
	}

	// files should be a directory (it has a hello child)
	filesNode, ok := children["files"]
	if !ok {
		t.Fatal("root does not have 'files' child")
	}
	filesDir, ok := filesNode.(fs.Dir)
	if !ok {
		t.Fatal("files child is not a Dir")
	}
	if len(filesDir.Children()) >= 1 {
		t.Logf("files directory has %d children", len(filesDir.Children()))
	} else {
		t.Fatal("files directory has no children")
	}

	// hello should be a file under files
	helloNode := filesDir.Children()["hello"]
	if helloNode == nil {
		t.Fatal("files does not have 'hello' child")
	}
	if _, ok := helloNode.(fs.Dir); ok {
		t.Error("hello should be a file, not a directory")
	}
}

func TestServerSmoke(t *testing.T) {
	r := router.NewRouter()
	r.Register("read", "/hello", nil)

	s := NewServer(r, false)
	if s == nil {
		t.Fatal("NewServer returned nil")
	}

	// Verify the router is accessible
	if s.router == nil {
		t.Error("server has nil router")
	}

	// Verify nodeMap contains root
	s.mu.RLock()
	_, ok := s.nodeMap[s.root]
	s.mu.RUnlock()
	if !ok {
		t.Error("root directory not mapped in nodeMap")
	}
}
