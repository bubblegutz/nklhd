// Package fuse provides a minimal FUSE implementation tailored for nklhd.
package fuse

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path"
	"strings"
	"syscall"
)

var debug bool

// SetDebug enables or disables debug logging for FUSE operations.
func SetDebug(d bool) {
	debug = d
}

// unixgramSocketpair creates a pair of connected UNIX domain socket.
func unixgramSocketpair() (l, r *os.File, err error) {
	fd, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_SEQPACKET, 0)
	if err != nil {
		return nil, nil, os.NewSyscallError("socketpair", err.(syscall.Errno))
	}
	l = os.NewFile(uintptr(fd[0]), "socketpair-half1")
	r = os.NewFile(uintptr(fd[1]), "socketpair-half2")
	return
}

// getConnection reads the file descriptor from the socket.
func getConnection(local *os.File) (int, error) {
	conn, err := net.FileConn(local)
	if err != nil {
		return 0, err
	}
	defer func() { _ = conn.Close() }()
	unixConn := conn.(*net.UnixConn)

	var data [4]byte
	control := make([]byte, 4*256)

	_, oobn, _, _, err := unixConn.ReadMsgUnix(data[:], control[:])
	if err != nil {
		return 0, err
	}

	messages, err := syscall.ParseSocketControlMessage(control[:oobn])
	if err != nil {
		return 0, err
	}
	if len(messages) != 1 {
		return 0, fmt.Errorf("getConnection: expect 1 control message, got %#v", messages)
	}
	message := messages[0]

	fds, err := syscall.ParseUnixRights(&message)
	if err != nil {
		return 0, err
	}
	if len(fds) != 1 {
		return 0, fmt.Errorf("getConnection: expect 1 fd, got %#v", fds)
	}
	fd := fds[0]

	if fd < 0 {
		return 0, fmt.Errorf("getConnection: fd < 0: %d", fd)
	}
	return fd, nil
}

// MountOptions holds options for mounting a FUSE filesystem.
type MountOptions struct {
	// Debug enables verbose logging.
	Debug bool
	// AllowOther allows other users to access the filesystem.
	AllowOther bool
	// DefaultPermissions enables kernel permission checking.
	DefaultPermissions bool
	// FsName sets the filesystem name (optional).
	FsName string
	// Name sets the subtype name (optional).
	Name string
	// Options is a list of extra mount options.
	Options []string
}

// optionsStrings returns mount options as strings for fusermount.
func (opts *MountOptions) optionsStrings() []string {
	var options []string
	if opts.AllowOther {
		options = append(options, "allow_other")
	}
	if opts.DefaultPermissions {
		options = append(options, "default_permissions")
	}
	options = append(options, opts.Options...)
	return options
}

// Mount mounts a new FUSE filesystem at mountPoint and returns a file descriptor
// for the FUSE device. The caller must close the fd when done.
func Mount(mountPoint string, opts *MountOptions) (fd int, err error) {
	if opts == nil {
		opts = &MountOptions{}
	}
	opts.Debug = opts.Debug || debug
	// Try direct mount first (open /dev/fuse, then mount syscall)
	fd, err = mountDirect(mountPoint, opts)
	if err == nil {
		return fd, nil
	}
	// Fallback to fusermount helper
	return mountViaFusermount(mountPoint, opts)
}

