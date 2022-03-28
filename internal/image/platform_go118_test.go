//go:build go1.18

package image

import "testing"

func FuzzParsePlatform(f *testing.F) {
	f.Add("windows/amd64")
	f.Add("linux/arm/v7")
	f.Add("linux/arm64")

	f.Fuzz(func(t *testing.T, input string) {
		// Test that parsing is panic-free.
		platform, err := ParsePlatform(input)
		if err != nil {
			t.SkipNow()
		}

		// Test that valid platforms can round-trip to their original input form
		// after parsing.
		gotFormat := FormatPlatform(platform)
		if gotFormat != input {
			t.Errorf("failed to round-trip %q: got %q", input, gotFormat)
		}
	})
}
