package fs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateDirectory(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "a", "b", "c")
	if err := CreateDirectory(sub); err != nil {
		t.Fatalf("CreateDirectory failed: %v", err)
	}
	if !IsDirectory(sub) {
		t.Error("expected directory to exist")
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()
	if !Exists(dir) {
		t.Error("expected directory to exist")
	}
	if Exists(filepath.Join(dir, "nonexistent")) {
		t.Error("expected nonexistent path to not exist")
	}
}

func TestIsDirectory(t *testing.T) {
	dir := t.TempDir()
	if !IsDirectory(dir) {
		t.Error("expected path to be directory")
	}
	f := filepath.Join(dir, "file.txt")
	os.WriteFile(f, []byte("hello"), 0644)
	if IsDirectory(f) {
		t.Error("expected file to not be directory")
	}
}

func TestIsRegularFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file.txt")
	os.WriteFile(f, []byte("hello"), 0644)
	if !IsRegularFile(f) {
		t.Error("expected path to be regular file")
	}
	if IsRegularFile(dir) {
		t.Error("expected directory to not be regular file")
	}
}

func TestCurrentPath(t *testing.T) {
	p, err := CurrentPath()
	if err != nil {
		t.Fatalf("CurrentPath failed: %v", err)
	}
	if p == "" {
		t.Error("expected non-empty path")
	}
}

func TestTempDirectoryPath(t *testing.T) {
	p := TempDirectoryPath()
	if p == "" {
		t.Error("expected non-empty temp directory")
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)

	names, err := List(dir, false)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(names) != 3 {
		t.Errorf("expected 3 entries, got %d: %v", len(names), names)
	}
}

func TestListRecursive(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("b"), 0644)

	names, err := List(dir, true)
	if err != nil {
		t.Fatalf("List recursive failed: %v", err)
	}
	// List includes both files and directories; root dir is excluded by Rel filter
	if len(names) != 3 {
		t.Errorf("expected 3 entries, got %d: %v", len(names), names)
	}
}