// mountDirect attempts to mount directly using syscall.Mount.
func mountDirect(mountPoint string, opts *MountOptions) (fd int, err error) {
	// Open /dev/fuse
	fd, err = syscall.Open("/dev/fuse", os.O_RDWR, 0)
	if err != nil {
		return -1, fmt.Errorf("open /dev/fuse: %v", err)
	}
	defer func() {
		if err != nil {
			_ = syscall.Close(fd)
		}
	}()

	// Get mount point stats to determine rootmode
	var st syscall.Stat_t
	if err := syscall.Stat(mountPoint, &st); err != nil {
		return -1, fmt.Errorf("stat mount point: %v", err)
	}
	rootmode := st.Mode & syscall.S_IFMT

	// Build options string for fuse
	options := []string{
		fmt.Sprintf("fd=%d", fd),
		fmt.Sprintf("rootmode=%o", rootmode),
		fmt.Sprintf("user_id=%d", os.Geteuid()),
		fmt.Sprintf("group_id=%d", os.Getegid()),
		// default max_read, use 128k similar to fusermount
		fmt.Sprintf("max_read=%d", 128*1024),
	}
	if opts.AllowOther {
		options = append(options, "allow_other")
	}
	if opts.DefaultPermissions {
		options = append(options, "default_permissions")
	}
	// Mount flags
	flags := uintptr(syscall.MS_NOSUID | syscall.MS_NODEV)
	// Parse options that affect flags and collect non-flag options
	for _, o := range opts.Options {
		switch o {
		case "nodev":
			flags |= syscall.MS_NODEV
		case "dev":
			flags &^= syscall.MS_NODEV
		case "nosuid":
			flags |= syscall.MS_NOSUID
		case "suid":
			flags &^= syscall.MS_NOSUID
		case "noexec":
			flags |= syscall.MS_NOEXEC
		case "exec":
			flags &^= syscall.MS_NOEXEC
		default:
			options = append(options, o)
		}
	}

	fsName := opts.FsName
	if fsName == "" {
		fsName = opts.Name
	}
	if fsName == "" {
		fsName = "nklhd"
	}
	fuseType := "fuse." + fsName

	if opts.Debug {
		fmt.Printf("mountDirect: calling syscall.Mount(%q, %q, %q, 0x%x, %q)\n",
			fsName, mountPoint, fuseType, flags, strings.Join(options, ","))
	}
	err = syscall.Mount(fsName, mountPoint, fuseType, flags, strings.Join(options, ","))
	if err != nil {
		return -1, fmt.Errorf("syscall.Mount: %v", err)
	}
	// Success - fd is now connected to the mounted filesystem
	return fd, nil
}

// mountViaFusermount uses the fusermount helper (setuid) to mount.
func mountViaFusermount(mountPoint string, opts *MountOptions) (fd int, err error) {
	local, remote, err := unixgramSocketpair()
	if err != nil {
		return -1, err
	}
	defer func() { _ = local.Close() }()
	defer func() { _ = remote.Close() }()

	bin, err := fusermountBinary()
	if err != nil {
		return -1, fmt.Errorf("cannot find fusermount binary: %v", err)
	}

	// Build command line options
	args := []string{bin, mountPoint}
	if options := opts.optionsStrings(); len(options) > 0 {
		args = append(args, "-o", strings.Join(options, ","))
	}
	if opts.Debug {
		fmt.Printf("mountViaFusermount: executing %v\n", args)
	}
	proc, err := os.StartProcess(bin,
		args,
		&os.ProcAttr{
			Env:   []string{"_FUSE_COMMFD=3"},
			Files: []*os.File{os.Stdin, os.Stdout, os.Stderr, remote}})
	if err != nil {
		return -1, err
	}

	w, err := proc.Wait()
	if err != nil {
		return -1, err
	}
	if opts.Debug {
		fmt.Printf("fusermount exited: %v\n", w.Sys())
	}
	if !w.Success() {
		return -1, fmt.Errorf("fusermount exited with code %v", w.Sys())
	}

	fd, err = getConnection(local)
	if err != nil {
		return -1, err
	}
	if opts.Debug {
		fmt.Printf("mountViaFusermount: got fd %d\n", fd)
	}
	return fd, nil
}

// Unmount unmounts the filesystem at mountPoint.
func Unmount(mountPoint string, opts *MountOptions) error {
	if opts == nil {
		opts = &MountOptions{}
	}
	opts.Debug = opts.Debug || debug
	// Try direct unmount first
	if err := syscall.Unmount(mountPoint, 0); err == nil {
		return nil
	}
	// Fallback to fusermount -u
	bin, err := fusermountBinary()
	if err != nil {
		return fmt.Errorf("cannot find fusermount binary: %v", err)
	}
	cmd := exec.Command(bin, "-u", mountPoint)
	if opts.Debug {
		fmt.Printf("Unmount: executing %v\n", cmd.Args)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("fusermount -u failed: %v, output: %s", err, out)
	}
	return nil
}

// fusermountBinary returns the path to fusermount3 or fusermount.
func fusermountBinary() (string, error) {
	if path, err := lookPathFallback("fusermount3", "/bin"); err == nil {
		return path, nil
	}
	return lookPathFallback("fusermount", "/bin")
}

// lookPathFallback looks for a binary in PATH, then in fallbackDir.
func lookPathFallback(file, fallbackDir string) (string, error) {
	binPath, err := exec.LookPath(file)
	if err == nil {
		return binPath, nil
	}
	abs := path.Join(fallbackDir, file)
	return exec.LookPath(abs)
}