// Package sshfs implements an SSH + SFTP server that bridges the router's
// route tree to the SFTP protocol, providing a third transport option
// alongside FUSE and 9p.
package sshfs

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bubblegutz/nklhd/internal/router"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/pkg/sftp"
)

// Server implements an SSH + SFTP server that bridges to the router.
type Server struct {
	router         *router.Router
	addr           string
	authorizedKeys string
	hostKeyPath    string
	debug          bool
	sshServer      *ssh.Server
	listener       net.Listener
	mu             sync.Mutex
}

// NewServer creates a new SSHFS server backed by the given router.
func NewServer(r *router.Router, addr, authorizedKeys, hostKeyPath string, debug bool) *Server {
	return &Server{
		router:         r,
		addr:           addr,
		authorizedKeys: authorizedKeys,
		hostKeyPath:    hostKeyPath,
		debug:          debug,
	}
}

// Serve starts the SSH server on the configured address. Blocks until
// the server stops (e.g., due to a network error).
func (s *Server) Serve() error {
	opts := []ssh.Option{
		wish.WithAddress(s.addr),
	}

	// Host key handling
	if s.hostKeyPath != "" {
		opts = append(opts, wish.WithHostKeyPath(s.hostKeyPath))
	} else {
		pem, err := generateHostKey()
		if err != nil {
			return fmt.Errorf("sshfs: generate host key: %w", err)
		}
		opts = append(opts, wish.WithHostKeyPEM(pem))
	}

	// Authorized keys — if no path given, allow any public key.
	if s.authorizedKeys != "" {
		opts = append(opts, wish.WithAuthorizedKeys(s.authorizedKeys))
	} else if s.debug {
		log.Println("sshfs: no authorized_keys file; allowing all public key auth")
	}

	// Main handler middleware: reject shell, handle scp exec, allow everything
	// else (subsystem requests are handled separately by wish).
	rfs := &routerFS{r: s.router, debug: s.debug}
	opts = append(opts, wish.WithMiddleware(func(next ssh.Handler) ssh.Handler {
		return func(session ssh.Session) {
			cmd := session.RawCommand()
			if cmd == "" {
				// Shell request — reject
				wish.Fatalf(session, "This SSH server only supports SFTP and SCP.\r\n")
				return
			}
			if strings.HasPrefix(cmd, "scp") {
				handleSCP(session, cmd, rfs)
				return
			}
			// Unknown exec command — reject
			wish.Fatalf(session, "Command not allowed: %s\r\n", cmd)
		}
	}))

	// SFTP subsystem
	opts = append(opts, wish.WithSubsystem("sftp", func(session ssh.Session) {
		if s.debug {
			log.Printf("sshfs: SFTP session from %s", session.RemoteAddr())
		}
		rwc := struct {
			io.Reader
			io.Writer
			io.Closer
		}{Reader: session, Writer: session, Closer: session}
		server := sftp.NewRequestServer(rwc, sftp.Handlers{
			FileGet:  rfs,
			FilePut:  rfs,
			FileCmd:  rfs,
			FileList: rfs,
		})
		if err := server.Serve(); err != nil && s.debug {
			log.Printf("sshfs: SFTP serve error: %v", err)
		}
	}))

	sshSrv, err := wish.NewServer(opts...)
	if err != nil {
		return fmt.Errorf("sshfs: create server: %w", err)
	}
	s.sshServer = sshSrv

	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("sshfs: listen on %s: %w", s.addr, err)
	}
	s.listener = listener

	if s.debug {
		log.Printf("sshfs: listening on %s", s.addr)
	}

	return sshSrv.Serve(listener)
}

// Stop gracefully stops the SSH server.
func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sshServer != nil {
		s.sshServer.Close()
	}
	if s.listener != nil {
		s.listener.Close()
	}
}

// ─── SCP handler ──────────────────────────────────────────────────────────────

