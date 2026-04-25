package fuse

import (
	"context"
	"os"
	"syscall"
	"testing"

	"bazil.org/fuse"
	"github.com/bubblegutz/nklhd/internal/router"
)

// testHandler implements router.ServeHandler for testing.
type testHandler struct {
	data string
	err  error
}

func (h *testHandler) Serve(req *router.Request) (*router.Response, error) {
	if h.err != nil {
		return nil, h.err
	}
	return &router.Response{Data: h.data}, nil
}

// mockTreeNode implements router.TreeNode for testing findChild with param/wildcard/glob types.
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

// newTestFS creates a test FUSE filesystem with optional route registration.
func newTestFS(registerFn func(r *router.Router)) *nklhdFS {
	r := router.NewRouter()
	if registerFn != nil {
		registerFn(r)
	}
	return &nklhdFS{router: r, debug: false}
}

// newTestNode creates a test nklhdNode with the given path and tree node.
func newTestNode(fs *nklhdFS, tree router.TreeNode, path string) *nklhdNode {
	return &nklhdNode{
		fs:       fs,
		treeNode: tree,
		path:     path,
		params:   make(map[string]string),
		inode:    hashString(path),
	}
}

// findChildFromTree navigates the router tree by components and returns the
// child nklhdNode. It recursively calls Lookup to simulate FUSE traversal.
func findChildFromTree(parent *nklhdNode, components ...string) *nklhdNode {
	ctx := context.Background()
	cur := parent
	for _, comp := range components {
		child, err := cur.Lookup(ctx, comp)
		if err != nil {
			return nil
		}
		var ok bool
		cur, ok = child.(*nklhdNode)
		if !ok {
			return nil
		}
	}
	return cur
}

// ---------------------------------------------------------------------------
// NewServer / SetRouter
// ---------------------------------------------------------------------------

func TestNewServer(t *testing.T) {
	r := router.NewRouter()
	s := NewServer(r, nil)
	if s == nil {
		t.Fatal("NewServer returned nil")
	}
}

func TestSetRouter(t *testing.T) {
	r1 := router.NewRouter()
	s := NewServer(r1, nil)
	r2 := router.NewRouter()
	s.SetRouter(r2)
}

// ---------------------------------------------------------------------------
// nklhdFS.Root
// ---------------------------------------------------------------------------

func TestNklhdFSRoot(t *testing.T) {
	f := newTestFS(nil)
	node, err := f.Root()
	if err != nil {
		t.Fatalf("Root() returned error: %v", err)
	}
	n, ok := node.(*nklhdNode)
	if !ok {
		t.Fatalf("Root() returned type %T, want *nklhdNode", node)
	}
	if n.path != "/" {
		t.Errorf("root node path = %q, want %q", n.path, "/")
	}
	if n.inode != FUSE_ROOT_ID {
		t.Errorf("root node inode = %d, want %d", n.inode, FUSE_ROOT_ID)
	}
	if n.treeNode == nil {
		t.Error("root node has nil treeNode")
	}
}

// ---------------------------------------------------------------------------
// isDir
// ---------------------------------------------------------------------------

func TestNklhdNodeIsDirDirNode(t *testing.T) {
	treeNode := &virtualTreeNode{
		name:  "subdir",
		isDir: true,
	}
	f := newTestFS(nil)
	n := newTestNode(f, treeNode, "/subdir")
	if !n.isDir() {
		t.Error("expected isDir = true for directory-type node")
	}
}

func TestNklhdNodeIsDirFileNode(t *testing.T) {
	treeNode := &virtualTreeNode{
		name:  "file",
		isDir: false,
	}
	f := newTestFS(nil)
	n := newTestNode(f, treeNode, "/file")
	if n.isDir() {
		t.Error("expected isDir = false for file-type node")
	}
}

