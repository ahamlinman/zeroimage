package image

import (
	"errors"
	"strings"

	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// ParsePlatform parses an image platform from a slash separated format
// consisting of the OS, architecture, and optional variant.
func ParsePlatform(ps string) (specsv1.Platform, error) {
	parts := strings.Split(ps, "/")
	if len(parts) < 2 || len(parts) > 3 {
		return specsv1.Platform{}, errors.New("must have 2 or 3 slash separated parts")
	}

	p := specsv1.Platform{OS: parts[0], Architecture: parts[1]}
	if p.OS == "" {
		return specsv1.Platform{}, errors.New("missing OS in platform")
	}
	if p.Architecture == "" {
		return specsv1.Platform{}, errors.New("missing architecture in platform")
	}

	if len(parts) == 3 {
		p.Variant = parts[2]
		if p.Variant == "" {
			return specsv1.Platform{}, errors.New("empty variant in platform")
		}
	}

	return p, nil
}

// FormatPlatform formats p into a slash separated format consisting of the OS,
// architecture, and optional variant.
func FormatPlatform(p specsv1.Platform) string {
	if p.Variant != "" {
		return strings.Join([]string{p.OS, p.Architecture, p.Variant}, "/")
	}
	return strings.Join([]string{p.OS, p.Architecture}, "/")
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