func TestListNonExistent(t *testing.T) {
	_, err := List("/nonexistent-path-12345", false)
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestFileSize(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file.txt")
	os.WriteFile(f, []byte("hello"), 0644)

	size, err := FileSize(f)
	if err != nil {
		t.Fatalf("FileSize failed: %v", err)
	}
	if size != 5 {
		t.Errorf("expected size 5, got %d", size)
	}

	_, err = FileSize(dir)
	if err == nil {
		t.Error("expected error for directory")
	}

	_, err = FileSize("/nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestRemove(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file.txt")
	os.WriteFile(f, []byte("hello"), 0644)

	removed, err := Remove(f)
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if !removed {
		t.Error("expected removed to be true")
	}
	if Exists(f) {
		t.Error("file should not exist after remove")
	}

	removed, err = Remove(f)
	if err != nil {
		t.Fatalf("Remove on nonexistent failed: %v", err)
	}
	if removed {
		t.Error("expected removed to be false for nonexistent file")
	}
}

func TestRemoveAll(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("b"), 0644)

	count, err := RemoveAll(dir)
	if err != nil {
		t.Fatalf("RemoveAll failed: %v", err)
	}
	if count == 0 {
		t.Error("expected count > 0")
	}
	if Exists(dir) {
		t.Error("directory should not exist after RemoveAll")
	}

	count, err = RemoveAll("/nonexistent-path")
	if err != nil {
		t.Fatalf("RemoveAll on nonexistent failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}
}

func TestRename(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "old.txt")
	dst := filepath.Join(dir, "new.txt")
	os.WriteFile(src, []byte("hello"), 0644)

	if err := Rename(src, dst); err != nil {
		t.Fatalf("Rename failed: %v", err)
	}
	if !Exists(dst) {
		t.Error("expected destination to exist")
	}
	if Exists(src) {
		t.Error("expected source to not exist")
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	os.WriteFile(src, []byte("hello world"), 0644)

	if err := Copy(src, dst, false, false, false, false, false, false); err != nil {
		t.Fatalf("Copy file failed: %v", err)
	}
	content, _ := os.ReadFile(dst)
	if string(content) != "hello world" {
		t.Errorf("expected 'hello world', got %q", content)
	}
}

func TestCopyFileToDirectory(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0755)
	os.WriteFile(src, []byte("content"), 0644)

	if err := Copy(src, sub, false, false, false, false, false, false); err != nil {
		t.Fatalf("Copy file to directory failed: %v", err)
	}
	dst := filepath.Join(sub, "src.txt")
	if !Exists(dst) {
		t.Error("expected file in subdirectory")
	}
}

func TestCopyFileSkipExisting(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	os.WriteFile(src, []byte("new"), 0644)
	os.WriteFile(dst, []byte("old"), 0644)

	if err := Copy(src, dst, false, false, true, false, false, false); err != nil {
		t.Fatalf("Copy with skip existing failed: %v", err)
	}
	content, _ := os.ReadFile(dst)
	if string(content) != "old" {
		t.Error("expected content to remain unchanged when skipping existing")
	}
}

func TestCopyFileOverwrite(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	os.WriteFile(src, []byte("new"), 0644)
	os.WriteFile(dst, []byte("old"), 0644)

	if err := Copy(src, dst, false, false, false, true, false, false); err != nil {
		t.Fatalf("Copy with overwrite failed: %v", err)
	}
	content, _ := os.ReadFile(dst)
	if string(content) != "new" {
		t.Errorf("expected 'new', got %q", content)
	}
}

func TestCopyFileErrorExisting(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	os.WriteFile(src, []byte("new"), 0644)
	os.WriteFile(dst, []byte("old"), 0644)

	err := Copy(src, dst, false, false, false, false, false, false)
	if err == nil {
		t.Fatal("expected error when destination exists without overwrite flag")
	}
}

func TestCopyDirectory(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	os.MkdirAll(filepath.Join(src, "sub"), 0755)
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("b"), 0644)

	if err := Copy(src, dst, true, false, false, false, false, false); err != nil {
		t.Fatalf("Copy directory failed: %v", err)
	}
	if !Exists(filepath.Join(dst, "a.txt")) {
		t.Error("expected a.txt in destination")
	}
	if !Exists(filepath.Join(dst, "sub", "b.txt")) {
		t.Error("expected b.txt in nested destination")
	}
}

func TestCopyDirectoryNoRecursive(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	os.MkdirAll(src, 0755)
	err := Copy(src, filepath.Join(dir, "dst"), false, false, false, false, false, false)
	if err == nil {
		t.Fatal("expected error when copying directory without recursive")
	}
}

func TestCopyDirectoriesOnly(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	os.MkdirAll(filepath.Join(src, "sub"), 0755)
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("a"), 0644)

	if err := Copy(src, dst, true, false, false, false, false, true); err != nil {
		t.Fatalf("Copy directories only failed: %v", err)
	}
	if !IsDirectory(dst) {
		t.Error("expected destination directory to exist")
	}
	if Exists(filepath.Join(dst, "a.txt")) {
		t.Error("expected a.txt to not be copied with directoriesOnly")
	}
	// Note: current implementation creates only the base destination directory
	// with directoriesOnly; it does not recurse into subdirectories
}

func TestPermissions(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file.txt")
	os.WriteFile(f, []byte("hello"), 0644)

	if err := Permissions(f, 0600); err != nil {
		t.Fatalf("Permissions failed: %v", err)
	}
	info, _ := os.Stat(f)
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected permissions 0600, got %o", info.Mode().Perm())
	}
}

func TestResizeFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file.txt")
	os.WriteFile(f, []byte("hello world"), 0644)

	if err := ResizeFile(f, 5); err != nil {
		t.Fatalf("ResizeFile failed: %v", err)
	}
	size, _ := FileSize(f)
	if size != 5 {
		t.Errorf("expected size 5, got %d", size)
	}
}