func TestNklhdNodeIsDirWithReaddirHandler(t *testing.T) {
	treeNode := &virtualTreeNode{
		name:     "dir",
		isDir:    false,
		handlers: map[string]any{"readdir": &testHandler{data: "item1\nitem2"}},
	}
	f := newTestFS(nil)
	n := newTestNode(f, treeNode, "/dir")
	if !n.isDir() {
		t.Error("expected isDir = true for node with readdir handler")
	}
}

func TestNklhdNodeIsDirWithChildren(t *testing.T) {
	child := &virtualTreeNode{name: "child", isDir: false}
	treeNode := &virtualTreeNode{
		name:     "parent",
		children: map[string]router.TreeNode{"child": child},
	}
	f := newTestFS(nil)
	n := newTestNode(f, treeNode, "/parent")
	if !n.isDir() {
		t.Error("expected isDir = true for node with children")
	}
}

// ---------------------------------------------------------------------------
// Attr
// ---------------------------------------------------------------------------

func TestNklhdNodeAttrDefaults(t *testing.T) {
	f := newTestFS(func(r *router.Router) {
		r.Register("read", "/leaf", &testHandler{data: "content"})
	})
	tree := f.router.GetTree()
	leafNode, _ := tree.FindNode([]string{"leaf"})
	if leafNode == nil {
		t.Fatal("could not find leaf node in tree")
	}
	n := newTestNode(f, leafNode, "/leaf")
	ctx := context.Background()
	var a fuse.Attr
	if err := n.Attr(ctx, &a); err != nil {
		t.Fatalf("Attr() returned error: %v", err)
	}
	if a.Inode != n.inode {
		t.Errorf("inode = %d, want %d", a.Inode, n.inode)
	}
	if a.Mode&os.ModeDir != 0 {
		t.Error("leaf should not be a directory")
	}
	if a.Nlink != 1 {
		t.Errorf("nlink = %d, want 1", a.Nlink)
	}
	if a.Size != 7 {
		t.Errorf("size = %d, want 7 (len of 'content')", a.Size)
	}
	if a.Uid != uint32(os.Getuid()) {
		t.Errorf("uid = %d, want %d", a.Uid, os.Getuid())
	}
}

func TestNklhdNodeAttrWithGetAttr(t *testing.T) {
	f := newTestFS(func(r *router.Router) {
		r.Register("getattr", "/statfile", &testHandler{
			data: "size=2048\nmode=0644\nuid=999\ngid=999",
		})
	})
	tree := f.router.GetTree()
	leaf, _ := tree.FindNode([]string{"statfile"})
	if leaf == nil {
		t.Fatal("could not find statfile node")
	}
	n := newTestNode(f, leaf, "/statfile")
	ctx := context.Background()
	var a fuse.Attr
	if err := n.Attr(ctx, &a); err != nil {
		t.Fatalf("Attr() returned error: %v", err)
	}
	if a.Size != 2048 {
		t.Errorf("size = %d, want 2048", a.Size)
	}
	if a.Mode != os.FileMode(0644) {
		t.Errorf("mode = %v, want 0644", a.Mode)
	}
	if a.Uid != 999 {
		t.Errorf("uid = %d, want 999", a.Uid)
	}
	if a.Gid != 999 {
		t.Errorf("gid = %d, want 999", a.Gid)
	}
}

func TestNklhdNodeAttrDirNode(t *testing.T) {
	f := newTestFS(func(r *router.Router) {
		r.Register("read", "/dir/file", &testHandler{data: "nested"})
	})
	tree := f.router.GetTree()
	dirNode, _ := tree.FindNode([]string{"dir"})
	if dirNode == nil {
		t.Fatal("could not find dir node")
	}
	n := newTestNode(f, dirNode, "/dir")
	ctx := context.Background()
	var a fuse.Attr
	if err := n.Attr(ctx, &a); err != nil {
		t.Fatalf("Attr() returned error: %v", err)
	}
	if a.Mode&os.ModeDir == 0 {
		t.Error("dir should be a directory")
	}
	if a.Nlink != 2 {
		t.Errorf("nlink = %d, want 2 for directory", a.Nlink)
	}
}

