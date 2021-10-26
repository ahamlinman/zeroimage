package tarbuild

import (
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

// Sys returns the underlying File.
func (fi FileInfo) Sys() interface{} {
	return fi.File
}
