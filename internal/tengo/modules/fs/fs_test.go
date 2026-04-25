package fs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d5/tengo/v2"
)

func TestFsMkdir(t *testing.T) {
	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "testdir")
	fn := Module["mkdir"].(*tengo.UserFunction)
	obj, err := fn.Value(&tengo.String{Value: testDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := obj.(*tengo.Bool); !ok || obj == tengo.FalseValue {
		t.Fatalf("expected true, got %v", obj)
	}
	if _, err := os.Stat(testDir); os.IsNotExist(err) {
		t.Error("directory was not created")
	}
}

func TestFsExists(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("hello"), 0644)

	fn := Module["exists"].(*tengo.UserFunction)
	obj, err := fn.Value(&tengo.String{Value: testFile})
	if err != nil {
		t.Fatal(err)
	}
	if obj == tengo.FalseValue {
		t.Error("expected true for existing file")
	}

	obj, err = fn.Value(&tengo.String{Value: filepath.Join(tmpDir, "nonexistent.txt")})
	if err != nil {
		t.Fatal(err)
	}
	if obj != tengo.FalseValue {
		t.Error("expected false for non-existing file")
	}
}

func TestFsWriteCat(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	writeFn := Module["write"].(*tengo.UserFunction)
	catFn := Module["cat"].(*tengo.UserFunction)

	content := "hello world"
	_, err := writeFn.Value(&tengo.String{Value: testFile}, &tengo.String{Value: content})
	if err != nil {
		t.Fatal(err)
	}

	obj, err := catFn.Value(&tengo.String{Value: testFile})
	if err != nil {
		t.Fatal(err)
	}
	s, ok := obj.(*tengo.String)
	if !ok {
		t.Fatalf("expected string, got %T", obj)
	}
	if s.Value != content {
		t.Errorf("expected %q, got %q", content, s.Value)
	}
}

func TestFsIsdirIsfile(t *testing.T) {
	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "dir")
	testFile := filepath.Join(tmpDir, "file.txt")
	os.MkdirAll(testDir, 0755)
	os.WriteFile(testFile, []byte("data"), 0644)

	isdirFn := Module["isdir"].(*tengo.UserFunction)
	isfileFn := Module["isfile"].(*tengo.UserFunction)

	obj, err := isdirFn.Value(&tengo.String{Value: testDir})
	if err != nil {
		t.Fatal(err)
	}
	if obj == tengo.FalseValue {
		t.Error("expected true for directory")
	}

	obj, err = isfileFn.Value(&tengo.String{Value: testFile})
	if err != nil {
		t.Fatal(err)
	}
	if obj == tengo.FalseValue {
		t.Error("expected true for file")
	}

	obj, err = isdirFn.Value(&tengo.String{Value: testFile})
	if err != nil {
		t.Fatal(err)
	}
	if obj != tengo.FalseValue {
		t.Error("expected false for file isdir")
	}
}

func TestFsLs(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "b.txt"), []byte("b"), 0644)

	fn := Module["ls"].(*tengo.UserFunction)
	obj, err := fn.Value(&tengo.String{Value: tmpDir})
	if err != nil {
		t.Fatal(err)
	}
	if errObj, ok := obj.(*tengo.Error); ok {
		t.Fatalf("ls error: %s", errObj.Value.String())
	}
	arr, ok := obj.(*tengo.Array)
	if !ok {
		t.Fatalf("expected array, got %T", obj)
	}
	found := 0
	for _, item := range arr.Value {
		if s, ok := item.(*tengo.String); ok {
			if strings.HasSuffix(s.Value, ".txt") {
				found++
			}
		}
	}
	if found < 2 {
		t.Errorf("expected at least 2 .txt files, found %d in %v", found, arr.String())
	}
}

func TestFsJoin(t *testing.T) {
	fn := Module["join"].(*tengo.UserFunction)
	obj, err := fn.Value(&tengo.String{Value: "/a"}, &tengo.String{Value: "b"}, &tengo.String{Value: "c.txt"})
	if err != nil {
		t.Fatal(err)
	}
	s, ok := obj.(*tengo.String)
	if !ok {
		t.Fatalf("expected string, got %T", obj)
	}
	expected := filepath.Join("/a", "b", "c.txt")
	if s.Value != expected {
		t.Errorf("expected %q, got %q", expected, s.Value)
	}
}

func TestFsTempDir(t *testing.T) {
	fn := Module["temp_dir"].(*tengo.UserFunction)
	obj, err := fn.Value()
	if err != nil {
		t.Fatal(err)
	}
	s, ok := obj.(*tengo.String)
	if !ok {
		t.Fatalf("expected string, got %T", obj)
	}
	if s.Value == "" {
		t.Error("temp dir should not be empty")
	}
}

func TestFsConstants(t *testing.T) {
	ft, ok := Module["file_type"].(*tengo.ImmutableMap)
	if !ok {
		t.Fatal("file_type not found or not ImmutableMap")
	}
	for _, name := range []string{"regular", "directory", "symlink", "not_found"} {
		if _, ok := ft.Value[name]; !ok {
			t.Errorf("missing file_type constant: %s", name)
		}
	}

	fp, ok := Module["file_perms"].(*tengo.ImmutableMap)
	if !ok {
		t.Fatal("file_perms not found or not ImmutableMap")
	}
	for _, name := range []string{"owner_read", "owner_write", "owner_exec", "all"} {
		if _, ok := fp.Value[name]; !ok {
			t.Errorf("missing file_perms constant: %s", name)
		}
	}
}

func TestFsRm(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "toremove.txt")
	os.WriteFile(testFile, []byte("data"), 0644)

	rmFn := Module["rm"].(*tengo.UserFunction)
	obj, err := rmFn.Value(&tengo.String{Value: testFile})
	if err != nil {
		t.Fatal(err)
	}
	if errObj, ok := obj.(*tengo.Error); ok {
		t.Fatalf("rm error: %s", errObj.Value.String())
	}
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("file should not exist after rm")
	}
}

func TestFsMissingArgs(t *testing.T) {
	for _, name := range []string{"mkdir", "rm", "exists", "isdir", "isfile", "cat", "abs", "dir", "base", "ext"} {
		fn, ok := Module[name].(*tengo.UserFunction)
		if !ok {
			t.Fatalf("%s not a UserFunction", name)
		}
		_, err := fn.Value()
		if err == nil {
			t.Errorf("%s: expected error for no args", name)
		}
	}
}
