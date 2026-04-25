// Package fs provides Tengo bindings for filesystem operations.
package fs

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	exportsfs "github.com/bubblegutz/nklhd/internal/exports/fs"
	"github.com/d5/tengo/v2"
)

// Module is the Tengo module map for fs.
var Module = map[string]tengo.Object{
	// Basic filesystem API
	"mkdir":     tFunc("mkdir", fsMkdir),
	"cp":        tFunc("cp", fsCp),
	"rm":        tFunc("rm", fsRm),
	"mv":        tFunc("mv", fsMv),
	"cat":       tFunc("cat", fsCat),
	"write":     tFunc("write", fsWrite),
	"append":    tFunc("append", fsAppend),
	"ls":        tFunc("ls", fsLs),
	"exists":    tFunc("exists", fsExists),
	"isdir":     tFunc("isdir", fsIsdir),
	"isfile":    tFunc("isfile", fsIsfile),
	"abs":       tFunc("abs", fsAbs),
	"join":      tFunc("join", fsJoin),
	"dir":       tFunc("dir", fsDir),
	"base":      tFunc("base", fsBase),
	"ext":       tFunc("ext", fsExt),
	"walk":      tFunc("walk", fsWalk),
	"readlines": tFunc("readlines", fsReadlines),
	// Extended fs API
	"file_size":   tFunc("file_size", fsFileSize),
	"file_status": tFunc("file_status", fsFileStatus),
	"remove_all":  tFunc("remove_all", fsRemoveAll),
	"rename":      tFunc("rename", fsRename),
	"resize":      tFunc("resize", fsResize),
	"space":       tFunc("space", fsSpace),
	"temp_dir":    tFunc("temp_dir", fsTempDir),
	"work_dir":    tFunc("work_dir", fsWorkDir),
	// Advanced fs API
	"copy":               tFunc("copy", fsCopyWithOpts),
	"create_dir":         tFunc("create_dir", fsCreateDir),
	"create_hardlink":    tFunc("create_hardlink", fsCreateHardlink),
	"create_symlink":     tFunc("create_symlink", fsCreateSymlink),
	"equiv":              tFunc("equiv", fsEquiv),
	"hardlink_count":     tFunc("hardlink_count", fsHardlinkCount),
	"permissions":        tFunc("permissions", fsPermissions),
	"proximate":          tFunc("proximate", fsProximate),
	"read_symlink":       tFunc("read_symlink", fsReadSymlink),
	"relative":           tFunc("relative", fsRelative),
	"symlink_status":     tFunc("symlink_status", fsSymlinkStatus),
	"write_time":         tFunc("write_time", fsWriteTime),
	"lexically_normal":   tFunc("lexically_normal", fsLexicallyNormal),
	"lexically_relative": tFunc("lexically_relative", fsLexicallyRelative),
	"lexically_proximate": tFunc("lexically_proximate", fsLexicallyProximate),
	// Constants (lazy init)
	"file_type":   &tengo.ImmutableMap{Value: fileTypeConstants()},
	"file_perms":  &tengo.ImmutableMap{Value: filePermConstants()},
	"perm_options": &tengo.ImmutableMap{Value: permOptionsConstants()},
	"copy_opts":   &tengo.ImmutableMap{Value: copyOptsConstants()},
}

// tFunc creates a named UserFunction.
func tFunc(name string, fn func(args ...tengo.Object) (tengo.Object, error)) *tengo.UserFunction {
	return &tengo.UserFunction{Name: name, Value: fn}
}

func checkStringArg(args []tengo.Object, idx int, name string) (string, error) {
	if len(args) <= idx {
		return "", tengo.ErrWrongNumArguments
	}
	s, ok := args[idx].(*tengo.String)
	if !ok {
		return "", tengo.ErrInvalidArgumentType{Name: name, Expected: "string", Found: args[idx].TypeName()}
	}
	return s.Value, nil
}

func checkBoolArg(args []tengo.Object, idx int, name string) (bool, error) {
	if len(args) <= idx {
		return false, nil
	}
	b, ok := args[idx].(*tengo.Bool)
	if !ok {
		return false, tengo.ErrInvalidArgumentType{Name: name, Expected: "bool", Found: args[idx].TypeName()}
	}
	return !b.IsFalsy(), nil
}

