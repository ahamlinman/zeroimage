package tarbuild

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"time"
)

// ErrClosed is returned when attempting to add entries to a closed Builder.
var ErrClosed = errors.New("tarbuild: builder closed")

// DuplicateEntryError is returned when attempting to add multiple entries to an
// archive with the same path. The string contains the duplicated path.
type DuplicateEntryError string

func (derr DuplicateEntryError) Error() string {
	return fmt.Sprintf("tarbuild: duplicate entry %s", string(derr))
}

// Builder creates a tape archive (tar) in an opinionated manner.
//
// All entries in the archive will have their user and group IDs set to 0
// (root). Unless otherwise specified, the modification time of all entries will
// be the time at which the Builder was initialized. The Builder will create all
// necessary parent directories for added files with mode 755.
//
// If an error occurs while using a Builder, no more entries will be written to
// the archive and all subsequent operations, and Close, will return the error.
// After all entries have been written, the client must call Close to write the
// tar footer. It is an error to attempt to add entries to a closed Builder.
type Builder struct {
	tw      *tar.Writer
	err     error
	modTime time.Time
	added   map[npath]tarTypeflag
}

// tarTypeflag matches the type of the Typeflag field in tar.Header.
type tarTypeflag = byte

// npath represents a path for which path == normalizePath(path).
type npath string

// normalizePath normalizes the provided file or directory path for use in a tar
// archive. In addition to the lexical processing performed by path.Clean,
// normalizePath transforms absolute paths into relative paths from the root of
// the archive. In particular, the root path normalizes to ".".
func normalizePath(p string) npath {
	p = path.Clean(p)
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		p = "."
	}
	return npath(p)
}

// NewBuilder returns a Builder that writes a tar archive to w.
func NewBuilder(w io.Writer) *Builder {
	return &Builder{
		tw:      tar.NewWriter(w),
		modTime: time.Now().UTC(),
		added:   make(map[npath]tarTypeflag),
	}
}

// AddFileContent adds an entry for a file with mode 644 containing the provided
// content at the provided path.
func (b *Builder) AddFileContent(path string, content []byte) error {
	return b.AddFile(path, File{
		Reader:  bytes.NewReader(content),
		Size:    int64(len(content)),
		Mode:    0644,
		ModTime: b.modTime,
	})
}

// AddFile adds an entry for the provided file at the provided path, including
// the file's mode bits, modification time, and extended attributes, but not
// including the file's owner, group, or original name. It reads the file to
// copy it into the archive, but does not close it.
func (b *Builder) AddFile(path string, file fs.File) error {
	if b.err != nil {
		return b.err
	}

	np := normalizePath(path)

	if _, ok := b.added[np]; ok {
		return DuplicateEntryError(string(np))
	} else {
		b.added[np] = tar.TypeReg
	}

	b.err = b.ensureParentDirectory(np)
	if b.err != nil {
		return b.err
	}

	var stat fs.FileInfo
	stat, b.err = file.Stat()
	if b.err != nil {
		return b.err
	}

	var header *tar.Header
	header, b.err = tar.FileInfoHeader(stat, "")
	if b.err != nil {
		return b.err
	}
	header.Name = string(np)
	header.Uid = 0
	header.Gid = 0
	header.Uname = ""
	header.Gname = ""
	if err := b.tw.WriteHeader(header); err != nil {
		b.err = err
		return err
	}

	_, b.err = io.Copy(b.tw, file)
	return b.err
}

// Close finishes writing the tar archive if all entries were added
// successfully, and returns any error encountered while adding entries.
func (b *Builder) Close() error {
	if b.err != nil {
		return b.err
	}

	b.err = b.tw.Close()
	if b.err != nil {
		return b.err
	}

	b.err = ErrClosed
	return nil
}

func (b *Builder) ensureParentDirectory(np npath) error {
	// This function has to operate entirely on the *parent* of our target path,
	// as the caller is expected to add the entry for the target path itself.
	//
	// path.Dir cleans the resulting path and returns "." for empty directories,
	// satisfying our normalized path rules.
	parent := npath(path.Dir(string(np)))

	if parent == "." {
		return nil
	}

	if typeflag, ok := b.added[parent]; ok {
		if typeflag == tar.TypeDir {
			return nil
		}
		return DuplicateEntryError(string(parent))
	}

	if err := b.ensureParentDirectory(parent); err != nil {
		return err
	}

	b.added[parent] = tar.TypeDir
	return b.tw.WriteHeader(&tar.Header{
		Name:    string(parent) + "/",
		Mode:    040755,
		ModTime: b.modTime,
	})
}
