// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestParseConvertFormats(t *testing.T) {
	cases := []struct {
		in      string
		want    []string
		wantErr bool
	}{
		{"avif", []string{"avif"}, false},
		{"avif,webp", []string{"avif", "webp"}, false},
		{"  AVIF , WebP ", []string{"avif", "webp"}, false}, // trimmed and lowercased
		{"avif,avif", []string{"avif"}, false},              // duplicates are set semantics
		{"none", nil, false},
		{"", nil, false},
		// already e-reader friendly, converting them would be a lossy downgrade
		{"jpg", nil, true},
		{"png", nil, true},
		{"gif", nil, true},
		{"bogus", nil, true},
		// "none" is exclusive: combining it with a format is a contradiction
		{"avif,none", nil, true},
		{"none,avif", nil, true},
		// a trailing comma is a typo, not an empty token to ignore
		{"avif,", nil, true},
	}

	for _, c := range cases {
		got, err := ParseConvertFormats(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseConvertFormats(%q) = %v, want an error", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseConvertFormats(%q) returned an unexpected error: %s", c.in, err)
			continue
		}

		if len(got) != len(c.want) {
			t.Errorf("ParseConvertFormats(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for _, format := range c.want {
			if !got.Has(format) {
				t.Errorf("ParseConvertFormats(%q) = %v, missing %q", c.in, got, format)
			}
		}
	}
}

// TestConvertFormatsHasIsNilSafe pins the nil-map behaviour a zero-valued
// Settings (and any Site implementation that doesn't set one) relies on: an
// unset value converts nothing rather than panicking.
func TestConvertFormatsHasIsNilSafe(t *testing.T) {
	var formats ConvertFormats
	if formats.Has(ConvertAVIF) {
		t.Error("a nil ConvertFormats should not convert anything")
	}
}