// ---------------------------------------------------------------------------
// Read / fetchData
// ---------------------------------------------------------------------------

func TestNklhdNodeRead(t *testing.T) {
	f := newTestFS(func(r *router.Router) {
		r.Register("read", "/hello", &testHandler{data: "Hello, world!"})
	})
	tree := f.router.GetTree()
	leaf, _ := tree.FindNode([]string{"hello"})
	if leaf == nil {
		t.Fatal("could not find hello node")
	}
	n := newTestNode(f, leaf, "/hello")
	ctx := context.Background()

	resp := &fuse.ReadResponse{}
	req := &fuse.ReadRequest{Offset: 0, Size: 100}
	if err := n.Read(ctx, req, resp); err != nil {
		t.Fatalf("Read() returned error: %v", err)
	}
	if string(resp.Data) != "Hello, world!" {
		t.Errorf("read data = %q, want %q", string(resp.Data), "Hello, world!")
	}
}

func TestNklhdNodeReadWithOffset(t *testing.T) {
	f := newTestFS(func(r *router.Router) {
		r.Register("read", "/data", &testHandler{data: "0123456789"})
	})
	tree := f.router.GetTree()
	leaf, _ := tree.FindNode([]string{"data"})
	if leaf == nil {
		t.Fatal("could not find data node")
	}
	n := newTestNode(f, leaf, "/data")
	ctx := context.Background()

	resp := &fuse.ReadResponse{}
	req := &fuse.ReadRequest{Offset: 3, Size: 4}
	if err := n.Read(ctx, req, resp); err != nil {
		t.Fatalf("Read() returned error: %v", err)
	}
	if string(resp.Data) != "3456" {
		t.Errorf("read data = %q, want %q", string(resp.Data), "3456")
	}
}

