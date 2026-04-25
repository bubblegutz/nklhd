// Package fuse provides a minimal FUSE implementation tailored for nklhd.
package fuse

import (
	"encoding/binary"
	"fmt"
	"syscall"
)

// Constants from fuse.h
const (
	FUSE_KERNEL_VERSION      = 7
	FUSE_KERNEL_MINOR_VERSION = 45 // Use FUSE v3 protocol (7.45)
	FUSE_ROOT_ID             = 1
	FUSE_MIN_READ_BUFFER     = 8192
	FUSE_OUT_HEADER_SIZE     = 16 // sizeof(fuse_out_header)
)

// Opcodes
const (
	FUSE_LOOKUP       = 1
	FUSE_GETATTR      = 3
	FUSE_SETATTR      = 4
	FUSE_OPEN         = 14
	FUSE_READ         = 15
	FUSE_WRITE        = 16
	FUSE_STATFS       = 17
	FUSE_RELEASE      = 18
	FUSE_FSYNC        = 20
	FUSE_INIT         = 26
	FUSE_OPENDIR      = 27
	FUSE_READDIR      = 28
	FUSE_RELEASEDIR   = 29
	FUSE_ACCESS       = 34
	FUSE_CREATE       = 35
)

// FUSE init flags (subset)
const (
	FUSE_ASYNC_READ       = 1 << 0
	FUSE_POSIX_LOCKS      = 1 << 1
	FUSE_ATOMIC_O_TRUNC   = 1 << 3
	FUSE_EXPORT_SUPPORT   = 1 << 4
	FUSE_BIG_WRITES       = 1 << 5
	FUSE_DONT_MASK        = 1 << 6
	FUSE_NO_OPEN_SUPPORT  = 1 << 17
	FUSE_PARALLEL_DIROPS  = 1 << 18
	FUSE_INIT_EXT         = 1 << 30
	FUSE_INIT_RESERVED    = 1 << 31
)

// Open flags
const (
	FOPEN_DIRECT_IO   = 1 << 0
	FOPEN_KEEP_CACHE  = 1 << 1
)

// Getattr flags
const (
	FUSE_GETATTR_FH = 1 << 0
)

// Write flags
const (
	FUSE_WRITE_CACHE = 1 << 0
)

// Entry out flags
const (
	FUSE_ENTRY_CACHE_TIMEOUT = 1  // placeholder
)

// Structs must match fuse.h layout (little-endian)

type fuseAttr struct {
	Ino         uint64
	Size        uint64
	Blocks      uint64
	Atime       uint64
	Mtime       uint64
	Ctime       uint64
	Atimensec   uint32
	Mtimensec   uint32
	Ctimensec   uint32
	Mode        uint32
	Nlink       uint32
	Uid         uint32
	Gid         uint32
	Rdev        uint32
	Blksize     uint32
	Flags       uint32
	Padding     uint32
}

type fuseEntryOut struct {
	Nodeid        uint64
	Generation    uint64
	EntryValid    uint64
	AttrValid     uint64
	EntryValidNSec uint32
	AttrValidNSec uint32
	Attr          fuseAttr
}



type fuseAttrOut struct {
	AttrValid     uint64
	AttrValidNSec uint32
	Dummy         uint32
	Attr          fuseAttr
}



type fuseOpenOut struct {
	Fh        uint64
	OpenFlags uint32
	Padding   uint32
}



type fuseReadIn struct {
	Fh         uint64
	Offset     uint64
	Size       uint32
	ReadFlags  uint32
	LockOwner  uint64
	Flags      uint32
	Padding    uint32
}

type fuseWriteIn struct {
	Fh         uint64
	Offset     uint64
	Size       uint32
	WriteFlags uint32
	LockOwner  uint64
	Flags      uint32
	Padding    uint32
}

type fuseWriteOut struct {
	Size     uint32
	Padding  uint32
}



