package tarbuild

import (
	"errors"
	"io"
	"io/fs"
	"time"
)

// File represents a file in a tar archive, the contents of which are produced
// by the embedded io.Reader, and the metadata of which is explicitly defined.
type File struct {
	io.Reader
	Size    int64
	Mode    fs.FileMode
	ModTime time.Time
	Sys     interface{}
}

// Stat returns the FileInfo representing f.
func (f File) Stat() (fs.FileInfo, error) {
	return FileInfo{f}, nil
}

// Close closes f's io.Reader if it is also an io.Closer. Otherwise, Close is a
// no-op.
func (f File) Close() error {
	if c, ok := f.Reader.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// FileInfo is the underlying type of the fs.FileInfo returned by the Stat
// method of a File.
type FileInfo struct {
	File
}

// Name returns the empty string. The tarbuild package ignores this field when
// adding a file to an archive, as the user is expected to provide a full
// destination path rather than a basename alone.
func (fi FileInfo) Name() string {
	return ""
}

// Size returns the Size of the underlying File.
func (fi FileInfo) Size() int64 {
	return fi.File.Size
}

// Mode returns the Mode of the underlying File.
func (fi FileInfo) Mode() fs.FileMode {
	return fi.File.Mode
}

// ModTime returns the ModTime of the underlying File.
func (fi FileInfo) ModTime() time.Time {
	return fi.File.ModTime
}

// IsDir returns false.
func (fi FileInfo) IsDir() bool {
	return false
}

// Sys returns the Sys of the underlying File.
func (fi FileInfo) Sys() interface{} {
	return fi.File.Sys
}

// Dir represents a tar entry for an empty directory, the metadata of which is
// explicitly defined.
type Dir struct {
	Mode    fs.FileMode
	ModTime time.Time
	Sys     interface{}
}

func (d Dir) Stat() (fs.FileInfo, error) {
	return DirInfo{d}, nil
}

var _ fs.ReadDirFile = Dir{}

func (Dir) ReadDir(_ int) ([]fs.DirEntry, error) {
	return nil, nil
}

func (Dir) Read(_ []byte) (int, error) {
	return 0, errors.New("tarbuild: cannot read Dir")
}

func (Dir) Close() error {
	return nil
}

// FileInfo is the underlying type of the fs.FileInfo returned by the Stat
// method of a File.
type DirInfo struct {
	Dir
}

// Name returns the empty string. The tarbuild package ignores this field when
// adding a directory to an archive, as the user is expected to provide a full
// destination path rather than a basename alone.
func (di DirInfo) Name() string {
	return ""
}

// Size returns 0.
func (di DirInfo) Size() int64 {
	return 0
}

// Mode returns the Mode of the underlying Dir.
func (di DirInfo) Mode() fs.FileMode {
	return di.Dir.Mode
}

// ModTime returns the ModTime of the underlying Dir.
func (di DirInfo) ModTime() time.Time {
	return di.Dir.ModTime
}

// IsDir returns true.
func (di DirInfo) IsDir() bool {
	return true
}

// Sys returns the Sys of the underlying Dir.
func (di DirInfo) Sys() interface{} {
	return di.Dir.Sys
}