// handleSCP implements the SCP protocol over an SSH session, bridging to the
// router for file I/O. SCP protocol:
//
//	sender:  "C%04o %lld %s\n"  (permissions, size, filename)
//	receiver: "\x00"             (ACK)
//	sender:  <file data of %lld bytes>
//	receiver: "\x00"             (ACK)
//
// Errors: "\x01<msg>\n" followed by "\x02".
func handleSCP(session ssh.Session, cmd string, rfs *routerFS) {
	var receive bool
	var path string

	// Parse "scp -t /path" (receive=store) or "scp -f /path" (send=fetch)
	// Also handle -p (preserve times), -r (recursive), -d (target is dir)
	fields := strings.Fields(cmd)
	for i, f := range fields {
		switch {
		case f == "scp":
			continue
		case f == "-t":
			receive = true
		case f == "-f":
			receive = false
		case f == "-p" || f == "-r" || f == "-d":
			continue
		case f == "-v":
			continue
		default:
			if strings.HasPrefix(f, "-") {
				// Unknown flag — skip it
				continue
			}
			path = f
			// If there are more non-flag fields after path, it's
			// the basename of the destination file; join them.
			for j := i + 1; j < len(fields); j++ {
				if !strings.HasPrefix(fields[j], "-") {
					path = filepath.Join(path, fields[j])
				}
			}
			break
		}
	}

	if path == "" {
		sendSCPError(session, "no path specified")
		return
	}

	// Send initial ACK
	if _, err := session.Write([]byte{0}); err != nil {
		return
	}

	if receive {
		receiveSCP(session, path, rfs)
	} else {
		sendSCP(session, path, rfs)
	}
}

func receiveSCP(session ssh.Session, path string, rfs *routerFS) {
	buf := make([]byte, 1)
	for {
		// Read warning byte — 0 = OK, 1 = error, 2 = fatal
		_, err := io.ReadFull(session, buf)
		if err != nil {
			return
		}
		if buf[0] != 0 {
			// Read error message
			errMsg, _ := readSCPLine(session)
			if rfs.debug {
				log.Printf("sshfs: scp receive warning: %s", errMsg)
			}
			if buf[0] == 2 {
				return
			}
			// Warning — skip and continue
			continue
		}

		// Read metadata line: "C%04o %lld %s\n" or "D%04o 0 %s\n" or "E\n"
		line, err := readSCPLine(session)
		if err != nil {
			return
		}
		if line == "" {
			continue
		}
		if line == "E" {
			// End of directory — not fully supported, just ACK
			session.Write([]byte{0})
			return
		}
		if line[0] == 'D' {
			// Directory — not fully supported, ACK and skip content
			session.Write([]byte{0})
			continue
		}
		if line[0] != 'C' {
			sendSCPError(session, fmt.Sprintf("unexpected SCP message: %s", line))
			return
		}

		// Parse "C%04o %lld %s"
		var mode uint32
		var size int64
		var name string
		if _, err := fmt.Sscanf(line, "C%04o %d %s", &mode, &size, &name); err != nil {
			sendSCPError(session, fmt.Sprintf("bad SCP metadata: %s", line))
			return
		}

		fullPath := path
		if name != "" && name != "." {
			fullPath = filepath.Join(path, name)
		}

		// ACK the metadata
		session.Write([]byte{0})

		// Read file data
		data := make([]byte, size)
		if _, err := io.ReadFull(session, data); err != nil {
			return
		}

		// Write to router
		_, err = rfs.r.Serve(&router.Request{
			Operation: "write",
			Path:      fullPath,
			Data:      data,
			Offset:    0,
		})
		if err != nil && rfs.debug {
			log.Printf("sshfs: scp write error for %s: %v", fullPath, err)
		}

		// Read trailing ACK (client sends \x00 after file data)
		io.ReadFull(session, buf)
		// Send final ACK
		session.Write([]byte{0})

		// For single file transfers, SCP sends just one file
		return
	}
}

func sendSCP(session ssh.Session, path string, rfs *routerFS) {
	resp, err := rfs.r.Serve(&router.Request{
		Operation: "read",
		Path:      path,
	})
	if err != nil {
		sendSCPError(session, fmt.Sprintf("file not found: %s", path))
		return
	}

	data := []byte(resp.Data)
	name := filepath.Base(path)

	// Send metadata
	meta := fmt.Sprintf("C0644 %d %s\n", len(data), name)
	if _, err := session.Write([]byte(meta)); err != nil {
		return
	}

	// Wait for ACK
	buf := make([]byte, 1)
	if _, err := io.ReadFull(session, buf); err != nil || buf[0] != 0 {
		return
	}

	// Send file data
	if _, err := session.Write(data); err != nil {
		return
	}

	// Wait for ACK
	io.ReadFull(session, buf)
}

func readSCPLine(session ssh.Session) (string, error) {
	var buf bytes.Buffer
	for {
		b := make([]byte, 1)
		_, err := io.ReadFull(session, b)
		if err != nil {
			return "", err
		}
		if b[0] == '\n' {
			return buf.String(), nil
		}
		buf.WriteByte(b[0])
	}
}

func sendSCPError(session ssh.Session, msg string) {
	session.Write([]byte{1})
	session.Write([]byte(msg + "\n"))
	session.Write([]byte{2})
}

// ─── SFTP handlers ────────────────────────────────────────────────────────────

