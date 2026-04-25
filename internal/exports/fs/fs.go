// Package fs provides generic filesystem operations following C++ std::filesystem API.
package fs

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Copy copies files and directories.
// If src is a directory and dst doesn't exist, creates dst and copies contents.
// If recursive is true, copies directories recursively.
// If copySymlinks is true, copies symlinks as symlinks (otherwise follows them).
// If skipExisting is true, skips files that already exist in destination.
// If overwriteExisting is true, overwrites existing files.
// If updateExisting is true, overwrites only if source is newer.
// If directoriesOnly is true, copies only directory structure, not files.
func Copy(src, dst string, recursive, copySymlinks, skipExisting, overwriteExisting, updateExisting, directoriesOnly bool) error {
	srcInfo, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("stat src: %w", err)
	}

	// Handle symlink
	if srcInfo.Mode()&os.ModeSymlink != 0 {
		if copySymlinks {
			// Read symlink target
			target, err := os.Readlink(src)
			if err != nil {
				return fmt.Errorf("readlink: %w", err)
			}
			// Check if dst exists and is directory
			dstInfo, err := os.Stat(dst)
			if err == nil && dstInfo.IsDir() {
				dst = filepath.Join(dst, filepath.Base(src))
			}
			return os.Symlink(target, dst)
		}
		// Follow symlink: treat src as its target
		src, err = os.Readlink(src)
		if err != nil {
			return fmt.Errorf("readlink: %w", err)
		}
		srcInfo, err = os.Stat(src)
		if err != nil {
			return fmt.Errorf("stat symlink target: %w", err)
		}
	}

	if srcInfo.IsDir() {
		if !recursive {
			return errors.New("src is a directory; use recursive=true to copy directories")
		}
		return copyDir(src, dst, copySymlinks, skipExisting, overwriteExisting, updateExisting, directoriesOnly)
	}
	return copyFile(src, dst, skipExisting, overwriteExisting, updateExisting)
}

func copyFile(src, dst string, skipExisting, overwriteExisting, updateExisting bool) error {
	// If dst is a directory, copy into it with same basename
	dstInfo, err := os.Stat(dst)
	if err == nil && dstInfo.IsDir() {
		dst = filepath.Join(dst, filepath.Base(src))
	}

	// Check if destination exists
	if _, err := os.Stat(dst); err == nil {
		if skipExisting {
			return nil
		}
		if overwriteExisting {
			// proceed
		} else if updateExisting {
			srcInfo, err := os.Stat(src)
			if err != nil {
				return fmt.Errorf("stat src: %w", err)
			}
			dstInfo, err := os.Stat(dst)
			if err != nil {
				return fmt.Errorf("stat dst: %w", err)
			}
			if !srcInfo.ModTime().After(dstInfo.ModTime()) {
				return nil // source not newer
			}
		} else {
			return fmt.Errorf("destination exists: %s", dst)
		}
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create dst: %w", err)
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	return dstFile.Sync()
}

func copyDir(src, dst string, copySymlinks, skipExisting, overwriteExisting, updateExisting, directoriesOnly bool) error {
	// Create destination directory
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	if directoriesOnly {
		return nil
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		entryInfo, err := os.Lstat(srcPath)
		if err != nil {
			return err
		}

		if entryInfo.Mode()&os.ModeSymlink != 0 && copySymlinks {
			target, err := os.Readlink(srcPath)
			if err != nil {
				return fmt.Errorf("readlink: %w", err)
			}
			if err := os.Symlink(target, dstPath); err != nil {
				return fmt.Errorf("symlink: %w", err)
			}
			continue
		}

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath, copySymlinks, skipExisting, overwriteExisting, updateExisting, directoriesOnly); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath, skipExisting, overwriteExisting, updateExisting); err != nil {
				return err
			}
		}
	}
	return nil
}

// Rename moves or renames a file or directory from src to dst.
func Rename(src, dst string) error {
	return os.Rename(src, dst)
}

// Remove removes a file or empty directory.
// Returns true if the file was removed, false if it didn't exist.
func Remove(path string) (bool, error) {
	err := os.Remove(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// RemoveAll removes a file or directory recursively.
// Returns the number of removed filesystem objects.
func RemoveAll(path string) (int, error) {
	// Walk the tree counting entries
	var count int
	err := filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		count++
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	err = os.RemoveAll(path)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// Exists returns true if the path exists (file, directory, or symlink).
func Exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// IsDirectory returns true if the path exists and is a directory.
func IsDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// IsRegularFile returns true if the path exists and is a regular file.
func IsRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// CreateDirectory creates a directory at the given path, including parent directories.
func CreateDirectory(path string) error {
	return os.MkdirAll(path, 0755)
}

// CurrentPath returns the current working directory.
func CurrentPath() (string, error) {
	return os.Getwd()
}

// TempDirectoryPath returns the system temporary directory.
func TempDirectoryPath() string {
	return os.TempDir()
}

// List returns a slice of directory entry names for the given path.
// If recursive is true, returns paths relative to the starting directory.
func List(path string, recursive bool) ([]string, error) {
	if recursive {
		var entries []string
		err := filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(path, p)
			if err != nil {
				return err
			}
			if rel == "." {
				return nil
			}
			entries = append(entries, rel)
			return nil
		})
		return entries, err
	}
	dirEntries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(dirEntries))
	for i, entry := range dirEntries {
		names[i] = entry.Name()
	}
	return names, nil
}