func fsMkdir(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	if err := exportsfs.CreateDirectory(path); err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	return tengo.TrueValue, nil
}

func fsCp(args ...tengo.Object) (tengo.Object, error) {
	src, err := checkStringArg(args, 0, "src")
	if err != nil {
		return nil, err
	}
	dst, err := checkStringArg(args, 1, "dst")
	if err != nil {
		return nil, err
	}
	recursive, _ := checkBoolArg(args, 2, "recursive")
	if err := exportsfs.Copy(src, dst, recursive, false, false, false, false, false); err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	return tengo.TrueValue, nil
}

func fsRm(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	recursive, _ := checkBoolArg(args, 1, "recursive")
	if recursive {
		if _, err := exportsfs.RemoveAll(path); err != nil {
			return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
		}
	} else {
		if _, err := exportsfs.Remove(path); err != nil {
			return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
		}
	}
	return tengo.TrueValue, nil
}

func fsMv(args ...tengo.Object) (tengo.Object, error) {
	src, err := checkStringArg(args, 0, "src")
	if err != nil {
		return nil, err
	}
	dst, err := checkStringArg(args, 1, "dst")
	if err != nil {
		return nil, err
	}
	if err := exportsfs.Rename(src, dst); err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	return tengo.TrueValue, nil
}

func fsCat(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	return &tengo.String{Value: string(data)}, nil
}

func fsWrite(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	content, err := checkStringArg(args, 1, "content")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	return tengo.TrueValue, nil
}

func fsAppend(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	content, err := checkStringArg(args, 1, "content")
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	return tengo.TrueValue, nil
}

func fsLs(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	recursive, _ := checkBoolArg(args, 1, "recursive")
	entries, err := exportsfs.List(path, recursive)
	if err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	objs := make([]tengo.Object, len(entries))
	for i, entry := range entries {
		objs[i] = &tengo.String{Value: entry}
	}
	return &tengo.Array{Value: objs}, nil
}

func fsExists(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	if exportsfs.Exists(path) {
		return tengo.TrueValue, nil
	}
	return tengo.FalseValue, nil
}

func fsIsdir(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	if exportsfs.IsDirectory(path) {
		return tengo.TrueValue, nil
	}
	return tengo.FalseValue, nil
}

func fsIsfile(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	if exportsfs.IsRegularFile(path) {
		return tengo.TrueValue, nil
	}
	return tengo.FalseValue, nil
}

func fsAbs(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	return &tengo.String{Value: absPath}, nil
}

func fsJoin(args ...tengo.Object) (tengo.Object, error) {
	if len(args) < 1 {
		return nil, tengo.ErrWrongNumArguments
	}
	elems := make([]string, len(args))
	for i, arg := range args {
		s, ok := arg.(*tengo.String)
		if !ok {
			return nil, tengo.ErrInvalidArgumentType{Name: "path", Expected: "string", Found: arg.TypeName()}
		}
		elems[i] = s.Value
	}
	return &tengo.String{Value: filepath.Join(elems...)}, nil
}

func fsDir(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	return &tengo.String{Value: filepath.Dir(path)}, nil
}

func fsBase(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	return &tengo.String{Value: filepath.Base(path)}, nil
}

func fsExt(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	return &tengo.String{Value: filepath.Ext(path)}, nil
}

func fsWalk(args ...tengo.Object) (tengo.Object, error) {
	root, err := checkStringArg(args, 0, "root")
	if err != nil {
		return nil, err
	}

	// Collect all entries by walking the tree
	var entries []string
	walkErr := exportsfs.Walk(root, func(path string, isDir bool) error {
		entries = append(entries, path)
		return nil
	})
	if walkErr != nil {
		return &tengo.Error{Value: &tengo.String{Value: walkErr.Error()}}, nil
	}

	// Call the user function for each entry by building a simple wrapper
	objs := make([]tengo.Object, len(entries))
	for i, entry := range entries {
		objs[i] = &tengo.String{Value: entry}
	}
	return &tengo.Array{Value: objs}, nil
}

func fsReadlines(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	objs := make([]tengo.Object, len(lines))
	for i, line := range lines {
		objs[i] = &tengo.String{Value: line}
	}
	return &tengo.Array{Value: objs}, nil
}