// routerFS bridges SFTP operations to the router. It implements
// sftp.FileReader, sftp.FileWriter, sftp.FileCmder, and sftp.FileLister.
type routerFS struct {
	r          *router.Router
	debug      bool
	writeCache sync.Map // path → string, caches write response data for subsequent reads
}

func (f *routerFS) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	path := r.Filepath
	if f.debug {
		log.Printf("sshfs: sftp read %s", path)
	}
	resp, err := f.r.Serve(&router.Request{
		Operation: "read",
		Path:      path,
	})
	if err != nil {
		if strings.Contains(err.Error(), "no handler registered") {
			// No read handler — check write cache (populated by write operations).
			if cached, ok := f.writeCache.Load(path); ok {
				if f.debug {
					log.Printf("sshfs: sftp read %s from write cache", path)
				}
				return bytes.NewReader([]byte(cached.(string))), nil
			}
		}
		if f.debug {
			log.Printf("sshfs: sftp read %s error: %v", path, err)
		}
		return nil, sftp.ErrSSHFxNoSuchFile
	}
	return bytes.NewReader([]byte(resp.Data)), nil
}

func (f *routerFS) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	path := r.Filepath
	if f.debug {
		log.Printf("sshfs: sftp write %s", path)
	}
	return &writeBuffer{r: f.r, path: path, fs: f, debug: f.debug}, nil
}

func (f *routerFS) Filecmd(r *sftp.Request) error {
	path := r.Filepath
	switch r.Method {
	case "Remove", "Rmdir":
		if f.debug {
			log.Printf("sshfs: sftp %s %s", r.Method, path)
		}
		_, err := f.r.Serve(&router.Request{
			Operation: "unlink",
			Path:      path,
		})
		if err != nil {
			return sftp.ErrSSHFxNoSuchFile
		}
		return nil
	case "Mkdir", "Rename", "Symlink":
		return sftp.ErrSSHFxOpUnsupported
	case "Setstat":
		return nil
	}
	return sftp.ErrSSHFxOpUnsupported
}

func (f *routerFS) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	path := r.Filepath
	switch r.Method {
	case "Stat", "Lstat":
		fi := f.getFileInfo(path)
		return &listerAtAdapter{items: []os.FileInfo{fi}}, nil
	case "List":
		entries, err := f.readDir(path)
		if err != nil {
			return nil, sftp.ErrSSHFxFailure
		}
		return &listerAtAdapter{items: entries}, nil
	case "Realpath":
		cleaned := filepath.Clean(path)
		return &listerAtAdapter{items: []os.FileInfo{
			&namedFileInfo{name: cleaned, isDir: strings.HasSuffix(path, "/")},
		}}, nil
	case "Readlink":
		return nil, sftp.ErrSSHFxOpUnsupported
	}
	return nil, sftp.ErrSSHFxOpUnsupported
}

func (f *routerFS) getFileInfo(path string) os.FileInfo {
	resp, err := f.r.Serve(&router.Request{
		Operation: "getattr",
		Path:      path,
	})
	fi := &routerFileInfo{name: filepath.Base(path)}
	if err == nil && resp.Data != "" {
		for _, line := range strings.Split(resp.Data, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			switch parts[0] {
			case "size":
				if size, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
					fi.size = size
				}
			case "mode":
				if mode, err := strconv.ParseUint(parts[1], 0, 32); err == nil {
					fi.mode = os.FileMode(mode)
				}
			case "uid":
				if uid, err := strconv.ParseUint(parts[1], 10, 32); err == nil {
					fi.uid = uint32(uid)
				}
			case "gid":
				if gid, err := strconv.ParseUint(parts[1], 10, 32); err == nil {
					fi.gid = uint32(gid)
				}
			}
		}
	}
	// Check for readdir handler to determine if it's a directory
	handler, _, _ := f.r.Find("readdir", path)
	if handler != nil {
		fi.isDir = true
		fi.mode |= os.ModeDir
	}
	return fi
}

