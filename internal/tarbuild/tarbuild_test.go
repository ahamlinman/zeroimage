package tarbuild

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

var defaultModTime = time.Date(2021, time.October, 24, 2, 36, 42, 0, time.UTC)

func TestBuilder(t *testing.T) {
	type testEntry struct {
		Path    string
		Content interface{}
	}

	testCases := []struct {
		Description string
		Entries     []testEntry
		WantHeaders []tar.Header
		WantError   error
	}{
		{
			Description: "basic test",
			Entries: []testEntry{
				{"etc/hostname", "test.example.com"},
				{"etc/passwd", File{
					Reader: strings.NewReader("root:x:0:0:root:/root:/bin/sh"),
					Size:   29, Mode: 0644, ModTime: defaultModTime}},
				{"tmp", Dir{Mode: fs.ModeDir | fs.ModeSticky | 0777, ModTime: defaultModTime}},
			},
			WantHeaders: []tar.Header{
				{Typeflag: tar.TypeDir, Name: "etc/", Mode: 0755, ModTime: defaultModTime},
				{Typeflag: tar.TypeReg, Name: "etc/hostname", Size: 16, Mode: 0644, ModTime: defaultModTime},
				{Typeflag: tar.TypeReg, Name: "etc/passwd", Size: 29, Mode: 0644, ModTime: defaultModTime},
				{Typeflag: tar.TypeDir, Name: "tmp/", Mode: 01777, ModTime: defaultModTime},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Description, func(t *testing.T) {
			var archive bytes.Buffer
			builder := NewBuilder(&archive)
			builder.DefaultModTime = defaultModTime
			for _, entry := range tc.Entries {
				switch content := entry.Content.(type) {
				case string:
					builder.AddContent(entry.Path, []byte(content))
				case fs.File:
					builder.Add(entry.Path, content)
				default:
					t.Fatalf("invalid test case: unrecognized entry content type: %T", entry.Content)
				}
			}

			err := builder.Close()
			if err != nil {
				if tc.WantError == nil {
					t.Fatalf("unexpected error: %v", err)
				} else if !errors.Is(err, tc.WantError) {
					t.Fatalf("error was different from expected\ngot:  %v\nwant: %v", err, tc.WantError)
				} else {
					return
				}
			}

			tr := tar.NewReader(&archive)
			var gotHeaders []tar.Header
			for {
				header, err := tr.Next()
				if errors.Is(err, io.EOF) {
					break
				} else if err != nil {
					t.Fatalf("error reading archive: %v", err)
				}
				gotHeaders = append(gotHeaders, *header)
			}

			diff := cmp.Diff(
				tc.WantHeaders, gotHeaders,
				cmpopts.IgnoreFields(tar.Header{}, "Format"),
			)
			if diff != "" {
				t.Errorf("unexpected archive contents (-want +got):\n%s", diff)
			}
		})
	}
}
