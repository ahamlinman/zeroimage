package image

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func TestPlatform(t *testing.T) {
	testCases := []struct {
		Input        string
		WantPlatform specsv1.Platform
		WantError    bool
	}{
		{
			Input:        "linux/amd64",
			WantPlatform: specsv1.Platform{OS: "linux", Architecture: "amd64"},
		},
		{
			Input:        "linux/arm64/v8",
			WantPlatform: specsv1.Platform{OS: "linux", Architecture: "arm64", Variant: "v8"},
		},
		{Input: "linux", WantError: true},
		{Input: "linux/5.17/arm64/v8", WantError: true},
		{Input: "linux/", WantError: true},
		{Input: "/arm64", WantError: true},
		{Input: "linux/arm/", WantError: true},
	}

	for _, tc := range testCases {
		t.Run(tc.Input, func(t *testing.T) {
			gotPlatform, err := ParsePlatform(tc.Input)
			if err != nil {
				if !tc.WantError {
					t.Fatalf("unexpected error: %v", err)
				} else {
					return
				}
			}
			if tc.WantError {
				t.Fatal("no error for expected invalid input")
			}

			if diff := cmp.Diff(tc.WantPlatform, gotPlatform); diff != "" {
				t.Errorf("unexpected result while parsing platform (-want +got):\n%s", diff)
			}

			gotFormat := FormatPlatform(gotPlatform)
			if gotFormat != tc.Input {
				t.Errorf("unexpected result while formatting platform: %s", gotFormat)
			}
		})
	}
}
