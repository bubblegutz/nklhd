// Package fuse provides a minimal FUSE implementation tailored for nklhd.
// This file contains adaptations from bazil.org/fuse.
// Original bazil/fuse license:
//
// Copyright (c) 2013-2019 Tommi Virtanen.
// Copyright (c) 2009, 2011, 2012 The Go Authors.
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are
// met:
//
//    * Redistributions of source code must retain the above copyright
// notice, this list of conditions and the following disclaimer.
//    * Redistributions in binary form must reproduce the above
// copyright notice, this list of conditions and the following disclaimer
// in the documentation and/or other materials provided with the
// distribution.
//    * Neither the name of Google Inc. nor the names of its
// contributors may be used to endorse or promote products derived from
// this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
// "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
// LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
// A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
// OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
// LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
// DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
// THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
package fuse

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/bubblegutz/nklhd/internal/router"
)

// findChild finds a child node by name under the given parent node.
// Returns the child TreeNode and merged parameters (parent params + any new captures).
func findChild(parent router.TreeNode, name string, parentParams map[string]string) (router.TreeNode, map[string]string, bool) {
	children := parent.GetChildren()

	// Exact match first
	if child, ok := children[name]; ok {

		// Skip placeholder nodes that are meant for pattern matching
		if child.GetType() == router.NodeTypeParam || child.GetType() == router.NodeTypeWildcard || child.GetType() == router.NodeTypeGlob {
			// fall through to pattern matching

		} else {

			return child, parentParams, true
		}
	}
	// Glob pattern nodes
	for childName, child := range children {
		if child.GetType() == router.NodeTypeGlob {
			if router.ComponentMatches(childName, name) {
				// Glob matches, no param capture

				return child, parentParams, true
			}
		}
	}
	// Parameter nodes
	for childName, child := range children {
		if child.GetType() == router.NodeTypeParam {
			if name == childName {
				// Literal match, treat as exact (already handled above)
				continue
			}
			paramName := childName[1 : len(childName)-1] // remove braces
			params := make(map[string]string)
			for k, v := range parentParams {
				params[k] = v
			}
			params[paramName] = name

			return child, params, true
		}
	}
	// Wildcard nodes
	for _, child := range children {
		if child.GetType() == router.NodeTypeWildcard {
			params := make(map[string]string)
			for k, v := range parentParams {
				params[k] = v
			}
			params["*"] = name

			return child, params, true
		}
	}

	return nil, nil, false
}

