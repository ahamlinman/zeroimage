package ociarchive

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"go.alexhamlin.co/zeroimage/internal/image"
)

func TestRoundTripExistingArchive(t *testing.T) {
	// Ensure that we can round-trip an OCI archive of the Docker "hello-world"
	// image pulled with Skopeo. This ensures that we can load a basic
	// single-platform image from Skopeo, and that our write implementation is at
	// least good enough for ourselves to load.

	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	archiveFile, err := os.Open(filepath.Join(wd, "testdata", "hello-world-linux-arm64.tar"))
	if err != nil {
		panic(err)
	}
	defer archiveFile.Close()

	index, err := Load(archiveFile)
	if err != nil {
		t.Fatalf("failed to load original archive: %v", err)
	}
	if len(index) != 1 {
		t.Fatalf("test archive contained %d image(s), want 1", len(index))
	}

	originalImage, err := index[0].GetImage(context.Background())
	if err != nil {
		t.Fatalf("failed to load original image: %v", err)
	}

	var buf bytes.Buffer
	err = WriteImage(originalImage, &buf)
	if err != nil {
		t.Fatalf("failed to write valid image: %v", err)
	}

	rewrittenIndex, err := Load(&buf)
	if err != nil {
		t.Fatalf("failed to load rewritten archive: %v", err)
	}
	rewrittenImage, err := rewrittenIndex[0].GetImage(context.Background())
	if err != nil {
		t.Fatalf("failed to load rewritten image: %v", err)
	}

	diff := cmp.Diff(
		originalImage, rewrittenImage,
		cmpopts.IgnoreFields(image.Layer{}, "OpenBlob"),
	)
	if diff != "" {
		t.Errorf("images not equivalent after round-trip (-want +got):\n%s", diff)
	}
}
