package image

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func TestSelectByPlatform(t *testing.T) {
	p := func(ps string) specsv1.Platform {
		pp, err := ParsePlatform(ps)
		if err != nil {
			t.Fatalf("invalid test case: invalid platform %s: %v", ps, err)
		}
		return pp
	}

	testCases := []struct {
		Description   string
		Platforms     []specsv1.Platform
		Selector      specsv1.Platform
		WantPlatforms []specsv1.Platform
	}{
		{
			Description:   "full match for single platform",
			Platforms:     []specsv1.Platform{p("linux/arm/v7"), p("linux/arm64/v8")},
			Selector:      p("linux/arm64/v8"),
			WantPlatforms: []specsv1.Platform{p("linux/arm64/v8")},
		},
		{
			Description:   "partial match for single platform",
			Platforms:     []specsv1.Platform{p("linux/arm/v7"), p("linux/arm64/v8")},
			Selector:      p("linux/arm64"),
			WantPlatforms: []specsv1.Platform{p("linux/arm64/v8")},
		},
		{
			Description:   "match for multiple platforms",
			Platforms:     []specsv1.Platform{p("linux/arm/v6"), p("linux/arm/v7"), p("linux/arm64/v8")},
			Selector:      p("linux/arm"),
			WantPlatforms: []specsv1.Platform{p("linux/arm/v6"), p("linux/arm/v7")},
		},
		{
			Description: "feature matches",
			Platforms: []specsv1.Platform{
				{OS: "zero", Architecture: "zero", OSFeatures: []string{"widgets", "gadgets"}},
				{OS: "zero", Architecture: "zero", OSFeatures: []string{"widgets"}},
				{OS: "zero", Architecture: "zero", OSFeatures: []string{"gadgets"}},
				{OS: "zero", Architecture: "zero"},
			},
			Selector: specsv1.Platform{OS: "zero", Architecture: "zero", OSFeatures: []string{"widgets"}},
			WantPlatforms: []specsv1.Platform{
				{OS: "zero", Architecture: "zero", OSFeatures: []string{"widgets", "gadgets"}},
				{OS: "zero", Architecture: "zero", OSFeatures: []string{"widgets"}},
			},
		},
		{
			Description: "version matches",
			Platforms: []specsv1.Platform{
				{OS: "zero", Architecture: "zero"},
				{OS: "zero", Architecture: "zero", OSVersion: "42.0.0"},
				{OS: "zero", Architecture: "zero", OSVersion: "43.0.0"},
			},
			Selector:      specsv1.Platform{OS: "zero", Architecture: "zero", OSVersion: "42.0.0"},
			WantPlatforms: []specsv1.Platform{{OS: "zero", Architecture: "zero", OSVersion: "42.0.0"}},
		},
		{
			Description:   "no match for OS",
			Platforms:     []specsv1.Platform{p("linux/amd64"), p("linux/arm64")},
			Selector:      p("windows/amd64"),
			WantPlatforms: nil,
		},
		{
			Description:   "no match for variant",
			Platforms:     []specsv1.Platform{p("linux/arm/v7"), p("linux/arm64/v8")},
			Selector:      p("linux/arm64/v9"),
			WantPlatforms: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Description, func(t *testing.T) {
			index := make(Index, len(tc.Platforms))
			for i, p := range tc.Platforms {
				index[i] = IndexEntry{Platform: p}
			}

			gotEntries := index.SelectByPlatform(tc.Selector)
			gotPlatforms := make([]specsv1.Platform, len(gotEntries))
			for i, e := range gotEntries {
				gotPlatforms[i] = e.Platform
			}

			diff := cmp.Diff(
				tc.WantPlatforms, gotPlatforms,
				cmpopts.EquateEmpty(),
			)
			if diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}
