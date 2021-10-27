package tarbuild

import (
	"errors"
	"io"
	"io/fs"
	"time"
)

// File implements fs.File for an io.Reader, using explicitly defined values for
// the values provided by the associated fs.FileInfo.
type File struct {
	io.Reader
	Name    string
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

// Name returns the Name of the underlying File.
func (fi FileInfo) Name() string {
	return fi.File.Name
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

// Dir implements an fs.File representing an empty directory, using explicitly
// defined values for the values provided by the associated fs.FileInfo.
type Dir struct {
	Name    string
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

// Name returns the Name of the underlying Dir.
func (di DirInfo) Name() string {
	return di.Dir.Name
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