func fsFileSize(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	size, err := exportsfs.FileSize(path)
	if err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	return &tengo.Int{Value: size}, nil
}

func fsFileStatus(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	stat, err := exportsfs.Status(path)
	if err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	return &tengo.ImmutableMap{Value: map[string]tengo.Object{
		"type":        &tengo.Int{Value: int64(stat.Type)},
		"permissions": &tengo.Int{Value: int64(stat.Permissions)},
	}}, nil
}

func fsRemoveAll(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	count, err := exportsfs.RemoveAll(path)
	if err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	return &tengo.Int{Value: int64(count)}, nil
}

func fsRename(args ...tengo.Object) (tengo.Object, error) {
	src, err := checkStringArg(args, 0, "src")
	if err != nil {
		return nil, err
	}
	dst, err := checkStringArg(args, 1, "dst")
	if err != nil {
		return nil, err
	}
	if err := exportsfs.Rename(src, dst); err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	return tengo.TrueValue, nil
}

func fsResize(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	if len(args) < 2 {
		return nil, tengo.ErrWrongNumArguments
	}
	size, ok := args[1].(*tengo.Int)
	if !ok {
		return nil, tengo.ErrInvalidArgumentType{Name: "size", Expected: "int", Found: args[1].TypeName()}
	}
	if err := exportsfs.ResizeFile(path, size.Value); err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	return tengo.TrueValue, nil
}

func fsSpace(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	info, err := exportsfs.Space(path)
	if err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	return &tengo.ImmutableMap{Value: map[string]tengo.Object{
		"capacity":  &tengo.Int{Value: int64(info.Capacity)},
		"free":      &tengo.Int{Value: int64(info.Free)},
		"available": &tengo.Int{Value: int64(info.Available)},
	}}, nil
}

func fsTempDir(args ...tengo.Object) (tengo.Object, error) {
	path := exportsfs.TempDirectoryPath()
	return &tengo.String{Value: path}, nil
}

func fsWorkDir(args ...tengo.Object) (tengo.Object, error) {
	if len(args) == 0 {
		path, err := exportsfs.CurrentPath()
		if err != nil {
			return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
		}
		return &tengo.String{Value: path}, nil
	}
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	if err := exportsfs.SetCurrentPath(path); err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	return tengo.TrueValue, nil
}

func fileTypeConstants() map[string]tengo.Object {
	return map[string]tengo.Object{
		"none":      &tengo.Int{Value: int64(exportsfs.FileTypeNone)},
		"not_found": &tengo.Int{Value: int64(exportsfs.FileTypeNotFound)},
		"regular":   &tengo.Int{Value: int64(exportsfs.FileTypeRegular)},
		"directory": &tengo.Int{Value: int64(exportsfs.FileTypeDirectory)},
		"symlink":   &tengo.Int{Value: int64(exportsfs.FileTypeSymlink)},
		"block":     &tengo.Int{Value: int64(exportsfs.FileTypeBlock)},
		"character": &tengo.Int{Value: int64(exportsfs.FileTypeCharacter)},
		"fifo":      &tengo.Int{Value: int64(exportsfs.FileTypeFIFO)},
		"socket":    &tengo.Int{Value: int64(exportsfs.FileTypeSocket)},
		"unknown":   &tengo.Int{Value: int64(exportsfs.FileTypeUnknown)},
	}
}

func filePermConstants() map[string]tengo.Object {
	perms := map[string]os.FileMode{
		"owner_read":  0400,
		"owner_write": 0200,
		"owner_exec":  0100,
		"owner_all":   0700,
		"group_read":  0040,
		"group_write": 0020,
		"group_exec":  0010,
		"group_all":   0070,
		"others_read":  0004,
		"others_write": 0002,
		"others_exec":  0001,
		"others_all":   0007,
		"all":          0777,
		"set_uid":      04000,
		"set_gid":      02000,
		"sticky_bit":   01000,
	}
	m := make(map[string]tengo.Object, len(perms))
	for name, val := range perms {
		m[name] = &tengo.Int{Value: int64(val)}
	}
	return m
}

func permOptionsConstants() map[string]tengo.Object {
	return map[string]tengo.Object{
		"replace": &tengo.String{Value: "replace"},
		"add":     &tengo.String{Value: "add"},
		"remove":  &tengo.String{Value: "remove"},
		"nofollow": &tengo.String{Value: "nofollow"},
	}
}