func TestNklhdNodeReadBeyondEnd(t *testing.T) {
	f := newTestFS(func(r *router.Router) {
		r.Register("read", "/data", &testHandler{data: "abc"})
	})
	tree := f.router.GetTree()
	leaf, _ := tree.FindNode([]string{"data"})
	if leaf == nil {
		t.Fatal("could not find data node")
	}
	n := newTestNode(f, leaf, "/data")
	ctx := context.Background()

	resp := &fuse.ReadResponse{}
	req := &fuse.ReadRequest{Offset: 10, Size: 5}
	if err := n.Read(ctx, req, resp); err != nil {
		t.Fatalf("Read() returned error: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Errorf("read data = %q, want empty", string(resp.Data))
	}
}

func TestNklhdNodeReadNoHandler(t *testing.T) {
	f := newTestFS(nil)
	tree := f.router.GetTree()
	n := newTestNode(f, tree, "/nonexistent")
	ctx := context.Background()
	resp := &fuse.ReadResponse{}
	req := &fuse.ReadRequest{Offset: 0, Size: 100}
	if err := n.Read(ctx, req, resp); err != nil {
		t.Fatalf("Read() returned error: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Errorf("read data = %q, want empty", string(resp.Data))
	}
}

func TestNklhdNodeFetchDataCaching(t *testing.T) {
	f := newTestFS(func(r *router.Router) {
		r.Register("read", "/cachetest", &testHandler{data: "cached content"})
	})
	tree := f.router.GetTree()
	leaf, _ := tree.FindNode([]string{"cachetest"})
	if leaf == nil {
		t.Fatal("could not find cachetest node")
	}
	n := newTestNode(f, leaf, "/cachetest")

	data, err := n.fetchData()
	if err != nil {
		t.Fatalf("fetchData() returned error: %v", err)
	}
	if data != "cached content" {
		t.Errorf("fetchData = %q, want %q", data, "cached content")
	}

	// Second call should return cached data.
	n.mu.Lock()
	n.cachedData = "overridden"
	n.mu.Unlock()

	data, err = n.fetchData()
	if err != nil {
		t.Fatalf("fetchData() second call returned error: %v", err)
	}
	if data != "overridden" {
		t.Errorf("cached data = %q, want %q", data, "overridden")
	}
}

func TestNklhdNodeFetchDataNoHandler(t *testing.T) {
	f := newTestFS(nil)
	tree := f.router.GetTree()
	n := newTestNode(f, tree, "/missing")
	data, err := n.fetchData()
	if err != nil {
		t.Fatalf("fetchData() returned error: %v", err)
	}
	if data != "" {
		t.Errorf("fetchData = %q, want empty", data)
	}
}

// ---------------------------------------------------------------------------
// Write
// ---------------------------------------------------------------------------

func TestNklhdNodeWrite(t *testing.T) {
	f := newTestFS(func(r *router.Router) {
		r.Register("write", "/echo", &testHandler{
			data: "Echo: test data",
		})
	})
	tree := f.router.GetTree()
	leaf, _ := tree.FindNode([]string{"echo"})
	if leaf == nil {
		t.Fatal("could not find echo node")
	}
	n := newTestNode(f, leaf, "/echo")
	ctx := context.Background()

	resp := &fuse.WriteResponse{}
	req := &fuse.WriteRequest{
		Offset: 0,
		Data:   []byte("test data"),
	}
	if err := n.Write(ctx, req, resp); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	if resp.Size != len("test data") {
		t.Errorf("resp.Size = %d, want %d", resp.Size, len("test data"))
	}

	n.mu.RLock()
	cached := n.cachedData
	n.mu.RUnlock()
	if cached != "Echo: test data" {
		t.Errorf("cachedData = %q, want %q", cached, "Echo: test data")
	}
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func TestNklhdNodeCreate(t *testing.T) {
	f := newTestFS(func(r *router.Router) {
		r.Register("create", "/parent/{name}", &testHandler{
			data: "created",
		})
	})
	tree := f.router.GetTree()
	parentNode, _ := tree.FindNode([]string{"parent", "{name}"})
	if parentNode == nil {
		t.Fatal("could not find parent/{name} node")
	}
	parent := newTestNode(f, parentNode, "/parent")
	ctx := context.Background()

	node, handle, err := parent.Create(ctx, &fuse.CreateRequest{
		Name: "newfile",
		Mode: 0644,
	}, &fuse.CreateResponse{})
	if err != nil {
		t.Fatalf("Create() returned error: %v", err)
	}
	if node == nil {
		t.Fatal("Create() returned nil node")
	}
	if handle == nil {
		t.Fatal("Create() returned nil handle")
	}
	n, ok := node.(*nklhdNode)
	if !ok {
		t.Fatalf("Create() returned node of type %T, want *nklhdNode", node)
	}
	if n.path != "/parent/newfile" {
		t.Errorf("node path = %q, want %q", n.path, "/parent/newfile")
	}
	vt, ok := n.treeNode.(*virtualTreeNode)
	if !ok {
		t.Fatalf("inner treeNode = %T, want *virtualTreeNode", n.treeNode)
	}
	if vt.name != "newfile" {
		t.Errorf("virtual node name = %q, want %q", vt.name, "newfile")
	}
	if node != handle {
		t.Error("Create returned different node and handle, expected same")
	}
}

func TestNklhdNodeCreateNoHandler(t *testing.T) {
	f := newTestFS(nil)
	tree := f.router.GetTree()
	n := newTestNode(f, tree, "/")
	ctx := context.Background()

	_, _, err := n.Create(ctx, &fuse.CreateRequest{
		Name: "newfile",
		Mode: 0644,
	}, &fuse.CreateResponse{})
	if err == nil {
		t.Fatal("expected error from Create() with no handler")
	}
}

// ---------------------------------------------------------------------------
// Remove
// ---------------------------------------------------------------------------

func TestNklhdNodeRemove(t *testing.T) {
	f := newTestFS(func(r *router.Router) {
		r.Register("unlink", "/delete_me", &testHandler{data: "deleted"})
	})
	tree := f.router.GetTree()
	root := newTestNode(f, tree, "/")
	ctx := context.Background()

	if err := root.Remove(ctx, &fuse.RemoveRequest{Name: "delete_me", Dir: false}); err != nil {
		t.Fatalf("Remove() returned error: %v", err)
	}
}

func TestNklhdNodeRemoveNoHandler(t *testing.T) {
	f := newTestFS(nil)
	tree := f.router.GetTree()
	n := newTestNode(f, tree, "/missing")
	ctx := context.Background()

	if err := n.Remove(ctx, &fuse.RemoveRequest{Name: "missing", Dir: false}); err == nil {
		t.Fatal("expected error from Remove() with no unlink handler")
	}
}

// ---------------------------------------------------------------------------
// Setattr
// ---------------------------------------------------------------------------

func TestNklhdNodeSetattrSize(t *testing.T) {
	f := newTestFS(nil)
	tree := f.router.GetTree()
	n := newTestNode(f, tree, "/file")
	ctx := context.Background()

	resp := &fuse.SetattrResponse{}
	req := &fuse.SetattrRequest{
		Valid: fuse.SetattrSize,
		Size:  2048,
	}
	if err := n.Setattr(ctx, req, resp); err != nil {
		t.Fatalf("Setattr() returned error: %v", err)
	}
	n.mu.RLock()
	size := n.cachedSize
	n.mu.RUnlock()
	if size != 2048 {
		t.Errorf("cachedSize = %d, want 2048", size)
	}
}

// ---------------------------------------------------------------------------
// Open
// ---------------------------------------------------------------------------

func TestNklhdNodeOpen(t *testing.T) {
	f := newTestFS(func(r *router.Router) {
		r.Register("open", "/openme", &testHandler{data: "opened"})
	})
	tree := f.router.GetTree()
	leaf, _ := tree.FindNode([]string{"openme"})
	if leaf == nil {
		t.Fatal("could not find openme node")
	}
	n := newTestNode(f, leaf, "/openme")
	ctx := context.Background()

	h, err := n.Open(ctx, &fuse.OpenRequest{}, &fuse.OpenResponse{})
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	if h != n {
		t.Error("Open() returned handle different from node")
	}
}

func TestNklhdNodeOpenNoHandler(t *testing.T) {
	f := newTestFS(nil)
	tree := f.router.GetTree()
	n := newTestNode(f, tree, "/")
	ctx := context.Background()

	h, err := n.Open(ctx, &fuse.OpenRequest{}, &fuse.OpenResponse{})
	if err != nil {
		t.Fatalf("Open() with no handler returned error: %v", err)
	}
	if h != n {
		t.Error("Open() returned handle different from node")
	}
}

// ---------------------------------------------------------------------------
// Lookup
// ---------------------------------------------------------------------------

func TestNklhdNodeLookupFound(t *testing.T) {
	f := newTestFS(func(r *router.Router) {
		r.Register("read", "/hello", &testHandler{data: "world"})
	})
	tree := f.router.GetTree()
	n := newTestNode(f, tree, "/")
	ctx := context.Background()

	child, err := n.Lookup(ctx, "hello")
	if err != nil {
		t.Fatalf("Lookup('hello') returned error: %v", err)
	}
	if child == nil {
		t.Fatal("Lookup('hello') returned nil")
	}
	cn, ok := child.(*nklhdNode)
	if !ok {
		t.Fatalf("Lookup returned type %T, want *nklhdNode", child)
	}
	if cn.path != "/hello" {
		t.Errorf("child path = %q, want %q", cn.path, "/hello")
	}
}

func TestNklhdNodeLookupVirtual(t *testing.T) {
	f := newTestFS(func(r *router.Router) {
		r.Register("readdir", "/", &testHandler{data: "virt_file\nvirt_dir/"})
	})
	tree := f.router.GetTree()
	n := newTestNode(f, tree, "/")
	ctx := context.Background()

	child, err := n.Lookup(ctx, "virt_file")
	if err != nil {
		t.Fatalf("Lookup('virt_file') returned error: %v", err)
	}
	if child == nil {
		t.Fatal("Lookup('virt_file') returned nil")
	}
	cn, ok := child.(*nklhdNode)
	if !ok {
		t.Fatalf("Lookup returned type %T, want *nklhdNode", child)
	}
	if cn.path != "/virt_file" {
		t.Errorf("child path = %q, want %q", cn.path, "/virt_file")
	}

	// virt_dir should be a directory (trailing / in readdir response).
	dirChild, err := n.Lookup(ctx, "virt_dir")
	if err != nil {
		t.Fatalf("Lookup('virt_dir') returned error: %v", err)
	}
	dn := dirChild.(*nklhdNode)
	if !dn.isDir() {
		t.Error("virt_dir should be a directory")
	}
}

func TestNklhdNodeLookupNotFound(t *testing.T) {
	f := newTestFS(nil)
	tree := f.router.GetTree()
	n := newTestNode(f, tree, "/")
	ctx := context.Background()

	_, err := n.Lookup(ctx, "nonexistent")
	if err != syscall.ENOENT {
		t.Fatalf("Lookup('nonexistent') = %v, want ENOENT", err)
	}
}

func TestNklhdNodeLookupNonDir(t *testing.T) {
	f := newTestFS(func(r *router.Router) {
		r.Register("read", "/file", &testHandler{data: "content"})
	})
	tree := f.router.GetTree()
	leaf, _ := tree.FindNode([]string{"file"})
	if leaf == nil {
		t.Fatal("could not find file node")
	}
	n := newTestNode(f, leaf, "/file")
	ctx := context.Background()
	_, err := n.Lookup(ctx, "child")
	if err != syscall.ENOTDIR {
		t.Fatalf("Lookup on file = %v, want ENOTDIR", err)
	}
}

// ---------------------------------------------------------------------------
// ReadDirAll
// ---------------------------------------------------------------------------

func TestNklhdNodeReadDirAllTreeChildren(t *testing.T) {
	f := newTestFS(func(r *router.Router) {
		r.Register("read", "/a", &testHandler{data: "a"})
		r.Register("read", "/b", &testHandler{data: "b"})
	})
	tree := f.router.GetTree()
	n := newTestNode(f, tree, "/")
	ctx := context.Background()

	entries, err := n.ReadDirAll(ctx)
	if err != nil {
		t.Fatalf("ReadDirAll() returned error: %v", err)
	}
	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name] = true
	}
	if !names["a"] {
		t.Errorf("expected 'a' in directory entries, got %v", entries)
	}
	if !names["b"] {
		t.Errorf("expected 'b' in directory entries, got %v", entries)
	}
}

func TestNklhdNodeReadDirAllFromReaddir(t *testing.T) {
	f := newTestFS(func(r *router.Router) {
		r.Register("readdir", "/", &testHandler{data: "item1\nitem2/\nitem3"})
	})
	tree := f.router.GetTree()
	n := newTestNode(f, tree, "/")
	ctx := context.Background()

	entries, err := n.ReadDirAll(ctx)
	if err != nil {
		t.Fatalf("ReadDirAll() returned error: %v", err)
	}
	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name] = true
	}
	if !names["item1"] {
		t.Errorf("expected 'item1' in directory entries, got %v", entries)
	}
	if !names["item2"] {
		t.Errorf("expected 'item2' in directory entries, got %v", entries)
	}
	if !names["item3"] {
		t.Errorf("expected 'item3' in directory entries, got %v", entries)
	}
	for _, e := range entries {
		if e.Name == "item2" && e.Type != fuse.DT_Dir {
			t.Errorf("item2 should be DT_Dir, got %v", e.Type)
		}
		if e.Name == "item1" && e.Type != fuse.DT_File {
			t.Errorf("item1 should be DT_File, got %v", e.Type)
		}
	}
}

