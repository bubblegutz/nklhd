// Package ninep implements a 9p2000 server that bridges the router's route tree
// to the 9p filesystem protocol, providing an alternative to FUSE for accessing
// nklhd's virtual filesystem.
package ninep

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/bubblegutz/nklhd/internal/router"
	"github.com/knusbaum/go9p"
	"github.com/knusbaum/go9p/fs"
	"github.com/knusbaum/go9p/proto"
)

// Server implements a 9p2000 server that bridges to the router's route tree.
type Server struct {
	router   *router.Router
	ninepFS  *fs.FS
	root     *fs.StaticDir
	nodeMap  map[fs.Dir]router.TreeNode
	mu       sync.RWMutex
	debug    bool
	listener net.Listener // stored for graceful shutdown via Stop()
}

// NewServer creates a new 9p server backed by the given router.
func NewServer(r *router.Router, debug bool) *Server {
	s := &Server{
		router:  r,
		nodeMap: make(map[fs.Dir]router.TreeNode),
		debug:   debug,
	}

	// Create the 9p filesystem with custom handlers.
	nfs, rootDir := fs.NewFS("nobody", "nogroup", 0555|proto.DMDIR,
		fs.WithWalkFailHandler(s.walkFail),
		fs.WithCreateFile(s.createFile),
		fs.WithRemoveFile(s.removeFile),
	)
	s.ninepFS = nfs
	s.root = rootDir

	// Map root to router tree root.
	treeRoot := r.GetTree()
	s.nodeMap[rootDir] = treeRoot

	// Build static tree from router tree.
	if treeRoot != nil {
		s.buildDir(rootDir, treeRoot)
	}

	return s
}

// buildDir recursively builds static directories and files for router tree nodes.
// Parameter, wildcard, and glob nodes are skipped — they are handled dynamically
// by the walkFail handler.
func (s *Server) buildDir(parent fs.ModDir, tn router.TreeNode) {
	for name, child := range tn.GetChildren() {
		switch child.GetType() {
		case router.NodeTypeDir:
			s.buildDirFromTree(parent, child, name)
		case router.NodeTypeFile:
			s.buildFileFromTree(parent, child, name)
		case router.NodeTypeParam, router.NodeTypeWildcard, router.NodeTypeGlob:
			// Handled dynamically by walkFail.
		}
	}
}

func (s *Server) buildDirFromTree(parent fs.ModDir, child router.TreeNode, name string) {
	stat := s.ninepFS.NewStat(name, "nobody", "nogroup", 0777|proto.DMDIR)
	subDir := newRouterDir(s, stat, child)
	if err := parent.AddChild(subDir); err != nil {
		if s.debug {
			log.Printf("9p: add dir %s: %v", name, err)
		}
		return
	}
	s.mu.Lock()
	s.nodeMap[subDir] = child
	s.mu.Unlock()
	s.buildDir(subDir, child)
}

func (s *Server) buildFileFromTree(parent fs.ModDir, child router.TreeNode, name string) {
	rf := newRouterFile(s, name, "nobody", "nogroup", 0666)
	if err := parent.AddChild(rf); err != nil {
		if s.debug {
			log.Printf("9p: add file %s: %v", name, err)
		}
	}
}

// walkFail is called when a client walks to a path that does not exist in the
// static tree. It handles parameterized paths ({id}), wildcards (*), and glob
// patterns by dynamically creating the appropriate node.
func (s *Server) walkFail(nfs *fs.FS, parent fs.Dir, name string) (fs.FSNode, error) {
	s.mu.RLock()
	tn, ok := s.nodeMap[parent]
	s.mu.RUnlock()
	if !ok {
		if s.debug {
			log.Printf("9p: walkFail - unknown parent directory")
		}
		return nil, fmt.Errorf("not found: %s", name)
	}

	// Search for pattern-matching children (same logic as FUSE findChild).
	child, _, found := findChild(tn, name)
	if !found {
		if s.debug {
			log.Printf("9p: walkFail - no match for %q under %s", name, tn.GetName())
		}
		return nil, fmt.Errorf("not found: %s", name)
	}

	// If the matched node has children (sub-routes), create a directory so
	// further walk operations can continue.
	if len(child.GetChildren()) > 0 {
		stat := nfs.NewStat(name, "nobody", "nogroup", 0777|proto.DMDIR)
		subDir := newRouterDir(s, stat, child)
		s.mu.Lock()
		s.nodeMap[subDir] = child
		s.mu.Unlock()
		return subDir, nil
	}

	// Leaf node — return a routerFile that proxies to router.Serve.
	return newRouterFile(s, name, "nobody", "nogroup", 0666), nil
}