// Server represents a FUSE server using bazil/fuse.
type Server struct {
	router     *router.Router
	opts       *MountOptions
	debug      bool
	mountPoint string
	conn       *fuse.Conn
	serveErr   error
	serveDone  chan struct{}
	mu         sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewServer creates a new FUSE server.
func NewServer(router *router.Router, opts *MountOptions) *Server {
	if opts == nil {
		opts = &MountOptions{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		router:    router,
		opts:      opts,
		debug:     opts.Debug,
		serveDone: make(chan struct{}),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// SetRouter updates the router used by the server.
func (s *Server) SetRouter(r *router.Router) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.router = r
}

// Mount mounts the filesystem at the given mount point.
func (s *Server) Mount(mountPoint string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.mountPoint != "" {
		return fmt.Errorf("already mounted")
	}
	
	// Prepare mount options
	mountOpts := []fuse.MountOption{
		fuse.FSName("nklhd"),
		fuse.Subtype("nklhd"),
	}
	if s.opts.AllowOther {
		mountOpts = append(mountOpts, fuse.AllowOther())
	}
	if s.opts.DefaultPermissions {
		mountOpts = append(mountOpts, fuse.DefaultPermissions())
	}
	if s.debug {
		// Debug logging disabled due to compilation error
		// mountOpts = append(mountOpts, fuse.Debug(func(msg interface{}) {
		//	log.Printf("fuse debug: %v", msg)
		// }))
	}
	
	// Call bazil/fuse Mount
	conn, err := fuse.Mount(mountPoint, mountOpts...)
	if err != nil {
		return fmt.Errorf("fuse mount failed: %v", err)
	}
	
	s.conn = conn
	s.mountPoint = mountPoint
	
	// Start serving in background
	go func(c *fuse.Conn) {
		err := s.serve(c)
		s.mu.Lock()
		s.serveErr = err
		s.mu.Unlock()
		close(s.serveDone)
	}(conn)
	
	return nil
}

// fusermountUnmount attempts to unmount a mount point using fusermount command.
func fusermountUnmount(mountPoint string) error {
	// Try to set working directory to parent directory (required by fusermount)
	parentDir := filepath.Dir(mountPoint)
	var cmd *exec.Cmd
	
	// First try lazy unmount (-z)
	cmd = exec.Command("fusermount", "-u", "-z", mountPoint)
	cmd.Dir = parentDir
	if _, err := cmd.CombinedOutput(); err == nil {
		return nil
	}
	// Try regular unmount
	cmd = exec.Command("fusermount", "-u", mountPoint)
	cmd.Dir = parentDir
	if _, err := cmd.CombinedOutput(); err == nil {
		return nil
	}
	// fusermount not found or failed
	return exec.ErrNotFound
}

// Unmount unmounts the filesystem.
func (s *Server) Unmount() error {
	s.mu.Lock()
	mountPoint := s.mountPoint
	conn := s.conn
	s.mu.Unlock()
	
	if mountPoint == "" {
		return nil
	}
	
	// Cancel the context to signal serve goroutine to exit
	if s.cancel != nil {
		s.cancel()
	}
	

	
	// Try fusermount first (recommended for FUSE)
	if err := fusermountUnmount(mountPoint); err == nil {
		if conn != nil {
			conn.Close()
		}
		s.clearMount()
		return nil
	}
	
	// Fallback to bazil/fuse unmount
	if err := fuse.Unmount(mountPoint); err == nil {
		if conn != nil {
			conn.Close()
		}
		s.clearMount()
		return nil
	}
	
	// Fallback to syscall unmount with lazy flag
	if err := syscall.Unmount(mountPoint, syscall.MNT_DETACH); err == nil {
		if conn != nil {
			conn.Close()
		}
		s.clearMount()
		return nil
	}
	
	// Final fallback: regular syscall unmount
	err := syscall.Unmount(mountPoint, 0)
	if err == nil {
		if conn != nil {
			conn.Close()
		}
		s.clearMount()
	}
	return err
}

// clearMount resets mount state after unmount.
func (s *Server) clearMount() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mountPoint = ""
	s.conn = nil
}

// Wait waits for the serve goroutine to finish and returns any error.
func (s *Server) Wait() error {
	<-s.serveDone
	s.mu.Lock()
	err := s.serveErr
	s.mu.Unlock()
	return err
}

// serve serves FUSE requests using bazil/fuse's fs.Serve.
func (s *Server) serve(conn *fuse.Conn) error {
	if s.debug {
		log.Printf("fuse serve starting")
	}
	// Create our filesystem implementation
	nklhdFS := &nklhdFS{
		router: s.router,
		debug:  s.debug,
	}
	
	// Serve the filesystem
	err := fs.Serve(conn, nklhdFS)
	if err != nil {
		if s.debug {
			log.Printf("fuse serve error: %v", err)
		}
		return err
	}

	if s.debug {
		log.Printf("fuse serve completed")
	}
	
	// Check if the mount process has an error to report
	// <-conn.Ready
	// if err := conn.MountError; err != nil {
	//	return err
	// }
	
	return nil
}

// nklhdFS implements fs.FS for the router tree.
type nklhdFS struct {
	router *router.Router
	debug  bool
}

// virtualTreeNode implements router.TreeNode for virtual entries from readdir handlers.
type virtualTreeNode struct {
	name     string
	isDir    bool
	handlers map[string]any
	children map[string]router.TreeNode
}

func (v *virtualTreeNode) GetName() string { return v.name }
func (v *virtualTreeNode) GetType() router.NodeType {
	if v.isDir {
		return router.NodeTypeDir
	}
	return router.NodeTypeFile
}
func (v *virtualTreeNode) GetPattern() string { return "" }
func (v *virtualTreeNode) GetChildren() map[string]router.TreeNode { return v.children }
func (v *virtualTreeNode) GetHandlers() map[string]any { return v.handlers }
func (v *virtualTreeNode) FindNode(components []string) (router.TreeNode, map[string]string) {
	// Virtual nodes don't support further lookup
	return nil, nil
}
func (v *virtualTreeNode) ListChildren() []string {
	names := make([]string, 0, len(v.children))
	for name := range v.children {
		names = append(names, name)
	}
	return names
}
func (v *virtualTreeNode) GetHandler(operation string) any {
	return v.handlers[operation]
}

func (f *nklhdFS) debugf(format string, args ...any) {
	if f.debug {
		log.Printf("fuse: "+format, args...)
	}
}

// Root returns the root node of the filesystem.
func (f *nklhdFS) Root() (fs.Node, error) {
	f.debugf("Root() called")
	root := f.router.GetTree()
	return &nklhdNode{
		fs:       f,
		treeNode: root,
		path:     "/",
		params:   make(map[string]string),
		inode:    FUSE_ROOT_ID,
	}, nil
}

// nklhdNode implements fs.Node for a router tree node.
type nklhdNode struct {
	mu              sync.RWMutex
	fs              *nklhdFS
	treeNode        router.TreeNode
	path            string
	params          map[string]string
	inode           uint64
	cachedData      string
	cachedSize      uint64
	cachedErr       error
	cachedListMap   map[string]bool // name -> isDir
	cachedListErr   error
}

// Attr returns file attributes.
func (n *nklhdNode) Attr(ctx context.Context, a *fuse.Attr) error {
	n.fs.debugf("Attr(%s) called", n.path)
	// Determine if this node is a directory
	isDir := n.isDir()
	
	a.Inode = n.inode
	if isDir {
		a.Mode = os.ModeDir | 0755
		a.Nlink = 2 // directory has at least 2 links (., ..)
	} else {
		a.Mode = 0644
		a.Nlink = 1
	}
	a.Uid = uint32(os.Getuid())
	a.Gid = uint32(os.Getgid())
	
	// Try to get attributes from getattr handler
	routerReq := &router.Request{
		Operation: "getattr",
		Path:      n.path,
		Params:    n.params,
		Data:      nil,
		Offset:    0,
	}
	routerResp, err := n.fs.router.Serve(routerReq)
	if err == nil && routerResp.Data != "" {
		// Parse key=value lines
		lines := splitLines(routerResp.Data)
		for _, line := range lines {
			line = trimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key, value := parts[0], parts[1]
			switch key {
			case "size":
				if size, err := strconv.ParseUint(value, 10, 64); err == nil {
					a.Size = size
					// Also update cached size for consistency
					n.mu.Lock()
					n.cachedSize = size
					n.mu.Unlock()
				}
			case "mode":
				if mode, err := strconv.ParseUint(value, 0, 32); err == nil {
					a.Mode = os.FileMode(mode)
				}
			case "uid":
				if uid, err := strconv.ParseUint(value, 10, 32); err == nil {
					a.Uid = uint32(uid)
				}
			case "gid":
				if gid, err := strconv.ParseUint(value, 10, 32); err == nil {
					a.Gid = uint32(gid)
				}
			case "atime":
				// TODO: parse time
			case "mtime":
				// TODO: parse time
			case "ctime":
				// TODO: parse time
			}
		}
	} else {
		// No getattr handler or error, use defaults
		if !isDir {
			// For files, fetch data to determine size (cached)
			if _, err := n.fetchData(); err != nil {
				n.fs.debugf("Attr(%s) fetchData error: %v", n.path, err)
				// Continue with size 0
			}
		}
		n.mu.RLock()
		a.Size = n.cachedSize
		n.mu.RUnlock()
	}
	
	a.Mtime = time.Now()
	a.Ctime = time.Now()
	a.Atime = time.Now()
	
	return nil
}

// isDir returns true if the node represents a directory.
func (n *nklhdNode) isDir() bool {
	if n.treeNode.GetType() == router.NodeTypeDir {
		return true
	}
	// Check if it has children (might be a directory with routes under it)
	children := n.treeNode.GetChildren()
	if len(children) > 0 {
		return true
	}
	// Check if it has a readdir handler (makes it a directory)
	handlers := n.treeNode.GetHandlers()
	if _, hasList := handlers["readdir"]; hasList {
		return true
	}
	return false
}

// getListEntries returns a map of entry names to isDir flag from the readdir handler.
// It caches the result for subsequent calls.
func (n *nklhdNode) getListEntries() (map[string]bool, error) {
	n.mu.RLock()
	listMap := n.cachedListMap
	listErr := n.cachedListErr
	n.mu.RUnlock()
	if listMap != nil || listErr != nil {
		return listMap, listErr
	}
	// Not cached, acquire write lock
	n.mu.Lock()
	defer n.mu.Unlock()
	// Double-check after acquiring lock
	if n.cachedListMap != nil || n.cachedListErr != nil {
		return n.cachedListMap, n.cachedListErr
	}
	// Call router.Serve for readdir operation
	routerReq := &router.Request{
		Operation: "readdir",
		Path:      n.path,
		Params:    n.params,
		Data:      nil,
		Offset:    -1,
	}
	routerResp, err := n.fs.router.Serve(routerReq)
	if err != nil {
		// No readdir handler or error - cache empty map
		n.cachedListMap = make(map[string]bool)
		n.cachedListErr = err
		return n.cachedListMap, err
	}
	// Parse readdir response
	data := routerResp.Data
	listMap = make(map[string]bool)
	if data != "" {
		lines := splitLines(data)
		for _, line := range lines {
			line = trimSpace(line)
			if line == "" {
				continue
			}
			isDir := false
			if len(line) > 0 && line[len(line)-1] == '/' {
				isDir = true
				line = line[:len(line)-1]
			}
			listMap[line] = isDir
		}
	}
	n.cachedListMap = listMap
	n.cachedListErr = nil
	return listMap, nil
}

// fetchData retrieves the file content by calling the router's read handler.
// It caches the result for subsequent calls.
func (n *nklhdNode) fetchData() (string, error) {
	n.mu.RLock()
	data := n.cachedData
	err := n.cachedErr
	n.mu.RUnlock()
	if data != "" || err != nil {
		return data, err
	}
	// Not cached, acquire write lock
	n.mu.Lock()
	defer n.mu.Unlock()
	// Double-check after acquiring lock
	if n.cachedData != "" || n.cachedErr != nil {
		return n.cachedData, n.cachedErr
	}
	// Call router.Serve for read operation
	routerReq := &router.Request{
		Operation: "read",
		Path:      n.path,
		Params:    n.params,
		Data:      nil,
		Offset:    0,
	}
	routerResp, err := n.fs.router.Serve(routerReq)
	if err != nil {
		if strings.Contains(err.Error(), "no handler registered") {
			// No read handler - treat as empty file, don't cache error
			n.cachedErr = nil
			return "", nil
		}
		n.cachedErr = err
		return "", err
	}
	n.cachedData = routerResp.Data
	n.cachedSize = uint64(len(routerResp.Data))
	return n.cachedData, nil
}

// Lookup looks up a child by name.
func (n *nklhdNode) Lookup(ctx context.Context, name string) (fs.Node, error) {
	n.fs.debugf("Lookup(%s, name=%s)", n.path, name)
	if !n.isDir() {
		return nil, syscall.ENOTDIR
	}
	// Debug: list children
	children := n.treeNode.GetChildren()
	var childNames []string
	for childName := range children {
		childNames = append(childNames, childName)
	}
	n.fs.debugf("Lookup(%s) children: %v", n.path, childNames)
	
	// Use findChild from server.go (we need to import it or copy logic)
	// For now, copy simplified logic
	childNode, params, found := findChild(n.treeNode, name, n.params)
	if found {
		// Build child path
		childPath := n.path
		if n.path != "/" {
			childPath += "/"
		}
		childPath += name
		
		// Generate a stable inode from path
		inode := hashString(childPath)
		
		n.fs.debugf("Lookup(%s, name=%s) found child path %s", n.path, name, childPath)
		return &nklhdNode{
			fs:       n.fs,
			treeNode: childNode,
			path:     childPath,
			params:   params,
			inode:    inode,
		}, nil
	}
	
	// Not found in tree, check if it's a virtual entry from readdir handler
	listMap, err := n.getListEntries()
	if err == nil {
		if isDir, ok := listMap[name]; ok {
			// Virtual entry exists
			childPath := n.path
			if n.path != "/" {
				childPath += "/"
			}
			childPath += name
			inode := hashString(childPath)
			
			// Create virtual tree node
			virtualNode := &virtualTreeNode{
				name:     name,
				isDir:    isDir,
				handlers: make(map[string]any),
				children: make(map[string]router.TreeNode),
			}
			
			n.fs.debugf("Lookup(%s, name=%s) found virtual entry, isDir=%v", n.path, name, isDir)
			return &nklhdNode{
				fs:       n.fs,
				treeNode: virtualNode,
				path:     childPath,
				params:   n.params, // Keep parent params
				inode:    inode,
			}, nil
		}
	}
	
	n.fs.debugf("Lookup(%s, name=%s) not found", n.path, name)
	return nil, syscall.ENOENT
}

// ReadDirAll returns directory entries.
func (n *nklhdNode) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	n.fs.debugf("ReadDirAll(%s) called", n.path)
	if !n.isDir() {
		return nil, syscall.ENOTDIR
	}
	
	var entries []fuse.Dirent
	
	// First, try to get entries from readdir handler if present
	routerReq := &router.Request{
		Operation: "readdir",
		Path:      n.path,
		Params:    n.params,
		Data:      nil,
		Offset:    -1,
	}
	
	routerResp, err := n.fs.router.Serve(routerReq)
	if err == nil {
		n.fs.debugf("ReadDirAll(%s) readdir handler succeeded", n.path)
		// Readdir handler returned data (newline-separated entries)
		// Parse the response
		data := routerResp.Data
		if data != "" {
			n.fs.debugf("ReadDirAll(%s) readdir handler returned %d bytes", n.path, len(data))
			lines := splitLines(data)
			for _, line := range lines {
				line = trimSpace(line)
				if line == "" {
					continue
				}
				
				entry := fuse.Dirent{
					Name: line,
				}
				
				// Check if entry ends with / indicating directory
				isDir := false
				if len(line) > 0 && line[len(line)-1] == '/' {
					isDir = true
					entry.Name = line[:len(line)-1]
				}
				
				// Determine entry type
				if isDir {
					entry.Type = fuse.DT_Dir
				} else {
					entry.Type = fuse.DT_File
				}
				
				// Generate inode
				childPath := n.path
				if n.path != "/" {
					childPath += "/"
				}
				childPath += entry.Name
				entry.Inode = hashString(childPath)
				
				entries = append(entries, entry)
			}
			n.fs.debugf("ReadDirAll(%s) returning %d entries from readdir handler", n.path, len(entries))
			return entries, nil
		} else {
			n.fs.debugf("ReadDirAll(%s) readdir handler returned empty data", n.path)
		}
	} else {
		n.fs.debugf("ReadDirAll(%s) readdir handler error: %v", n.path, err)
	}
	
	// Fall back to tree children
	children := n.treeNode.GetChildren()
	
	for name, child := range children {
		entry := fuse.Dirent{
			Name: name,
		}
		
		// Determine if child is a directory
		isDir := false
		if child.GetType() == router.NodeTypeDir {
			isDir = true
		} else if len(child.GetChildren()) > 0 {
			isDir = true
		}
		
		if isDir {
			entry.Type = fuse.DT_Dir
		} else {
			entry.Type = fuse.DT_File
		}
		
		// Generate inode for child
		childPath := n.path
		if n.path != "/" {
			childPath += "/"
		}
		childPath += name
		entry.Inode = hashString(childPath)
		
		entries = append(entries, entry)
	}
	
	n.fs.debugf("ReadDirAll(%s) returning %d entries from tree children", n.path, len(entries))
	return entries, nil
}

// Open opens a file for reading or writing.
func (n *nklhdNode) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (fs.Handle, error) {
	n.fs.debugf("Open(%s) flags=%v", n.path, req.Flags)
	// Call router.Serve for open operation if handler exists
	routerReq := &router.Request{
		Operation: "open",
		Path:      n.path,
		Params:    n.params,
		Data:      nil,
		Offset:    0,
	}
	_, err := n.fs.router.Serve(routerReq)
	if err != nil && !strings.Contains(err.Error(), "no handler registered") {
		n.fs.debugf("Open(%s) error: %v", n.path, err)
		// Still allow opening, just log error
	}
	// For now, just allow opening
	// TODO: check permissions, handle flags
	return n, nil
}

// Read reads file content.
func (n *nklhdNode) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	n.fs.debugf("Read(%s) offset=%d size=%d", n.path, req.Offset, req.Size)
	// Fetch cached data (or retrieve via router)
	data, err := n.fetchData()
	if err != nil {
		n.fs.debugf("Read(%s) error: %v", n.path, err)
		return err
	}
	n.fs.debugf("Read(%s) succeeded, cached length=%d", n.path, len(data))
	
	// Handle offset and size
	offset := int(req.Offset)
	if offset < 0 {
		offset = 0
	}
	if offset > len(data) {
		offset = len(data)
	}
	
	end := offset + req.Size
	if end > len(data) {
		end = len(data)
	}
	
	resp.Data = []byte(data)[offset:end]
	return nil
}