func copyOptsConstants() map[string]tengo.Object {
	return map[string]tengo.Object{
		"none":               &tengo.Int{Value: 0},
		"skip_existing":      &tengo.Int{Value: 1 << 0},
		"overwrite_existing": &tengo.Int{Value: 1 << 1},
		"update_existing":    &tengo.Int{Value: 1 << 2},
		"recursive":          &tengo.Int{Value: 1 << 3},
		"copy_symlinks":      &tengo.Int{Value: 1 << 4},
		"skip_symlinks":      &tengo.Int{Value: 1 << 5},
		"directories_only":   &tengo.Int{Value: 1 << 6},
		"create_symlinks":    &tengo.Int{Value: 1 << 7},
		"create_hard_links":  &tengo.Int{Value: 1 << 8},
	}
}

func checkImmutableMapArg(args []tengo.Object, idx int, name string) (*tengo.ImmutableMap, error) {
	if len(args) <= idx {
		return &tengo.ImmutableMap{Value: make(map[string]tengo.Object)}, nil
	}
	if m, ok := args[idx].(*tengo.ImmutableMap); ok {
		return m, nil
	}
	if m, ok := args[idx].(*tengo.Map); ok {
		return &tengo.ImmutableMap{Value: m.Value}, nil
	}
	return nil, tengo.ErrInvalidArgumentType{Name: name, Expected: "map", Found: args[idx].TypeName()}
}

func fsCopyWithOpts(args ...tengo.Object) (tengo.Object, error) {
	src, err := checkStringArg(args, 0, "src")
	if err != nil {
		return nil, err
	}
	dst, err := checkStringArg(args, 1, "dst")
	if err != nil {
		return nil, err
	}
	opts, err := checkImmutableMapArg(args, 2, "opts")
	if err != nil {
		return nil, err
	}
	recursive := getMapBool(opts, "recursive")
	copySymlinks := getMapBool(opts, "copy_symlinks")
	skipExisting := getMapBool(opts, "skip_existing")
	overwriteExisting := getMapBool(opts, "overwrite_existing")
	updateExisting := getMapBool(opts, "update_existing")
	directoriesOnly := getMapBool(opts, "directories_only")
	if err := exportsfs.Copy(src, dst, recursive, copySymlinks, skipExisting, overwriteExisting, updateExisting, directoriesOnly); err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	return tengo.TrueValue, nil
}

func getMapBool(m *tengo.ImmutableMap, key string) bool {
	if v, ok := m.Value[key]; ok {
		if b, ok := v.(*tengo.Bool); ok {
			return !b.IsFalsy()
		}
	}
	return false
}

func fsCreateDir(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	if err := exportsfs.CreateDirectory(path); err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	return tengo.TrueValue, nil
}

func fsCreateHardlink(args ...tengo.Object) (tengo.Object, error) {
	target, err := checkStringArg(args, 0, "target")
	if err != nil {
		return nil, err
	}
	link, err := checkStringArg(args, 1, "link")
	if err != nil {
		return nil, err
	}
	if err := exportsfs.CreateHardLink(target, link); err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	return tengo.TrueValue, nil
}

func fsCreateSymlink(args ...tengo.Object) (tengo.Object, error) {
	target, err := checkStringArg(args, 0, "target")
	if err != nil {
		return nil, err
	}
	link, err := checkStringArg(args, 1, "link")
	if err != nil {
		return nil, err
	}
	if err := exportsfs.CreateSymlink(target, link); err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	return tengo.TrueValue, nil
}

func fsEquiv(args ...tengo.Object) (tengo.Object, error) {
	path1, err := checkStringArg(args, 0, "path1")
	if err != nil {
		return nil, err
	}
	path2, err := checkStringArg(args, 1, "path2")
	if err != nil {
		return nil, err
	}
	same, err := exportsfs.Equivalent(path1, path2)
	if err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	if same {
		return tengo.TrueValue, nil
	}
	return tengo.FalseValue, nil
}

func fsHardlinkCount(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	count, err := exportsfs.HardLinkCount(path)
	if err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	return &tengo.Int{Value: int64(count)}, nil
}