func (f *routerFS) readDir(path string) ([]os.FileInfo, error) {
	// First check if a readdir handler is explicitly registered for this path.
	handler, _, _ := f.r.Find("readdir", path)
	if handler != nil {
		resp, err := f.r.Serve(&router.Request{
			Operation: "readdir",
			Path:      path,
		})
		if err != nil {
			return nil, err
		}
		lines := strings.Split(strings.TrimSpace(resp.Data), "\n")
		entries := make([]os.FileInfo, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			isDir := strings.HasSuffix(line, "/")
			name := strings.TrimSuffix(line, "/")
			entries = append(entries, &namedFileInfo{name: name, isDir: isDir})
		}
		return entries, nil
	}

	// No explicit readdir handler — list children from the route tree.
	node, _ := f.r.GetTree().FindNode(splitPath(path))
	if node == nil {
		return nil, fmt.Errorf("path not found: %s", path)
	}
	children := node.ListChildren()
	entries := make([]os.FileInfo, 0, len(children))
	for _, name := range children {
		// Check if the child has a readdir handler (i.e., is a directory).
		childPath := path + "/" + name
		if path == "/" {
			childPath = "/" + name
		}
		childNode, _ := f.r.GetTree().FindNode(splitPath(childPath))
		_, isDir := false, false
		if childNode != nil {
			_, hasList := childNode.GetHandlers()["readdir"]
			_, hasGetAttr := childNode.GetHandlers()["getattr"]
			if hasList {
				isDir = true
			}
			// Also check if child has any sub-children
			if len(childNode.GetChildren()) > 0 {
				isDir = true
			}
			// If child has a file handler (read/write) but no readdir, it's a file
			if !isDir && !hasGetAttr {
				// Check for any operation handler that indicates a file
				for op := range childNode.GetHandlers() {
					if op == "read" || op == "write" || op == "create" || op == "unlink" {
						isDir = false
						break
					}
				}
			}
		}
		entries = append(entries, &namedFileInfo{name: name, isDir: isDir})
	}
	return entries, nil
}

// splitPath splits a path into components, ignoring empty strings.
func splitPath(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return []string{}
	}
	return strings.Split(path, "/")
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func generateHostKey() ([]byte, error) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	marshal, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: marshal,
	}), nil
}

// writeBuffer buffers write data and sends it to the router on each WriteAt.
type writeBuffer struct {
	r     *router.Router
	path  string
	debug bool
	fs    *routerFS
	mu    sync.Mutex
	data  []byte
}

func (wb *writeBuffer) WriteAt(p []byte, off int64) (int, error) {
	wb.mu.Lock()
	defer wb.mu.Unlock()
	if int(off)+len(p) > len(wb.data) {
		newData := make([]byte, int(off)+len(p))
		copy(newData, wb.data)
		wb.data = newData
	}
	copy(wb.data[off:], p)
	resp, err := wb.r.Serve(&router.Request{
		Operation: "write",
		Path:      wb.path,
		Data:      wb.data[:int(off)+len(p)],
		Offset:    0,
	})
	if err != nil {
		if wb.debug {
			log.Printf("sshfs: sftp write error for %s: %v", wb.path, err)
		}
	} else if wb.fs != nil {
		// Cache the write response for subsequent reads (e.g., echo handlers).
		if resp != nil && resp.Data != "" {
			if wb.debug {
				log.Printf("sshfs: caching write response for %s", wb.path)
			}
			wb.fs.writeCache.Store(wb.path, resp.Data)
		}
	}
	return len(p), nil
}

// listerAtAdapter wraps []os.FileInfo to implement sftp.ListerAt.
type listerAtAdapter struct {
	items []os.FileInfo
	idx   int
}

func (l *listerAtAdapter) ListAt(fis []os.FileInfo, offset int64) (int, error) {
	if offset >= int64(len(l.items)) {
		return 0, io.EOF
	}
	n := copy(fis, l.items[offset:])
	if offset > 0 && offset+int64(n) >= int64(len(l.items)) {
		return n, io.EOF
	}
	if n < len(fis) {
		return n, io.EOF
	}
	return n, nil
}

// routerFileInfo implements os.FileInfo for router responses.
type routerFileInfo struct {
	name  string
	size  int64
	mode  os.FileMode
	mod   time.Time
	isDir bool
	uid   uint32
	gid   uint32
}

func (fi *routerFileInfo) Name() string      { return fi.name }
func (fi *routerFileInfo) Size() int64        { return fi.size }
func (fi *routerFileInfo) Mode() os.FileMode  { return fi.mode }
func (fi *routerFileInfo) ModTime() time.Time { return fi.mod }
func (fi *routerFileInfo) IsDir() bool        { return fi.isDir || fi.mode.IsDir() }
func (fi *routerFileInfo) Sys() any           { return nil }

// namedFileInfo is a simple os.FileInfo with name and isDir flag.
type namedFileInfo struct {
	name  string
	isDir bool
}

func (fi *namedFileInfo) Name() string      { return fi.name }
func (fi *namedFileInfo) Size() int64        { return 0 }
func (fi *namedFileInfo) Mode() os.FileMode  { return 0644 }
func (fi *namedFileInfo) ModTime() time.Time { return time.Now() }
func (fi *namedFileInfo) IsDir() bool        { return fi.isDir }
func (fi *namedFileInfo) Sys() any           { return nil }