func TestNklhdNodeReadDirAllNoChildren(t *testing.T) {
	f := newTestFS(nil)
	tree := f.router.GetTree()
	n := newTestNode(f, tree, "/")
	ctx := context.Background()

	entries, err := n.ReadDirAll(ctx)
	if err != nil {
		t.Fatalf("ReadDirAll() returned error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d: %v", len(entries), entries)
	}
}

// ---------------------------------------------------------------------------
// getListEntries caching
// ---------------------------------------------------------------------------

func TestNklhdNodeGetListEntriesCaching(t *testing.T) {
	handler := &testHandler{data: "first/"}
	f := newTestFS(func(r *router.Router) {
		r.Register("readdir", "/", handler)
	})
	tree := f.router.GetTree()
	n := newTestNode(f, tree, "/")

	entries1, err := n.getListEntries()
	if err != nil {
		t.Fatalf("getListEntries() returned error: %v", err)
	}
	if !entries1["first"] {
		t.Errorf("expected 'first' entry, got %v", entries1)
	}

	// Modify handler data to prove caching.
	handler.data = "second"

	entries2, err := n.getListEntries()
	if err != nil {
		t.Fatalf("getListEntries() second call returned error: %v", err)
	}
	// Should still return "first" (cached).
	if !entries2["first"] {
		t.Errorf("expected cached 'first', got %v", entries2)
	}
}