// Write writes file content.
func (n *nklhdNode) Write(ctx context.Context, req *fuse.WriteRequest, resp *fuse.WriteResponse) error {
	n.fs.debugf("Write(%s) offset=%d size=%d", n.path, req.Offset, len(req.Data))
	// Create request for router
	routerReq := &router.Request{
		Operation: "write",
		Path:      n.path,
		Params:    n.params,
		Data:      req.Data,
		Offset:    req.Offset,
	}
	
	// Execute via router
	routerResp, err := n.fs.router.Serve(routerReq)
	if err != nil {
		n.fs.debugf("Write(%s) error: %v", n.path, err)
		return err
	}
	n.fs.debugf("Write(%s) succeeded, response=%s", n.path, routerResp.Data)
	
	// Store response data as new file content for subsequent reads,
	// but only when there is no read handler (otherwise read+write are independent).
	readHandler, _, _ := n.fs.router.Find("read", n.path)
	n.mu.Lock()
	if readHandler == nil {
		n.cachedData = routerResp.Data
		n.cachedSize = uint64(len(routerResp.Data))
		n.cachedErr = nil
	} else {
		// Invalidate stale cache so the read handler is called next time.
		n.cachedData = ""
		n.cachedSize = 0
		n.cachedErr = nil
	}
	n.mu.Unlock()
	
	// Write handler should return something (could be empty)
	// For now, assume all data was written
	resp.Size = len(req.Data)
	return nil
}

