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
// All entries in the archive will have clean relative paths, and will be owned
// by UID and GID 0. Before writing an entry, a Builder will add all parent
// directories of the entry that have not yet been added. These directories will
// have mode 755, and their modification times will be set to the time at which
// the Builder was created.
//
// If an error occurs while using a Builder, no more entries will be written to
// the archive and all subsequent operations, and Close, will return the error.
// After all entries have been written, the client must call Close to write the
// tar footer. It is an error to attempt to add entries to a closed Builder.
type Builder struct {
	tw      *tar.Writer
	err     error
	modTime time.Time
	entries map[npath]tarTypeflag
}

// tarTypeflag matches the type of the Typeflag field in tar.Header.
type tarTypeflag = byte

// npath is a normalized path: a Clean relative path separated by forward
// slashes.
//
// Because npath is a relative path, it contains no leading or trailing slashes,
// and the root path is represented as ".".
type npath string

// normalizePath returns the npath corresponding to the provided slash-separated
// path.
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
		entries: make(map[npath]tarTypeflag),
	}
}

// AddContent adds the provided content to the archive as a file at the provided
// path. The file will have mode 644, and its modification time will be set to
// the time at which the Builder was created.
func (b *Builder) AddContent(path string, content []byte) error {
	return b.Add(path, File{
		Reader:  bytes.NewReader(content),
		Size:    int64(len(content)),
		Mode:    0644,
		ModTime: b.modTime,
	})
}

// Add adds the provided file to the archive at the provided path. It preserves
// the original size, mode, and modification time reported by file.Stat, and may
// preserve some fields of file.Stat.Sys, but does not preserve the original
// name, owner, or group.
//
// Add reads the provided file in order to copy it into the archive, but does
// not close it.
func (b *Builder) Add(path string, file fs.File) (err error) {
	if b.err != nil {
		return b.err
	}
	defer func() {
		if err != nil {
			b.err = err
		}
	}()

	np := normalizePath(path)

	if _, ok := b.entries[np]; ok {
		return DuplicateEntryError(string(np))
	} else {
		b.entries[np] = tar.TypeReg
	}

	if err := b.ensureParentDirectory(np); err != nil {
		return err
	}

	stat, err := file.Stat()
	if err != nil {
		return err
	}

	header, err := tar.FileInfoHeader(stat, "")
	if err != nil {
		return err
	}
	header.Name = string(np)
	header.Uid = 0
	header.Gid = 0
	header.Uname = ""
	header.Gname = ""
	if err := b.tw.WriteHeader(header); err != nil {
		return err
	}

	_, err = io.Copy(b.tw, file)
	return err
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
	// This function operates entirely on the *parent* of np, to ensure that the
	// caller can handle the b.entries checks for np itself as it sees fit. As
	// such, np should never show up after this line.
	//
	// Because np is an npath, we know that it does not contain a leading slash.
	// path.Dir is documented to Clean the resulting parent path, remove trailing
	// slashes, and return "." in place of an empty path. Based on these rules, we
	// know that the result of path.Dir on an npath will itself be a valid npath.
	parent := npath(path.Dir(string(np)))

	if parent == "." {
		return nil
	}

	if typeflag, ok := b.entries[parent]; ok {
		if typeflag != tar.TypeDir {
			return DuplicateEntryError(string(parent))
		}
		// TODO: This may be problematic if a user explicitly adds directory entries
		// without adding their parents first. Might be good to lock that down?
		return nil
	}

	if err := b.ensureParentDirectory(parent); err != nil {
		return err
	}

	b.entries[parent] = tar.TypeDir
	return b.tw.WriteHeader(&tar.Header{
		Name:    string(parent) + "/",
		Mode:    040755,
		ModTime: b.modTime,
	})
}
