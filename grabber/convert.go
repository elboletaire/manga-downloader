// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"strings"
)

// Source image formats that can be transcoded to JPEG when packing, plus the
// "all" and "none" values. The format names match the extensions packer's
// sniffing reports, so no translation table is needed between the two.
const (
	ConvertAVIF = "avif"
	ConvertWebP = "webp"
	// ConvertPNG is converted only when the JPEG comes out smaller, since
	// PNG is readable everywhere and a bigger JPEG would just cost quality
	ConvertPNG = "png"
	// ConvertAll converts every page that isn't already JPEG, except animated
	// GIF (see Has)
	ConvertAll = "all"
	// ConvertImagesNone disables conversion entirely, keeping every page in
	// the format the site served it in
	ConvertImagesNone = "none"
	// ConvertImagesDefault converts AVIF only: no e-reader can render it, so
	// there's nothing to lose. WebP is left alone unless asked for, since
	// phone readers handle it fine and transcoding costs quality and size.
	ConvertImagesDefault = ConvertAVIF
	// JPEGQualityDefault is the quality pages are re-encoded at. The stdlib
	// default (75) visibly muddies the screentones and small lettering that
	// make up most of a manga page.
	JPEGQualityDefault = 90
)

// ConvertFormats is the set of source image formats to transcode to JPEG when
// packing, keyed by the extension the packer detects from the image bytes.
type ConvertFormats map[string]bool

// Has reports whether the given format should be converted. It's nil-safe, so
// an unset value simply converts nothing.
func (c ConvertFormats) Has(format string) bool {
	if c[ConvertAll] {
		// animated GIF stays as it is: converting keeps only its first frame
		return format != "jpg" && format != "gif"
	}
	return c[format]
}

// ParseConvertFormats parses the --convert-images value: a comma separated
// list of source formats to convert to JPEG ("avif", "avif,webp", "png"), "all"
// for every format but JPEG and GIF, or "none" (or an empty string) to disable
// conversion. Formats e-readers already handle (jpg/gif) are rejected rather
// than silently accepted, since converting them would be a lossy downgrade.
func ParseConvertFormats(value string) (ConvertFormats, error) {
	formats := ConvertFormats{}

	value = strings.TrimSpace(value)
	if value == "" {
		return formats, nil
	}

	tokens := strings.Split(value, ",")
	for _, token := range tokens {
		format := strings.ToLower(strings.TrimSpace(token))

		switch format {
		case ConvertAVIF, ConvertWebP, ConvertPNG, ConvertAll:
			formats[format] = true
		case ConvertImagesNone:
			// "none" is exclusive: pairing it with a format contradicts itself
			if len(tokens) > 1 {
				return nil, fmt.Errorf("invalid --convert-images value %q, %q cannot be combined with other formats", value, ConvertImagesNone)
			}
		default:
			return nil, fmt.Errorf("invalid --convert-images format %q, valid formats are %q, %q, %q, %q or %q", token, ConvertAVIF, ConvertWebP, ConvertPNG, ConvertAll, ConvertImagesNone)
		}
	}

	return formats, nil
}