func TestLastWriteTime(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file.txt")
	os.WriteFile(f, []byte("hello"), 0644)

	tm, err := LastWriteTime(f)
	if err != nil {
		t.Fatalf("LastWriteTime failed: %v", err)
	}
	if tm.IsZero() {
		t.Error("expected non-zero time")
	}

	_, err = LastWriteTime("/nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestEquivalent(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file.txt")
	os.WriteFile(f, []byte("hello"), 0644)

	eq, err := Equivalent(f, f)
	if err != nil {
		t.Fatalf("Equivalent failed: %v", err)
	}
	if !eq {
		t.Error("expected file to be equivalent to itself")
	}

	_, err = Equivalent(f, "/nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestRelative(t *testing.T) {
	r, err := Relative("/a/b/c", "/a/b")
	if err != nil {
		t.Fatalf("Relative failed: %v", err)
	}
	if r != "c" {
		t.Errorf("expected 'c', got %q", r)
	}
}

func TestProximate(t *testing.T) {
	r, err := Proximate("/a/b/c", "/a/b")
	if err != nil {
		t.Fatalf("Proximate failed: %v", err)
	}
	if r != "c" {
		t.Errorf("expected 'c', got %q", r)
	}

	// When relative is not possible, should return target unchanged
	r, err = Proximate("/a", "/b")
	if err != nil {
		t.Fatalf("Proximate failed: %v", err)
	}
	// filepath.Rel may return errors on some systems, Proximate handles by returning target
	if r == "" {
		t.Error("expected non-empty result")
	}
}

func TestLexicallyNormal(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"/a/b/../c", "/a/c"},
		{"/a/b/./c", "/a/b/c"},
		{"/a//b/c", "/a/b/c"},
	}
	for _, tt := range tests {
		got := LexicallyNormal(tt.input)
		if got != tt.expected {
			t.Errorf("LexicallyNormal(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestLexicallyRelative(t *testing.T) {
	path := "/some/path/file.txt"
	base := "/some/path"
	r, err := LexicallyRelative(path, base)
	if err != nil {
		t.Fatalf("LexicallyRelative failed: %v", err)
	}
	if r == "" {
		t.Error("expected non-empty result")
	}
}

func TestLexicallyProximate(t *testing.T) {
	r, err := LexicallyProximate("/a/b/c", "/a/b")
	if err != nil {
		t.Fatalf("LexicallyProximate failed: %v", err)
	}
	if r != "c" {
		t.Errorf("expected 'c', got %q", r)
	}

	r, err = LexicallyProximate("/a", "/b")
	if err != nil {
		t.Fatalf("LexicallyProximate failed: %v", err)
	}
	if r == "" {
		t.Error("expected non-empty result")
	}
}

func TestSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	link := filepath.Join(dir, "link.txt")
	os.WriteFile(target, []byte("hello"), 0644)

	if err := CreateSymlink("target.txt", link); err != nil {
		t.Fatalf("CreateSymlink failed: %v", err)
	}

	read, err := ReadSymlink(link)
	if err != nil {
		t.Fatalf("ReadSymlink failed: %v", err)
	}
	if read != "target.txt" {
		t.Errorf("expected 'target.txt', got %q", read)
	}
}

func TestHardLink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	link := filepath.Join(dir, "hardlink.txt")
	os.WriteFile(target, []byte("hello"), 0644)

	if err := CreateHardLink(target, link); err != nil {
		t.Fatalf("CreateHardLink failed: %v", err)
	}
	if !Exists(link) {
		t.Error("expected hardlink to exist")
	}
}

func TestHardLinkCount(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file.txt")
	os.WriteFile(f, []byte("hello"), 0644)

	count, err := HardLinkCount(f)
	if err != nil {
		t.Fatalf("HardLinkCount failed: %v", err)
	}
	if count < 1 {
		t.Errorf("expected at least 1 hard link, got %d", count)
	}
}

func TestSetCurrentPath(t *testing.T) {
	orig, _ := CurrentPath()
	defer SetCurrentPath(orig)

	dir := t.TempDir()
	if err := SetCurrentPath(dir); err != nil {
		t.Fatalf("SetCurrentPath failed: %v", err)
	}
	p, _ := CurrentPath()
	// On some platforms the resolved path may differ, just check it changed
	if p == orig {
		t.Error("expected working directory to change")
	}
}

