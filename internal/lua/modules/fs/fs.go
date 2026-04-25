// Package fs provides Lua bindings for filesystem operations following lua-stdfs API.
package fs

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
	exportsfs "github.com/bubblegutz/nklhd/internal/exports/fs"
)

// Register adds the fs module to the given Lua state.
// The module provides functions similar to C++ std::filesystem (lua-stdfs).
func Register(L *lua.LState) {
	mod := L.RegisterModule("fs", map[string]lua.LGFunction{
		// Basic filesystem API
		"mkdir":     mkdir,
		"cp":        cp,
		"rm":        rm,
		"mv":        mv,
		"cat":       cat,
		"write":     write,
		"append":    appendFunc,
		"ls":        ls,
		"exists":    exists,
		"isdir":     isdir,
		"isfile":    isfile,
		"abs":       abs,
		"join":      join,
		"dir":       dir,
		"base":      base,
		"ext":       ext,
		"walk":      walk,
		"readlines": readlines,
		// Extended fs API
		"copy":               copy,
		"create_dir":         createDir,
		"create_hardlink":    createHardlink,
		"create_symlink":     createSymlink,
		"equiv":              equiv,
		"file_size":          fileSize,
		"file_status":        fileStatus,
		"hardlink_count":     hardlinkCount,
		"permissions":        permissions,
		"proximate":          proximate,
		"read_symlink":       readSymlink,
		"relative":           relative,
		"remove_all":         removeAll,
		"rename":             rename,
		"resize":             resize,
		"space":              space,
		"symlink_status":     symlinkStatus,
		"temp_dir":           tempDir,
		"work_dir":           workDir,
		"write_time":         writeTime,
		"lexically_normal":   lexicallyNormal,
		"lexically_relative": lexicallyRelative,
		"lexically_proximate": lexicallyProximate,
	}).(*lua.LTable)

	// Constants
	fileType := L.NewTable()
	L.SetField(fileType, "none", lua.LNumber(exportsfs.FileTypeNone))
	L.SetField(fileType, "not_found", lua.LNumber(exportsfs.FileTypeNotFound))
	L.SetField(fileType, "regular", lua.LNumber(exportsfs.FileTypeRegular))
	L.SetField(fileType, "directory", lua.LNumber(exportsfs.FileTypeDirectory))
	L.SetField(fileType, "symlink", lua.LNumber(exportsfs.FileTypeSymlink))
	L.SetField(fileType, "block", lua.LNumber(exportsfs.FileTypeBlock))
	L.SetField(fileType, "character", lua.LNumber(exportsfs.FileTypeCharacter))
	L.SetField(fileType, "fifo", lua.LNumber(exportsfs.FileTypeFIFO))
	L.SetField(fileType, "socket", lua.LNumber(exportsfs.FileTypeSocket))
	L.SetField(fileType, "unknown", lua.LNumber(exportsfs.FileTypeUnknown))
	L.SetField(mod, "file_type", fileType)

	// file_perms constants (simplified)
	filePerms := L.NewTable()
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
	for name, value := range perms {
		L.SetField(filePerms, name, lua.LNumber(value))
	}
	L.SetField(mod, "file_perms", filePerms)

	// perm_options constants
	permOpts := L.NewTable()
	L.SetField(permOpts, "replace", lua.LString("replace"))
	L.SetField(permOpts, "add", lua.LString("add"))
	L.SetField(permOpts, "remove", lua.LString("remove"))
	L.SetField(permOpts, "nofollow", lua.LString("nofollow"))
	L.SetField(mod, "perm_options", permOpts)

	// copy_opts constants (bit flags)
	copyOpts := L.NewTable()
	L.SetField(copyOpts, "none", lua.LNumber(0))
	L.SetField(copyOpts, "skip_existing", lua.LNumber(1 << 0))
	L.SetField(copyOpts, "overwrite_existing", lua.LNumber(1 << 1))
	L.SetField(copyOpts, "update_existing", lua.LNumber(1 << 2))
	L.SetField(copyOpts, "recursive", lua.LNumber(1 << 3))
	L.SetField(copyOpts, "copy_symlinks", lua.LNumber(1 << 4))
	L.SetField(copyOpts, "skip_symlinks", lua.LNumber(1 << 5))
	L.SetField(copyOpts, "directories_only", lua.LNumber(1 << 6))
	L.SetField(copyOpts, "create_symlinks", lua.LNumber(1 << 7))
	L.SetField(copyOpts, "create_hard_links", lua.LNumber(1 << 8))
	L.SetField(mod, "copy_opts", copyOpts)

	L.SetGlobal("fs", mod)
}