// FileSize returns the size of a regular file in bytes.
func FileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() {
		return 0, errors.New("not a regular file")
	}
	return info.Size(), nil
}

// FileType represents the type of a filesystem entry.
type FileType int

const (
	FileTypeNone FileType = iota
	FileTypeNotFound
	FileTypeRegular
	FileTypeDirectory
	FileTypeSymlink
	FileTypeBlock
	FileTypeCharacter
	FileTypeFIFO
	FileTypeSocket
	FileTypeUnknown
)

// FileStatus contains file type and permissions.
type FileStatus struct {
	Type       FileType
	Permissions uint32
}

// Status returns the file status (following symlinks).
func Status(path string) (*FileStatus, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &FileStatus{Type: FileTypeNotFound}, nil
		}
		return nil, err
	}
	return statusFromFileInfo(info), nil
}

// SymlinkStatus returns the file status without following symlinks.
func SymlinkStatus(path string) (*FileStatus, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &FileStatus{Type: FileTypeNotFound}, nil
		}
		return nil, err
	}
	return statusFromFileInfo(info), nil
}

func statusFromFileInfo(info os.FileInfo) *FileStatus {
	fs := &FileStatus{}
	mode := info.Mode()
	switch {
	case mode.IsRegular():
		fs.Type = FileTypeRegular
	case mode.IsDir():
		fs.Type = FileTypeDirectory
	case mode&os.ModeSymlink != 0:
		fs.Type = FileTypeSymlink
	case mode&os.ModeDevice != 0:
		if mode&os.ModeCharDevice != 0 {
			fs.Type = FileTypeCharacter
		} else {
			fs.Type = FileTypeBlock
		}
	case mode&os.ModeNamedPipe != 0:
		fs.Type = FileTypeFIFO
	case mode&os.ModeSocket != 0:
		fs.Type = FileTypeSocket
	default:
		fs.Type = FileTypeUnknown
	}
	fs.Permissions = uint32(mode.Perm())
	return fs
}

// Permissions changes file permissions.
// perm is a Unix permission mask (e.g., 0644).
func Permissions(path string, perm os.FileMode) error {
	return os.Chmod(path, perm)
}

// ReadSymlink returns the target of a symbolic link.
func ReadSymlink(path string) (string, error) {
	return os.Readlink(path)
}

// CreateSymlink creates a symbolic link.
func CreateSymlink(target, link string) error {
	return os.Symlink(target, link)
}

// CreateHardLink creates a hard link.
func CreateHardLink(target, link string) error {
	return os.Link(target, link)
}

// SpaceInfo contains filesystem space information.
type SpaceInfo struct {
	Capacity  uint64
	Free      uint64
	Available uint64
}

// Space returns space information for the filesystem containing path.
func Space(path string) (*SpaceInfo, error) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(path, &stat)
	if err != nil {
		return nil, err
	}
	// Block size may differ; use Bsize for block size in bytes
	blockSize := uint64(stat.Bsize)
	return &SpaceInfo{
		Capacity:  stat.Blocks * blockSize,
		Free:      stat.Bfree * blockSize,
		Available: stat.Bavail * blockSize,
	}, nil
}

// ResizeFile changes the size of a regular file.
func ResizeFile(path string, size int64) error {
	return os.Truncate(path, size)
}

// LastWriteTime returns the last modification time of a file.
func LastWriteTime(path string) (time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

// Equivalent returns true if two paths refer to the same filesystem entity.
func Equivalent(path1, path2 string) (bool, error) {
	info1, err := os.Stat(path1)
	if err != nil {
		return false, err
	}
	info2, err := os.Stat(path2)
	if err != nil {
		return false, err
	}
	return os.SameFile(info1, info2), nil
}

// Relative returns a relative path from base to target.
func Relative(target, base string) (string, error) {
	return filepath.Rel(base, target)
}

// Proximate returns a relative path if possible, otherwise returns target unchanged.
func Proximate(target, base string) (string, error) {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		// If cannot make relative (e.g., different drives on Windows), return target
		return target, nil
	}
	return rel, nil
}

// LexicallyNormal returns a normalized path (clean . and .. components).
func LexicallyNormal(path string) string {
	return filepath.Clean(path)
}

// LexicallyRelative returns a lexical relative path (no filesystem access).
func LexicallyRelative(path, base string) (string, error) {
	// This is a simplified lexical relative; actual std::filesystem lexical relative
	// is more complex. We'll use filepath.Rel which requires existing paths.
	// For lexical only, we can implement a simple version.
	// For now, delegate to Relative which uses filesystem.
	return Relative(path, base)
}

// LexicallyProximate returns lexical proximate path.
func LexicallyProximate(path, base string) (string, error) {
	rel, err := LexicallyRelative(path, base)
	if err != nil {
		return path, nil
	}
	return rel, nil
}

// HardLinkCount returns the number of hard links to a file.
func HardLinkCount(path string) (uint64, error) {
	var stat syscall.Stat_t
	err := syscall.Stat(path, &stat)
	if err != nil {
		return 0, err
	}
	return uint64(stat.Nlink), nil
}

// SetCurrentPath changes the current working directory.
func SetCurrentPath(path string) error {
	return os.Chdir(path)
}

// WalkFunc is the type of the function called for each file or directory visited by Walk.
type WalkFunc func(path string, isDir bool) error

// Walk walks the file tree rooted at root, calling walkFn for each file or directory.
func Walk(root string, walkFn WalkFunc) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return walkFn(path, info.IsDir())
	})
}