// Create creates a new file in the directory.
func (n *nklhdNode) Create(ctx context.Context, req *fuse.CreateRequest, resp *fuse.CreateResponse) (fs.Node, fs.Handle, error) {
	n.fs.debugf("Create(%s) name=%s flags=%v mode=%v", n.path, req.Name, req.Flags, req.Mode)
	// Build child path
	childPath := n.path
	if n.path != "/" {
		childPath += "/"
	}
	childPath += req.Name
	// Call router.Serve for create operation
	routerReq := &router.Request{
		Operation: "create",
		Path:      childPath,
		Params:    n.params, // parent params (may be merged later)
		Data:      nil,
		Offset:    0,
	}
	_, err := n.fs.router.Serve(routerReq)
	if err != nil {
		n.fs.debugf("Create(%s) error: %v", n.path, err)
		return nil, nil, err
	}
	// Create virtual node for the new file
	childNode := &virtualTreeNode{
		name:     req.Name,
		isDir:    req.Mode&os.ModeDir != 0,
		handlers: make(map[string]any),
		children: make(map[string]router.TreeNode),
	}
	// Generate inode
	inode := hashString(childPath)
	// Return node and handle (same node)
	node := &nklhdNode{
		fs:       n.fs,
		treeNode: childNode,
		path:     childPath,
		params:   n.params, // keep parent params
		inode:    inode,
	}
	return node, node, nil
}