func fsPermissions(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	if len(args) < 2 {
		return nil, tengo.ErrWrongNumArguments
	}
	perm, ok := args[1].(*tengo.Int)
	if !ok {
		return nil, tengo.ErrInvalidArgumentType{Name: "perm", Expected: "int", Found: args[1].TypeName()}
	}
	if err := exportsfs.Permissions(path, os.FileMode(perm.Value)); err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	return tengo.TrueValue, nil
}

func fsProximate(args ...tengo.Object) (tengo.Object, error) {
	target, err := checkStringArg(args, 0, "target")
	if err != nil {
		return nil, err
	}
	base, err := checkStringArg(args, 1, "base")
	if err != nil {
		return nil, err
	}
	rel, err := exportsfs.Proximate(target, base)
	if err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	return &tengo.String{Value: rel}, nil
}

func fsReadSymlink(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	target, err := exportsfs.ReadSymlink(path)
	if err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	return &tengo.String{Value: target}, nil
}

func fsRelative(args ...tengo.Object) (tengo.Object, error) {
	target, err := checkStringArg(args, 0, "target")
	if err != nil {
		return nil, err
	}
	base, err := checkStringArg(args, 1, "base")
	if err != nil {
		return nil, err
	}
	rel, err := exportsfs.Relative(target, base)
	if err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	return &tengo.String{Value: rel}, nil
}

func fsSymlinkStatus(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	stat, err := exportsfs.SymlinkStatus(path)
	if err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	return &tengo.ImmutableMap{Value: map[string]tengo.Object{
		"type":        &tengo.Int{Value: int64(stat.Type)},
		"permissions": &tengo.Int{Value: int64(stat.Permissions)},
	}}, nil
}

func fsWriteTime(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	tm, err := exportsfs.LastWriteTime(path)
	if err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	gmt := tm.UTC()
	loc := tm.Local()
	return &tengo.ImmutableMap{Value: map[string]tengo.Object{
		"sec":         &tengo.Int{Value: int64(gmt.Second())},
		"min":         &tengo.Int{Value: int64(gmt.Minute())},
		"hour":        &tengo.Int{Value: int64(gmt.Hour())},
		"mday":        &tengo.Int{Value: int64(gmt.Day())},
		"mon":         &tengo.Int{Value: int64(gmt.Month())},
		"year":        &tengo.Int{Value: int64(gmt.Year())},
		"wday":        &tengo.Int{Value: int64(int(gmt.Weekday()) + 1)},
		"yday":        &tengo.Int{Value: int64(gmt.YearDay())},
		"isdst":       tengo.FalseValue,
		"since_epoch": &tengo.Int{Value: tm.Unix()},
		"ctm":         &tengo.String{Value: tm.Format(time.ANSIC)},
		"loc": &tengo.ImmutableMap{Value: map[string]tengo.Object{
			"sec":  &tengo.Int{Value: int64(loc.Second())},
			"min":  &tengo.Int{Value: int64(loc.Minute())},
			"hour": &tengo.Int{Value: int64(loc.Hour())},
			"mday": &tengo.Int{Value: int64(loc.Day())},
			"mon":  &tengo.Int{Value: int64(loc.Month())},
			"year": &tengo.Int{Value: int64(loc.Year())},
			"wday": &tengo.Int{Value: int64(int(loc.Weekday()) + 1)},
			"yday": &tengo.Int{Value: int64(loc.YearDay())},
		}},
	}}, nil
}

func fsLexicallyNormal(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	return &tengo.String{Value: exportsfs.LexicallyNormal(path)}, nil
}

func fsLexicallyRelative(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	base, err := checkStringArg(args, 1, "base")
	if err != nil {
		return nil, err
	}
	rel, err := exportsfs.LexicallyRelative(path, base)
	if err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	return &tengo.String{Value: rel}, nil
}

func fsLexicallyProximate(args ...tengo.Object) (tengo.Object, error) {
	path, err := checkStringArg(args, 0, "path")
	if err != nil {
		return nil, err
	}
	base, err := checkStringArg(args, 1, "base")
	if err != nil {
		return nil, err
	}
	rel, err := exportsfs.LexicallyProximate(path, base)
	if err != nil {
		return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
	}
	return &tengo.String{Value: rel}, nil
}