func copy(L *lua.LState) int {
	src := L.CheckString(1)
	dst := L.CheckString(2)
	opts := L.OptTable(3, L.NewTable())

	recursive := false
	copySymlinks := false
	skipExisting := false
	overwriteExisting := false
	updateExisting := false
	directoriesOnly := false

	if v := opts.RawGetString("recursive"); lua.LVAsBool(v) {
		recursive = true
	}
	if v := opts.RawGetString("copy_symlinks"); lua.LVAsBool(v) {
		copySymlinks = true
	}
	if v := opts.RawGetString("skip_existing"); lua.LVAsBool(v) {
		skipExisting = true
	}
	if v := opts.RawGetString("overwrite_existing"); lua.LVAsBool(v) {
		overwriteExisting = true
	}
	if v := opts.RawGetString("update_existing"); lua.LVAsBool(v) {
		updateExisting = true
	}
	if v := opts.RawGetString("directories_only"); lua.LVAsBool(v) {
		directoriesOnly = true
	}

	err := exportsfs.Copy(src, dst, recursive, copySymlinks, skipExisting, overwriteExisting, updateExisting, directoriesOnly)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

func createDir(L *lua.LState) int {
	path := L.CheckString(1)
	err := exportsfs.CreateDirectory(path)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

func createHardlink(L *lua.LState) int {
	target := L.CheckString(1)
	link := L.CheckString(2)
	err := exportsfs.CreateHardLink(target, link)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

func createSymlink(L *lua.LState) int {
	target := L.CheckString(1)
	link := L.CheckString(2)
	// is_dir parameter ignored
	err := exportsfs.CreateSymlink(target, link)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

func exists(L *lua.LState) int {
	path := L.CheckString(1)
	ok := exportsfs.Exists(path)
	L.Push(lua.LBool(ok))
	return 1
}

func equiv(L *lua.LState) int {
	path1 := L.CheckString(1)
	path2 := L.CheckString(2)
	same, err := exportsfs.Equivalent(path1, path2)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LBool(same))
	L.Push(lua.LNil)
	return 2
}

func fileSize(L *lua.LState) int {
	path := L.CheckString(1)
	size, err := exportsfs.FileSize(path)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LNumber(size))
	L.Push(lua.LNil)
	return 2
}

func fileStatus(L *lua.LState) int {
	path := L.CheckString(1)
	stat, err := exportsfs.Status(path)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	tbl := L.NewTable()
	L.SetField(tbl, "type", lua.LNumber(stat.Type))
	L.SetField(tbl, "permissions", lua.LNumber(stat.Permissions))
	L.Push(tbl)
	L.Push(lua.LNil)
	return 2
}

func hardlinkCount(L *lua.LState) int {
	path := L.CheckString(1)
	count, err := exportsfs.HardLinkCount(path)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LNumber(count))
	L.Push(lua.LNil)
	return 2
}

func ls(L *lua.LState) int {
	path := L.CheckString(1)
	recursive := L.OptBool(2, false)
	entries, err := exportsfs.List(path, recursive)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	tbl := L.NewTable()
	for i, entry := range entries {
		L.RawSetInt(tbl, i+1, lua.LString(entry))
	}
	L.Push(tbl)
	L.Push(lua.LNil)
	return 2
}

func permissions(L *lua.LState) int {
	path := L.CheckString(1)
	perm := L.CheckInt(2)
	// opt parameter ignored
	err := exportsfs.Permissions(path, os.FileMode(perm))
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

func proximate(L *lua.LState) int {
	target := L.CheckString(1)
	base := L.CheckString(2)
	rel, err := exportsfs.Proximate(target, base)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LString(rel))
	L.Push(lua.LNil)
	return 2
}

func readSymlink(L *lua.LState) int {
	path := L.CheckString(1)
	target, err := exportsfs.ReadSymlink(path)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LString(target))
	L.Push(lua.LNil)
	return 2
}