func TestWalk(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("b"), 0644)

	var visited []string
	err := Walk(dir, func(path string, isDir bool) error {
		visited = append(visited, path)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}
	if len(visited) != 4 {
		t.Errorf("expected 4 visits (root, a.txt, sub, sub/b.txt), got %d: %v", len(visited), visited)
	}
}

func TestWalkError(t *testing.T) {
	err := Walk("/nonexistent-path-12345", func(path string, isDir bool) error {
		return nil
	})
	if err == nil {
		t.Error("expected error for nonexistent root")
	}
}

func TestStatus(t *testing.T) {
	dir := t.TempDir()

	// Regular file
	f := filepath.Join(dir, "file.txt")
	os.WriteFile(f, []byte("hello"), 0644)
	s, err := Status(f)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if s.Type != FileTypeRegular {
		t.Errorf("expected FileTypeRegular, got %v", s.Type)
	}
	if s.Permissions == 0 {
		t.Error("expected non-zero permissions")
	}

	// Directory
	s, err = Status(dir)
	if err != nil {
		t.Fatalf("Status failed for dir: %v", err)
	}
	if s.Type != FileTypeDirectory {
		t.Errorf("expected FileTypeDirectory, got %v", s.Type)
	}

	// Nonexistent
	s, err = Status("/nonexistent")
	if err != nil {
		t.Fatalf("Status on nonexistent failed: %v", err)
	}
	if s.Type != FileTypeNotFound {
		t.Errorf("expected FileTypeNotFound, got %v", s.Type)
	}
}

func TestSymlinkStatus(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	link := filepath.Join(dir, "link.txt")
	os.WriteFile(target, []byte("hello"), 0644)
	os.Symlink("target.txt", link)

	s, err := SymlinkStatus(link)
	if err != nil {
		t.Fatalf("SymlinkStatus failed: %v", err)
	}
	if s.Type != FileTypeSymlink {
		t.Errorf("expected FileTypeSymlink, got %v", s.Type)
	}
}

func TestSpace(t *testing.T) {
	info, err := Space("/")
	if err != nil {
		t.Fatalf("Space failed: %v", err)
	}
	if info.Capacity == 0 {
		t.Error("expected non-zero capacity")
	}
}

func TestCopySymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	link := filepath.Join(dir, "link.txt")
	dst := filepath.Join(dir, "link_copy.txt")
	os.WriteFile(target, []byte("hello"), 0644)
	os.Symlink("target.txt", link)

	// Copy symlink as symlink
	if err := Copy(link, dst, false, true, false, false, false, false); err != nil {
		t.Fatalf("Copy symlink failed: %v", err)
	}
	// The copy should be a symlink
	read, err := ReadSymlink(dst)
	if err != nil {
		t.Fatalf("ReadSymlink on copy failed: %v", err)
	}
	if read == "" {
		t.Error("expected non-empty symlink target")
	}
}

func TestCopyUpdateExisting(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	os.WriteFile(src, []byte("newer"), 0644)
	os.WriteFile(dst, []byte("older"), 0644)

	// Set src modification time in the past, dst in the future
	os.Chtimes(src, time.Now(), time.Now().Add(-time.Hour))
	os.Chtimes(dst, time.Now(), time.Now())

	// With updateExisting, should skip because src is older
	if err := Copy(src, dst, false, false, false, false, true, false); err != nil {
		t.Fatalf("Copy with updateExisting failed: %v", err)
	}
	content, _ := os.ReadFile(dst)
	if string(content) != "older" {
		t.Error("expected content to remain 'older' since src is older")
	}

	// Make src newer
	os.Chtimes(src, time.Now(), time.Now())
	os.Chtimes(dst, time.Now(), time.Now().Add(-time.Hour))

	if err := Copy(src, dst, false, false, false, false, true, false); err != nil {
		t.Fatalf("Copy with updateExisting (newer) failed: %v", err)
	}
	content, _ = os.ReadFile(dst)
	if string(content) != "newer" {
		t.Errorf("expected 'newer', got %q", content)
	}
}
