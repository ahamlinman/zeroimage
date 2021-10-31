// Package image provides common types to represent container images and their
// filesystem layers, based on the Go types defined by the OCI Image Layout
// Specification.
package image

import (
	"context"
	"io"

	"github.com/opencontainers/go-digest"
	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// Index represents an OCI image index that references platform specific
// container images.
type Index []IndexEntry

// IndexEntry represents a reference to a platform specific container image in
// an OCI image index.
type IndexEntry struct {
	Platform specsv1.Platform
	Image    func(context.Context) (Image, error)
}

// SelectByPlatform returns a new Index of the images in idx where all non empty
// fields of platform are an exact match or subset of the image's Platform
// values.
//
// Examples
//
// If idx references images for both the v6 and v7 variants of the linux/arm
// platform, and platform.Variant is empty, SelectByPlatform will return an
// index referencing both images.
//
// If idx only references an image for the v8 variant of the linux/arm64
// platform, and platform.Variant is empty, SelectByPlatform will return an
// index referencing only that image.
//
// If idx only references an image for the v8 variant of the linux/arm64
// platform, and platform.Variant is set to "v9", SelectByPlatform will return
// an empty index.
//
// If idx includes Windows images and platform.OSFeatures includes "win32k",
// SelectByPlatform will only return images that include the "win32k" feature.
// The returned images may include other OS features as well.
func (idx Index) SelectByPlatform(platform specsv1.Platform) Index {
	var selected Index
	for _, img := range idx {
		if platformMatches(platform, img.Platform) {
			selected = append(selected, img)
		}
	}
	return selected
}

func platformMatches(requested, comparand specsv1.Platform) bool {
	if requested.Architecture != "" && requested.Architecture != comparand.Architecture {
		return false
	}
	if requested.OS != "" && requested.OS != comparand.OS {
		return false
	}
	if requested.OSVersion != "" && requested.OSVersion != comparand.OSVersion {
		return false
	}
	if !featuresMatch(requested.OSFeatures, comparand.OSFeatures) {
		return false
	}
	if requested.Variant != "" && requested.Variant != comparand.Variant {
		return false
	}
	return true
}

func featuresMatch(requested, comparand []string) bool {
	comparandSet := make(map[string]bool, len(comparand))
	for _, cmp := range comparand {
		comparandSet[cmp] = true
	}
	for _, req := range requested {
		if !comparandSet[req] {
			return false
		}
	}
	return true
}

// Image represents a platform specific container image.
type Image struct {
	Layers []Layer
	// Config represents the OCI image configuration for this image.
	Config specsv1.Image
	// Platform represents the "platform" value for this image in the "manifests"
	// array of an OCI image index.
	Platform specsv1.Platform
	// Annotations represents the "annotations" value for the OCI image manifest
	// associated with this image.
	Annotations map[string]string
}

// Layer represents a single filesystem layer in a container image.
type Layer struct {
	Descriptor specsv1.Descriptor
	DiffID     digest.Digest
	Blob       func(context.Context) (io.ReadCloser, error)
}

// AppendLayer appends layer to img.Layers and updates corresponding values of
// img.Config.
func (img *Image) AppendLayer(layer Layer) {
	img.Layers = append(img.Layers, layer)
	img.Config.RootFS.DiffIDs = append(img.Config.RootFS.DiffIDs, layer.DiffID)
}

// SetPlatform sets img.Platform and updates corresponding values of img.Config.
func (img *Image) SetPlatform(platform specsv1.Platform) {
	img.Platform = platform
	img.Config.OS = platform.OS
	img.Config.Architecture = platform.Architecture
}