func relative(L *lua.LState) int {
	target := L.CheckString(1)
	base := L.CheckString(2)
	rel, err := exportsfs.Relative(target, base)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LString(rel))
	L.Push(lua.LNil)
	return 2
}



func removeAll(L *lua.LState) int {
	path := L.CheckString(1)
	count, err := exportsfs.RemoveAll(path)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LNumber(count))
	L.Push(lua.LNil)
	return 2
}

func rename(L *lua.LState) int {
	src := L.CheckString(1)
	dst := L.CheckString(2)
	err := exportsfs.Rename(src, dst)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

func resize(L *lua.LState) int {
	path := L.CheckString(1)
	size := L.CheckInt64(2)
	err := exportsfs.ResizeFile(path, size)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

func space(L *lua.LState) int {
	path := L.CheckString(1)
	info, err := exportsfs.Space(path)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	tbl := L.NewTable()
	L.SetField(tbl, "capacity", lua.LNumber(info.Capacity))
	L.SetField(tbl, "free", lua.LNumber(info.Free))
	L.SetField(tbl, "available", lua.LNumber(info.Available))
	L.Push(tbl)
	L.Push(lua.LNil)
	return 2
}

func symlinkStatus(L *lua.LState) int {
	path := L.CheckString(1)
	stat, err := exportsfs.SymlinkStatus(path)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	tbl := L.NewTable()
	L.SetField(tbl, "type", lua.LNumber(stat.Type))
	L.SetField(tbl, "permissions", lua.LNumber(stat.Permissions))
	L.Push(tbl)
	L.Push(lua.LNil)
	return 2
}

func tempDir(L *lua.LState) int {
	path := exportsfs.TempDirectoryPath()
	L.Push(lua.LString(path))
	return 1
}

func workDir(L *lua.LState) int {
	n := L.GetTop()
	if n == 0 {
		// get current directory
		path, err := exportsfs.CurrentPath()
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(path))
		L.Push(lua.LNil)
		return 2
	}
	// set current directory
	path := L.CheckString(1)
	err := exportsfs.SetCurrentPath(path)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

func writeTime(L *lua.LState) int {
	path := L.CheckString(1)
	t, err := exportsfs.LastWriteTime(path)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	// Return a table with time fields as per lua-stdfs
	tbl := L.NewTable()
	// GMT time
	gmt := t.UTC()
	L.SetField(tbl, "sec", lua.LNumber(gmt.Second()))
	L.SetField(tbl, "min", lua.LNumber(gmt.Minute()))
	L.SetField(tbl, "hour", lua.LNumber(gmt.Hour()))
	L.SetField(tbl, "mday", lua.LNumber(gmt.Day()))
	L.SetField(tbl, "mon", lua.LNumber(gmt.Month()))
	L.SetField(tbl, "year", lua.LNumber(gmt.Year()))
	L.SetField(tbl, "wday", lua.LNumber(int(gmt.Weekday())+1)) // Monday = 1
	L.SetField(tbl, "yday", lua.LNumber(gmt.YearDay()))
	L.SetField(tbl, "isdst", lua.LBool(false)) // UTC has no DST
	L.SetField(tbl, "since_epoch", lua.LNumber(t.Unix()))
	L.SetField(tbl, "ctm", lua.LString(t.Format(time.ANSIC)))
	// Local time
	loc := t.Local()
	locTbl := L.NewTable()
	L.SetField(locTbl, "sec", lua.LNumber(loc.Second()))
	L.SetField(locTbl, "min", lua.LNumber(loc.Minute()))
	L.SetField(locTbl, "hour", lua.LNumber(loc.Hour()))
	L.SetField(locTbl, "mday", lua.LNumber(loc.Day()))
	L.SetField(locTbl, "mon", lua.LNumber(loc.Month()))
	L.SetField(locTbl, "year", lua.LNumber(loc.Year()))
	L.SetField(locTbl, "wday", lua.LNumber(int(loc.Weekday())+1))
	L.SetField(locTbl, "yday", lua.LNumber(loc.YearDay()))
	L.SetField(tbl, "loc", locTbl)
	L.Push(tbl)
	L.Push(lua.LNil)
	return 2
}

func lexicallyNormal(L *lua.LState) int {
	path := L.CheckString(1)
	normal := exportsfs.LexicallyNormal(path)
	L.Push(lua.LString(normal))
	return 1
}

func lexicallyRelative(L *lua.LState) int {
	path := L.CheckString(1)
	base := L.CheckString(2)
	rel, err := exportsfs.LexicallyRelative(path, base)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LString(rel))
	L.Push(lua.LNil)
	return 2
}

func lexicallyProximate(L *lua.LState) int {
	path := L.CheckString(1)
	base := L.CheckString(2)
	rel, err := exportsfs.LexicallyProximate(path, base)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LString(rel))
	L.Push(lua.LNil)
	return 2
}

