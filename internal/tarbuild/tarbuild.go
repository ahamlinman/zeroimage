package tarbuild

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"strings"
)

// ErrClosed is returned when attempting to add entries to a closed Builder.
var ErrClosed = errors.New("tarbuild: builder closed")

// Builder creates a tape archive (tar) in an opinionated manner.
//
// All entries in the archive will have their user and group IDs set to 0
// (root). Unless otherwise specified, all entries will have a modification time
// at the start of the Unix epoch (1 January 1970 at midnight UTC).
//
// If an error occurs while using a Builder, no more entries will be written to
// the archive and all subsequent operations, and Close, will return the error.
// After all entries have been written, the client must call Close to write the
// tar footer. It is an error to attempt to add entries to a closed Builder.
type Builder struct {
	tw  *tar.Writer
	err error
}

// NewBuilder returns a Builder that writes a tar archive to w.
func NewBuilder(w io.Writer) *Builder {
	return &Builder{tw: tar.NewWriter(w)}
}

// AddDirectory adds an entry for a directory with mode 755 at the provided
// path.
func (b *Builder) AddDirectory(path string) error {
	if b.err != nil {
		return b.err
	}

	path = strings.TrimSuffix(path, "/") + "/"
	b.err = b.tw.WriteHeader(&tar.Header{Name: path, Mode: 040755})
	return b.err
}

// AddFileContent adds an entry for a file with mode 644 containing the provided
// content at the provided path.
func (b *Builder) AddFileContent(path string, content []byte) error {
	if b.err != nil {
		return b.err
	}

	err := b.tw.WriteHeader(&tar.Header{
		Name: path,
		Size: int64(len(content)),
		Mode: 0644,
	})
	if err != nil {
		b.err = err
		return err
	}

	_, b.err = io.Copy(b.tw, bytes.NewReader(content))
	return b.err
}

// AddFile adds an entry for the provided file at the provided path, including
// the file's mode bits, modification time, and extended attributes, but not
// including the file's owner, group, or original name. It reads the file to
// copy it into the archive, but does not close it.
func (b *Builder) AddFile(path string, file fs.File) error {
	if b.err != nil {
		return b.err
	}

	stat, err := file.Stat()
	if err != nil {
		b.err = err
		return err
	}

	header, err := tar.FileInfoHeader(stat, "")
	if err != nil {
		b.err = err
		return err
	}
	header.Name = path
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