// ---------------------------------------------------------------------------
// Virtual directory node
// ---------------------------------------------------------------------------

func TestVirtualTreeNodeIsDir(t *testing.T) {
	fileVT := &virtualTreeNode{name: "f", isDir: false}
	if fileVT.GetType() != router.NodeTypeFile {
		t.Error("virtualTreeNode with isDir=false should be NodeTypeFile")
	}
	dirVT := &virtualTreeNode{name: "d", isDir: true}
	if dirVT.GetType() != router.NodeTypeDir {
		t.Error("virtualTreeNode with isDir=true should be NodeTypeDir")
	}
	if fileVT.GetPattern() != "" {
		t.Error("virtualTreeNode pattern should be empty")
	}
	if fileVT.ListChildren() == nil {
		t.Error("ListChildren on empty children should return non-nil")
	}
	if len(fileVT.ListChildren()) != 0 {
		t.Error("ListChildren on empty children should return empty slice")
	}
	if fileVT.GetHandler("read") != nil {
		t.Error("GetHandler on empty handlers should return nil")
	}
}

func TestVirtualTreeNodeFindNode(t *testing.T) {
	vt := &virtualTreeNode{name: "v"}
	node, params := vt.FindNode([]string{"child"})
	if node != nil {
		t.Error("FindNode on virtualTreeNode should return nil node")
	}
	if params != nil {
		t.Error("FindNode on virtualTreeNode should return nil params")
	}
}