// Remove removes a file or directory from the directory.
func (n *nklhdNode) Remove(ctx context.Context, req *fuse.RemoveRequest) error {
	n.fs.debugf("Remove(%s) name=%s dir=%v", n.path, req.Name, req.Dir)
	// Build child path
	childPath := n.path
	if n.path != "/" {
		childPath += "/"
	}
	childPath += req.Name
	// Call router.Serve for unlink operation
	routerReq := &router.Request{
		Operation: "unlink",
		Path:      childPath,
		Params:    n.params,
		Data:      nil,
		Offset:    0,
	}
	_, err := n.fs.router.Serve(routerReq)
	if err != nil {
		n.fs.debugf("Remove(%s) error: %v", n.path, err)
		return err
	}
	return nil
}

// Setattr sets file attributes.
func (n *nklhdNode) Setattr(ctx context.Context, req *fuse.SetattrRequest, resp *fuse.SetattrResponse) error {
	n.fs.debugf("Setattr(%s) valid=%v size=%d mode=%v", n.path, req.Valid, req.Size, req.Mode)
	// Update cached size if size is being set
	if req.Valid.Size() {
		n.mu.Lock()
		n.cachedSize = req.Size
		// If we have cached data, truncate or extend it (simplify)
		// For now, just update size
		n.mu.Unlock()
	}
	// TODO: handle other attributes (mode, times, etc.)
	return nil
}

// splitLines splits a string by newlines, preserving empty lines.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, r := range s {
		if r == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	lines = append(lines, s[start:])
	return lines
}

// trimSpace removes leading and trailing whitespace.
func trimSpace(s string) string {
	// Simple implementation
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

// hashString creates a simple hash for inode numbers.
func hashString(s string) uint64 {
	var h uint64 = 14695981039346656037
	for _, c := range s {
		h ^= uint64(c)
		h *= 1099511628211
	}
	return h
}