type fuseInitOut struct {
	Major               uint32
	Minor               uint32
	MaxReadahead        uint32
	Flags               uint32
	MaxBackground       uint16
	CongestionThreshold uint16
	MaxWrite            uint32
	TimeGran            uint32
	MaxPages            uint16
	MapAlignment        uint16
	Flags2              uint32
	MaxStackDepth       uint32
	RequestTimeout      uint16
	Unused              [11]uint16
}

type fuseDirent struct { //nolint:unused
	Ino     uint64
	Off     uint64
	Namelen uint32
	Type    uint32
	// name follows
}
var _ = fuseDirent{}

const fuseDirentSize = 24 // size without name

// FUSE header structures
type fuseInHeader struct {
	Len          uint32
	Opcode       uint32
	Unique       uint64
	Nodeid       uint64
	Uid          uint32
	Gid          uint32
	Pid          uint32
	TotalExtlen  uint16
	Padding      uint16
}

type fuseOutHeader struct {
	Len    uint32
	Error  int32
	Unique uint64
}

// readFuseInHeader reads a fuse_in_header from data.
func readFuseInHeader(data []byte) (*fuseInHeader, error) {
	if len(data) < 40 {
		return nil, fmt.Errorf("header too short")
	}
	h := &fuseInHeader{}
	h.Len = binary.LittleEndian.Uint32(data[0:4])
	h.Opcode = binary.LittleEndian.Uint32(data[4:8])
	h.Unique = binary.LittleEndian.Uint64(data[8:16])
	h.Nodeid = binary.LittleEndian.Uint64(data[16:24])
	h.Uid = binary.LittleEndian.Uint32(data[24:28])
	h.Gid = binary.LittleEndian.Uint32(data[28:32])
	h.Pid = binary.LittleEndian.Uint32(data[32:36])
	h.TotalExtlen = binary.LittleEndian.Uint16(data[36:38])
	h.Padding = binary.LittleEndian.Uint16(data[38:40])
	return h, nil
}

// readFuseReadIn reads a fuse_read_in from data.
func readFuseReadIn(data []byte) (*fuseReadIn, error) {
	if len(data) < 40 {
		return nil, fmt.Errorf("read_in too short")
	}
	r := &fuseReadIn{}
	r.Fh = binary.LittleEndian.Uint64(data[0:8])
	r.Offset = binary.LittleEndian.Uint64(data[8:16])
	r.Size = binary.LittleEndian.Uint32(data[16:20])
	r.ReadFlags = binary.LittleEndian.Uint32(data[20:24])
	r.LockOwner = binary.LittleEndian.Uint64(data[24:32])
	r.Flags = binary.LittleEndian.Uint32(data[32:36])
	r.Padding = binary.LittleEndian.Uint32(data[36:40])
	return r, nil
}

// readFuseWriteIn reads a fuse_write_in from data.
func readFuseWriteIn(data []byte) (*fuseWriteIn, error) {
	if len(data) < 40 {
		return nil, fmt.Errorf("write_in too short")
	}
	w := &fuseWriteIn{}
	w.Fh = binary.LittleEndian.Uint64(data[0:8])
	w.Offset = binary.LittleEndian.Uint64(data[8:16])
	w.Size = binary.LittleEndian.Uint32(data[16:20])
	w.WriteFlags = binary.LittleEndian.Uint32(data[20:24])
	w.LockOwner = binary.LittleEndian.Uint64(data[24:32])
	w.Flags = binary.LittleEndian.Uint32(data[32:36])
	w.Padding = binary.LittleEndian.Uint32(data[36:40])
	return w, nil
}

// writeFuseOutHeader writes a fuse_out_header to buf.
func writeFuseOutHeader(buf []byte, h *fuseOutHeader) {
	binary.LittleEndian.PutUint32(buf[0:4], h.Len)
	binary.LittleEndian.PutUint32(buf[4:8], uint32(h.Error))
	binary.LittleEndian.PutUint64(buf[8:16], h.Unique)
}

// modeFromType converts file type to fuse mode.
func modeFromType(isDir bool) uint32 {
	if isDir {
		return syscall.S_IFDIR | 0755
	}
	return syscall.S_IFREG | 0644
}

