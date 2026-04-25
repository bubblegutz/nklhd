package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRootScript(t *testing.T) {
	// Backup original environment
	origConfigHome := os.Getenv("XDG_CONFIG_HOME")
	origConfigDirs := os.Getenv("XDG_CONFIG_DIRS")
	defer func() {
		_ = os.Setenv("XDG_CONFIG_HOME", origConfigHome)
		_ = os.Setenv("XDG_CONFIG_DIRS", origConfigDirs)
	}()

	// Set up a temporary XDG config home
	tmpDir := t.TempDir()
	_ = os.Setenv("XDG_CONFIG_HOME", tmpDir)
	_ = os.Setenv("XDG_CONFIG_DIRS", filepath.Join(tmpDir, "xdg"))

	// Test 1: empty path -> default config file
	path := ResolveRootScript("")
	if path == "" {
		t.Fatal("empty path should return default config path")
	}
	expected := filepath.Join(tmpDir, "nklhd", "config.toml")
	if path != expected {
		t.Errorf("default config path mismatch: got %q, expected %q", path, expected)
	}

	// Test 2: absolute path
	absPath := "/etc/nklhd/config.toml"
	path = ResolveRootScript(absPath)
	if path != absPath {
		t.Errorf("absolute path should be returned unchanged: got %q", path)
	}

	// Test 3: relative path with separator
	relPath := "./config.toml"
	path = ResolveRootScript(relPath)
	if path != relPath {
		t.Errorf("relative path with separator should be returned unchanged: got %q", path)
	}

	// Test 4: basename without separator -> search in XDG config home
	basename := "myconfig.lua"
	path = ResolveRootScript(basename)
	expected = filepath.Join(tmpDir, "nklhd", basename)
	if path != expected {
		t.Errorf("basename should be resolved to XDG config home: got %q, expected %q", path, expected)
	}

	// Test 5: basename with dot but no separator (still basename)
	basename2 := "config.test.lua"
	path = ResolveRootScript(basename2)
	expected = filepath.Join(tmpDir, "nklhd", basename2)
	if path != expected {
		t.Errorf("basename with dots should be resolved to XDG config home: got %q, expected %q", path, expected)
	}

	// Test 6: path with backslash (Windows style)
	winPath := "subdir\\config.lua"
	path = ResolveRootScript(winPath)
	if !strings.Contains(winPath, "\\") {
		t.Skip("test environment doesn't have backslash path separator")
	}
	if path != winPath {
		t.Errorf("path with backslash should be returned unchanged: got %q", path)
	}
}

func TestResolveMountPoint(t *testing.T) {
	// Backup original environment
	origRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	defer func() {
		_ = os.Setenv("XDG_RUNTIME_DIR", origRuntimeDir)
	}()

	// Test 1: user-provided mount point
	mount := "/tmp/mymount"
	path := ResolveMountPoint(mount)
	if path != mount {
		t.Errorf("user-provided mount point should be returned unchanged: got %q", path)
	}

	// Test 2: empty mount point, XDG_RUNTIME_DIR set
	tmpDir := t.TempDir()
	_ = os.Setenv("XDG_RUNTIME_DIR", tmpDir)
	path = ResolveMountPoint("")
	expected := filepath.Join(tmpDir, "nklhd")
	if path != expected {
		t.Errorf("empty mount point should default to XDG_RUNTIME_DIR/nklhd: got %q, expected %q", path, expected)
	}

	// Test 3: empty mount point, XDG_RUNTIME_DIR unset (fallback)
	_ = os.Unsetenv("XDG_RUNTIME_DIR")
	// RuntimeDir should provide a fallback
	if RuntimeDir() == "" {
		t.Fatal("RuntimeDir should not be empty even when XDG_RUNTIME_DIR is unset")
	}
	path = ResolveMountPoint("")
	expected = filepath.Join(RuntimeDir(), "nklhd")
	if path != expected {
		t.Errorf("empty mount point should use RuntimeDir fallback: got %q, expected %q", path, expected)
	}
}

func TestResolveRootScriptEnvFallback(t *testing.T) {
	// Test that XDG_CONFIG_HOME environment variable is respected
	tmpDir := t.TempDir()
	_ = os.Setenv("XDG_CONFIG_HOME", tmpDir)

	path := ResolveRootScript("")
	expected := filepath.Join(tmpDir, "nklhd", "config.toml")
	if path != expected {
		t.Errorf("XDG_CONFIG_HOME not respected: got %q, expected %q", path, expected)
	}
}