// ---------------------------------------------------------------------------
// Utility: hashString
// ---------------------------------------------------------------------------

func TestHashString(t *testing.T) {
	h1 := hashString("/hello")
	h2 := hashString("/hello")
	if h1 != h2 {
		t.Errorf("hashString is not deterministic: %d vs %d", h1, h2)
	}
	h3 := hashString("/world")
	if h1 == h3 {
		t.Errorf("hashString collision: /hello == /world == %d", h1)
	}
	if h1 == 0 {
		t.Error("hashString returned 0")
	}
}

// ---------------------------------------------------------------------------
// Utility: splitLines
// ---------------------------------------------------------------------------

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", []string{""}},
		{"hello", []string{"hello"}},
		{"a\nb", []string{"a", "b"}},
		{"a\nb\n", []string{"a", "b", ""}},
		{"\n", []string{"", ""}},
		{"a\n\nb", []string{"a", "", "b"}},
	}
	for _, tt := range tests {
		got := splitLines(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("splitLines(%q) = %v (len=%d), want %v (len=%d)", tt.input, got, len(got), tt.want, len(tt.want))
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitLines(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Utility: trimSpace
// ---------------------------------------------------------------------------

func TestTrimSpace(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"hello", "hello"},
		{"  hello", "hello"},
		{"hello  ", "hello"},
		{"  hello  ", "hello"},
		{"\t\nhello\r ", "hello"},
		{"\n", ""},
	}
	for _, tt := range tests {
		got := trimSpace(tt.input)
		if got != tt.want {
			t.Errorf("trimSpace(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// findChild — FUSE version (with parentParams)
// ---------------------------------------------------------------------------

func TestFindChildExact(t *testing.T) {
	child := &mockTreeNode{name: "hello", typ: router.NodeTypeFile}
	parent := &mockTreeNode{
		name:     "",
		children: map[string]router.TreeNode{"hello": child},
	}
	found, params, ok := findChild(parent, "hello", nil)
	if !ok {
		t.Fatal("findChild did not find exact match")
	}
	if found != child {
		t.Error("findChild returned wrong node for exact match")
	}
	if params != nil {
		t.Errorf("expected nil params, got %v", params)
	}
}

func TestFindChildParam(t *testing.T) {
	paramChild := &mockTreeNode{name: "{id}", typ: router.NodeTypeParam}
	parent := &mockTreeNode{
		name:     "",
		children: map[string]router.TreeNode{
			"{id}": paramChild,
		},
	}
	parentParams := map[string]string{"parent": "val"}
	found, params, ok := findChild(parent, "42", parentParams)
	if !ok {
		t.Fatal("findChild did not find param match")
	}
	if found != paramChild {
		t.Error("findChild returned wrong node for param match")
	}
	if params == nil {
		t.Fatal("params is nil")
	}
	if params["id"] != "42" {
		t.Errorf("params['id'] = %q, want %q", params["id"], "42")
	}
	if params["parent"] != "val" {
		t.Errorf("params['parent'] = %q, want %q", params["parent"], "val")
	}
}

func TestFindChildWildcard(t *testing.T) {
	wildcardChild := &mockTreeNode{name: "*", typ: router.NodeTypeWildcard}
	parent := &mockTreeNode{
		name:     "",
		children: map[string]router.TreeNode{"*": wildcardChild},
	}
	found, params, ok := findChild(parent, "anything", nil)
	if !ok {
		t.Fatal("findChild did not find wildcard match")
	}
	if found != wildcardChild {
		t.Error("findChild returned wrong node for wildcard match")
	}
	if params == nil || params["*"] != "anything" {
		t.Errorf("params['*'] = %v, want %q", params, "anything")
	}
}

func TestFindChildGlob(t *testing.T) {
	globChild := &mockTreeNode{name: "prefix*", typ: router.NodeTypeGlob}
	parent := &mockTreeNode{
		name:     "",
		children: map[string]router.TreeNode{"prefix*": globChild},
	}
	found, _, ok := findChild(parent, "prefix123", nil)
	if !ok {
		t.Fatal("findChild did not find glob match")
	}
	if found != globChild {
		t.Error("findChild returned wrong node for glob match")
	}
}

func TestFindChildNoMatch(t *testing.T) {
	parent := &mockTreeNode{
		name:     "",
		children: map[string]router.TreeNode{},
	}
	_, _, ok := findChild(parent, "nonexistent", nil)
	if ok {
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

	child, _, found := findChild(parent, "status", nil)
	if !found {
		t.Fatal("findChild did not find exact match when param also matches")
	}
	if child != exactChild {
		t.Error("findChild did not prefer exact match over param")
	}

	child, params, found := findChild(parent, "42", nil)
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

// vim: noexpandtab
