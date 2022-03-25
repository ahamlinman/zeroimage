// Package tarbuild provides a simplified interface to build tar archives.
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

// ErrBuilderClosed is returned when attempting to add entries to a closed
// Builder.
var ErrBuilderClosed = errors.New("tarbuild: builder closed")

// ErrDuplicateEntry is the cause of an AddError resulting from an attempt to
// add multiple entries at the same path.
var ErrDuplicateEntry = errors.New("duplicate entry")

// AddError represents an error that occurred while adding the entry at Path to
// a tar archive.
type AddError struct {
	Path string
	Err  error
}

func (aerr AddError) Error() string {
	return fmt.Sprintf("tarbuild: add %s: %s", aerr.Path, aerr.Err.Error())
}

func (aerr AddError) Unwrap() error {
	return aerr.Err
}

// Builder is a convenient, opinionated tape archive (tar) builder.
//
// All entries in the archive will have clean relative paths, and will be owned
// by UID and GID 0. Before writing an entry, a Builder will add all parent
// directories of the entry that have not yet been added. These directories will
// have mode 755, and their modification times will be set to DefaultModTime.
//
// If an error occurs while using a Builder, no more entries will be written to
// the archive and all subsequent operations, and Close, will return the error.
// After all entries have been written, the client must call Close to write the
// tar footer. It is an error to attempt to add entries to a closed Builder.
type Builder struct {
	DefaultModTime time.Time

	tw      *tar.Writer
	err     error
	entries map[npath]tarTypeflag
}

// tarTypeflag matches the type of the Typeflag field in tar.Header.
type tarTypeflag = byte

// npath holds a normalized path: a Clean relative path separated by forward
// slashes. Because an npath is relative, it contains no leading or trailing
// slashes, and the root path is represented as ".".
type npath string

// normalizePath converts an arbitrary slash-separated path to an npath.
func normalizePath(p string) npath {
	p = path.Clean(p)
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		p = "."
	}
	return npath(p)
}

// NewBuilder returns a Builder that writes a tar archive to w, and whose
// DefaultModTime is initialized to the current UTC time.
func NewBuilder(w io.Writer) *Builder {
	return &Builder{
		DefaultModTime: time.Now().UTC(),
		tw:             tar.NewWriter(w),
		entries:        make(map[npath]tarTypeflag),
	}
}

// AddContent adds the provided content to the archive as a file following the
// semantics of Add, with mode 644 and the Builder's DefaultModTime as the
// modification time.
func (b *Builder) AddContent(path string, content []byte) error {
	return b.Add(path, File{
		Reader:  bytes.NewReader(content),
		Size:    int64(len(content)),
		Mode:    0644,
		ModTime: b.DefaultModTime,
	})
}

// Add adds the provided file to the archive at the provided path, creating any
// necessary parent directories as described by Builder. Add preserves the size,
// mode, and modification time reported by file.Stat, and may preserve some
// fields of file.Stat.Sys, but does not preserve the original name, owner, or
// group of the file.
//
// When file represents a regular file, Add immediately copies its contents into
// the archive.
//
// When file represents a directory, Add creates an entry for an empty directory
// using the fields of file.Stat as defined above, without regard to the
// contents of the directory. To add a directory of files to an archive while
// preserving Stat fields, first add the directory, then add each file that it
// contains.
func (b *Builder) Add(path string, file fs.File) (err error) {
	if b.err != nil {
		return b.err
	}

	np := normalizePath(path)

	defer func() {
		if err != nil {
			if aerr, ok := err.(AddError); ok {
				b.err = aerr
			} else {
				b.err = AddError{string(np), err}
			}
		}
	}()

	if _, ok := b.entries[np]; ok {
		return ErrDuplicateEntry
	}
	b.entries[np] = tar.TypeReg

	err = b.ensureParentDirectory(np)
	if err != nil {
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
	if stat.IsDir() && !strings.HasSuffix(header.Name, "/") {
		header.Name += "/"
	}

	header.Uid = 0
	header.Gid = 0
	header.Uname = ""
	header.Gname = ""
	if err := b.tw.WriteHeader(header); err != nil {
		return err
	}

	if !stat.IsDir() {
		_, err = io.Copy(b.tw, file)
	}

	return err
}

func (b *Builder) ensureParentDirectory(np npath) error {
	// This function operates entirely on the *parent* of np, to ensure that the
	// caller can handle the b.entries checks for np itself as it sees fit. As
	// such, np should never show up after this line.
	//
	// Because np is an npath, we know that it does not contain a leading slash.
	// path.Dir is documented to Clean the resulting parent path, remove trailing
	// slashes, and return "." in place of an empty path. Based on these rules, we
	// know that the result of path.Dir on an npath will be a valid npath.
	parent := npath(path.Dir(string(np)))

	if parent == "." {
		return nil
	}

	if typeflag, ok := b.entries[parent]; ok {
		if typeflag != tar.TypeDir {
			return AddError{
				Path: string(parent),
				Err:  fmt.Errorf("%w: cannot add directory where non-directory exists", ErrDuplicateEntry),
			}
		}
		// Whoever added this parent should have filled out the rest of the chain.
		return nil
	}

	if err := b.ensureParentDirectory(parent); err != nil {
		return err
	}

	b.entries[parent] = tar.TypeDir
	return b.tw.WriteHeader(&tar.Header{
		Name:    string(parent) + "/",
		Mode:    0755,
		ModTime: b.DefaultModTime,
	})
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

	b.err = ErrBuilderClosed
	return nil
}