// Basic filesystem functions
func mkdir(L *lua.LState) int {
	path := L.CheckString(1)
	_ = L.OptBool(2, false) // parents (ignored)
	_ = L.OptInt(3, 0755)   // perm (ignored)
	// Note: parents parameter ignored, always creates parent directories
	err := exportsfs.CreateDirectory(path)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

func cp(L *lua.LState) int {
	src := L.CheckString(1)
	dst := L.CheckString(2)
	recursive := L.OptBool(3, false)
	// Note: fs.cp expects recursive flag; our copy uses multiple flags
	// Use recursive flag, copy_symlinks=false, skip_existing=false, etc.
	err := exportsfs.Copy(src, dst, recursive, false, false, false, false, false)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

func rm(L *lua.LState) int {
	path := L.CheckString(1)
	recursive := L.OptBool(2, false)
	if recursive {
		_, err := exportsfs.RemoveAll(path)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
	} else {
		_, err := exportsfs.Remove(path)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
	}
	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

func mv(L *lua.LState) int {
	src := L.CheckString(1)
	dst := L.CheckString(2)
	err := exportsfs.Rename(src, dst)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

func cat(L *lua.LState) int {
	path := L.CheckString(1)
	// Read file using os.ReadFile directly
	data, err := os.ReadFile(path)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LString(string(data)))
	L.Push(lua.LNil)
	return 2
}

func write(L *lua.LState) int {
	path := L.CheckString(1)
	content := L.CheckString(2)
	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

func appendFunc(L *lua.LState) int {
	path := L.CheckString(1)
	content := L.CheckString(2)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	defer f.Close()
	_, err = f.WriteString(content)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

func isdir(L *lua.LState) int {
	path := L.CheckString(1)
	ok := exportsfs.IsDirectory(path)
	L.Push(lua.LBool(ok))
	return 1
}

func isfile(L *lua.LState) int {
	path := L.CheckString(1)
	ok := exportsfs.IsRegularFile(path)
	L.Push(lua.LBool(ok))
	return 1
}

func abs(L *lua.LState) int {
	path := L.CheckString(1)
	absPath, err := filepath.Abs(path)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LString(absPath))
	L.Push(lua.LNil)
	return 2
}

func join(L *lua.LState) int {
	n := L.GetTop()
	elems := make([]string, n)
	for i := 1; i <= n; i++ {
		elems[i-1] = L.CheckString(i)
	}
	result := filepath.Join(elems...)
	L.Push(lua.LString(result))
	return 1
}

func dir(L *lua.LState) int {
	path := L.CheckString(1)
	result := filepath.Dir(path)
	L.Push(lua.LString(result))
	return 1
}

func base(L *lua.LState) int {
	path := L.CheckString(1)
	result := filepath.Base(path)
	L.Push(lua.LString(result))
	return 1
}

func ext(L *lua.LState) int {
	path := L.CheckString(1)
	result := filepath.Ext(path)
	L.Push(lua.LString(result))
	return 1
}

func walk(L *lua.LState) int {
	root := L.CheckString(1)
	fn := L.CheckFunction(2)
	err := exportsfs.Walk(root, func(path string, isDir bool) error {
		L.Push(fn)
		L.Push(lua.LString(path))
		L.Push(lua.LBool(isDir))
		if err := L.PCall(2, 0, nil); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

func readlines(L *lua.LState) int {
	path := L.CheckString(1)
	data, err := os.ReadFile(path)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	lines := strings.Split(string(data), "\n")
	// Remove trailing empty line if file ends with newline
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	tbl := L.NewTable()
	for i, line := range lines {
		L.RawSetInt(tbl, i+1, lua.LString(line))
	}
	L.Push(tbl)
	L.Push(lua.LNil)
	return 2
}