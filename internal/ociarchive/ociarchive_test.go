package ociarchive

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	// Required by github.com/opencontainers/go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"go.alexhamlin.co/zeroimage/internal/image"
)

func TestRoundTripExistingArchive(t *testing.T) {
	// Ensure that we can round-trip an OCI archive of the Docker "hello-world"
	// image pulled with Skopeo. This ensures that we can load a basic
	// single-platform image from Skopeo, and that our write implementation is at
	// least good enough for ourselves to load.
	index, err := loadTestdataArchive("hello-world-linux-arm64.tar")
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

func TestLoadMultiarchArchive(t *testing.T) {
	// Ensure that we can load a multi-platform OCI archive of the Docker
	// "hello-world" image pulled with Skopeo.
	index, err := loadTestdataArchive("hello-world-multiarch.tar")
	if err != nil {
		t.Fatalf("failed to load original archive: %v", err)
	}

	gotPlatforms := make([]string, len(index))
	for i, entry := range index {
		gotPlatforms[i] = image.FormatPlatform(entry.Platform)
	}
	wantPlatforms := []string{
		"linux/amd64", "linux/arm/v5", "linux/arm/v7", "linux/arm64/v8",
		"linux/386", "linux/mips64le", "linux/ppc64le", "linux/riscv64",
		"linux/s390x", "windows/amd64", "windows/amd64",
	}
	if diff := cmp.Diff(wantPlatforms, gotPlatforms); diff != "" {
		t.Errorf("unexpected list of platforms (-want +got):\n%s", diff)
	}
}

func loadTestdataArchive(name string) (image.Index, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	archiveFile, err := os.Open(filepath.Join(wd, "testdata", name))
	if err != nil {
		return nil, err
	}
	defer archiveFile.Close()

	return Load(archiveFile)
}