// findChild searches a router TreeNode for a child matching the given name.
// It returns the matched child, captured parameters, and whether a match was found.
func findChild(parent router.TreeNode, name string) (router.TreeNode, map[string]string, bool) {
	children := parent.GetChildren()

	// Exact match first.
	if child, ok := children[name]; ok {
		if child.GetType() == router.NodeTypeParam || child.GetType() == router.NodeTypeWildcard || child.GetType() == router.NodeTypeGlob {
			// Fall through to pattern matching.
		} else {
			return child, nil, true
		}
	}

	// Glob pattern nodes.
	for childName, child := range children {
		if child.GetType() == router.NodeTypeGlob {
			if router.ComponentMatches(childName, name) {
				return child, nil, true
			}
		}
	}

	// Parameter nodes.
	for childName, child := range children {
		if child.GetType() == router.NodeTypeParam {
			if name == childName {
				continue // literal match already handled above
			}
			paramName := childName[1 : len(childName)-1] // remove braces
			params := map[string]string{paramName: name}
			return child, params, true
		}
	}

	// Wildcard nodes.
	for _, child := range children {
		if child.GetType() == router.NodeTypeWildcard {
			return child, map[string]string{"*": name}, true
		}
	}

	return nil, nil, false
}

// createFile handles file creation via 9p Create.
func (s *Server) createFile(nfs *fs.FS, parent fs.Dir, user, name string, perm uint32, mode uint8) (fs.File, error) {
	reqPath := path.Join("/", fs.FullPath(parent), name)

	if s.debug {
		log.Printf("9p: create %s (user=%s, perm=%o, mode=%d)", reqPath, user, perm, mode)
	}

	// Notify the router about the create operation.
	_, err := s.router.Serve(&router.Request{
		Operation: "create",
		Path:      reqPath,
	})
	if err != nil && s.debug {
		log.Printf("9p: create handler error for %s: %v", reqPath, err)
	}

	// Create the file and add to parent.
	modParent, ok := parent.(fs.ModDir)
	if !ok {
		return nil, fmt.Errorf("parent does not support modification")
	}

	rf := newRouterFile(s, name, user, "nogroup", perm)
	if err := modParent.AddChild(rf); err != nil {
		return nil, err
	}

	// Notify the "open" on the newly created file after creating.
	_, err = s.router.Serve(&router.Request{
		Operation: "open",
		Path:      reqPath,
	})
	if err != nil && s.debug {
		log.Printf("9p: open after create handler error for %s: %v", reqPath, err)
	}

	return rf, nil
}

// removeFile handles file removal via 9p Remove.
func (s *Server) removeFile(nfs *fs.FS, f fs.FSNode) error {
	reqPath := path.Join("/", fs.FullPath(f))

	if s.debug {
		log.Printf("9p: remove %s", reqPath)
	}

	_, err := s.router.Serve(&router.Request{
		Operation: "unlink",
		Path:      reqPath,
	})
	if err != nil {
		return err
	}

	// Remove from parent.
	parent, ok := f.Parent().(fs.ModDir)
	if !ok {
		return fmt.Errorf("parent does not support modification")
	}
	return parent.DeleteChild(f.Stat().Name)
}

// parse9pAddr parses a Plan 9 network address format and returns
// the Go network and address. Supports:
//
//	tcp!host!port  -> ("tcp", "host:port")
//	unix!/path     -> ("unix", "/path")
//	host:port      -> ("tcp", "host:port")  (backward compatible)
func parse9pAddr(addr string) (network, address string) {
	if strings.HasPrefix(addr, "tcp!") {
		parts := strings.SplitN(addr, "!", 3)
		if len(parts) == 3 {
			return "tcp", parts[1] + ":" + parts[2]
		}
		return "tcp", parts[1]
	}
	if strings.HasPrefix(addr, "unix!") {
		return "unix", addr[5:]
	}
	return "tcp", addr
}

// serveOnListener accepts connections on the given listener and serves
// the 9p protocol on each connection in a goroutine.
func (s *Server) serveOnListener(l net.Listener) error {
	for {
		a, err := l.Accept()
		if err != nil {
			return err
		}
		go func(nc net.Conn) {
			defer nc.Close()
			read := bufio.NewReader(nc)
			err := go9p.ServeReadWriter(read, nc, s.ninepFS.Server())
			if err != nil && s.debug {
				log.Printf("9p: connection error: %v", err)
			}
		}(a)
	}
}

// Serve starts the 9p server on the given address. The address can be
// in Plan 9 format (tcp!host!port, unix!/path) or Go format (host:port).
// Blocks until the server stops (e.g., due to a network error).
func (s *Server) Serve(addr string) error {
	network, address := parse9pAddr(addr)
	if s.debug {
		log.Printf("9p: serving on %s (%s)", address, network)
	}

	// Remove any leftover Unix socket before listening.
	if network == "unix" {
		os.Remove(address)
	}

	l, err := net.Listen(network, address)
	if err != nil {
		return fmt.Errorf("9p listen on %s %s: %w", network, address, err)
	}
	s.listener = l
	return s.serveOnListener(l)
}

// Stop gracefully stops the 9p server by closing the listener.
// After Stop returns, no new connections will be accepted.
// Existing connections continue until they finish.
func (s *Server) Stop() {
	if s.listener != nil {
		s.listener.Close()
	}
}

