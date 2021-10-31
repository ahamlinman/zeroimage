// Package ociarchive works with tar archives whose contents comply with the OCI
// Image Layout Specification.
package ociarchive

import specsv1 "github.com/opencontainers/image-spec/specs-go/v1"

// extendedImage implements some properties of the OCI image configuration
// specification that the Go types do not yet include.
type extendedImage struct {
	specsv1.Image
	OSVersion  string   `json:"os.version,omitempty"`
	OSFeatures []string `json:"os.features,omitempty"`
	Variant    string   `json:"variant,omitempty"`
}