// routerDir is a directory that lazily fetches entries from the readdir handler
// in addition to its static tree children. It embeds fs.StaticDir so that
// AddChild/DeleteChild still work for modification-capable directories.
type routerDir struct {
	*fs.StaticDir
	server   *Server
	treeNode router.TreeNode
}

func newRouterDir(server *Server, stat *proto.Stat, tn router.TreeNode) *routerDir {
	return &routerDir{
		StaticDir: fs.NewStaticDir(stat),
		server:    server,
		treeNode:  tn,
	}
}

// Children returns the static tree children merged with dynamic entries from
// the readdir handler. Dynamic entries are created as routerFiles that proxy
// to the router, so param/wildcard/glob children are still handled by walkFail.
func (d *routerDir) Children() map[string]fs.FSNode {
	children := d.StaticDir.Children()

	// Check if this node has a readdir handler
	handlers := d.treeNode.GetHandlers()
	if _, hasList := handlers["readdir"]; !hasList {
		return children
	}

	dirPath := fs.FullPath(d)
	resp, err := d.server.router.Serve(&router.Request{
		Operation: "readdir",
		Path:      dirPath,
	})
	if err != nil || resp.Data == "" {
		return children
	}

	// Parse readdir response (one entry per line, optional trailing "/" for dirs)
	lines := strings.Split(strings.TrimSpace(resp.Data), "\n")
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		isDir := strings.HasSuffix(name, "/")
		if isDir {
			name = name[:len(name)-1]
		}
		if _, exists := children[name]; exists {
			continue
		}
		if isDir {
			stat := d.server.ninepFS.NewStat(name, "nobody", "nogroup", 0777|proto.DMDIR)
			// Use parent's treeNode for virtual dirs since there's no dedicated one
			vdir := newRouterDir(d.server, stat, d.treeNode)
			vdir.SetParent(d)
			children[name] = vdir
		} else {
			rf := newRouterFile(d.server, name, "nobody", "nogroup", 0666)
			rf.SetParent(d)
			children[name] = rf
		}
	}
	return children
}

// routerFile implements fs.File by proxying Read/Write/Open to the router.
// The file path is derived at runtime via fs.FullPath, and the router's
// radix tree handles pattern matching and parameter extraction.
type routerFile struct {
	*fs.BaseFile
	server     *Server
	cachedData string
	mu         sync.RWMutex
}

func newRouterFile(server *Server, name, uid, gid string, mode uint32) *routerFile {
	stat := server.ninepFS.NewStat(name, uid, gid, mode)
	return &routerFile{
		BaseFile: fs.NewBaseFile(stat),
		server:   server,
	}
}

func (f *routerFile) Open(fid uint64, omode proto.Mode) error {
	_, err := f.server.router.Serve(&router.Request{
		Operation: "open",
		Path:      fs.FullPath(f),
	})
	if err != nil && f.server.debug {
		log.Printf("9p: open error for %s: %v", fs.FullPath(f), err)
	}
	return nil
}

func (f *routerFile) Read(fid uint64, offset uint64, count uint64) ([]byte, error) {
	fullPath := fs.FullPath(f)

	// Check cached data first (populated by Write for write-only files,
	// or by a previous Read for read handler results).
	f.mu.RLock()
	cached := f.cachedData
	f.mu.RUnlock()
	if cached != "" {
		data := []byte(cached)
		if offset >= uint64(len(data)) {
			return []byte{}, nil
		}
		if offset+count > uint64(len(data)) {
			count = uint64(len(data)) - offset
		}
		return data[offset : offset+count], nil
	}

	// No cached data — try the read handler.
	resp, err := f.server.router.Serve(&router.Request{
		Operation: "read",
		Path:      fullPath,
		Offset:    int64(offset),
	})
	if err == nil {
		data := []byte(resp.Data)
		if offset >= uint64(len(data)) {
			return []byte{}, nil
		}
		if offset+count > uint64(len(data)) {
			count = uint64(len(data)) - offset
		}
		return data[offset : offset+count], nil
	}

	if f.server.debug {
		log.Printf("9p: read error for %s: %v", fullPath, err)
	}
	return []byte{}, nil
}

func (f *routerFile) Write(fid uint64, offset uint64, data []byte) (uint32, error) {
	fullPath := fs.FullPath(f)
	resp, err := f.server.router.Serve(&router.Request{
		Operation: "write",
		Path:      fullPath,
		Data:      data,
		Offset:    int64(offset),
	})
	if err != nil {
		if f.server.debug {
			log.Printf("9p: write error for %s: %v", fullPath, err)
		}
		return 0, err
	}
	// Only cache write data when there is no read handler.
	readHandler, _, _ := f.server.router.Find("read", fullPath)
	if readHandler == nil {
		f.mu.Lock()
		f.cachedData = resp.Data
		f.mu.Unlock()
	}
	// Return the number of bytes written from the input data.
	return uint32(len(data)), nil
}

func (f *routerFile) Close(fid uint64) error {
	// No close handler in the router currently.
	return nil
}
