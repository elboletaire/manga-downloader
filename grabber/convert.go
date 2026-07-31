// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"strings"
)

// Source image formats that can be transcoded to JPEG when packing, plus the
// "none" opt-out. The format names match the extensions packer's sniffing
// reports, so no translation table is needed between the two.
const (
	ConvertAVIF = "avif"
	ConvertWebP = "webp"
	// ConvertImagesNone disables conversion entirely, keeping every page in
	// the format the site served it in
	ConvertImagesNone = "none"
	// ConvertImagesDefault converts AVIF only: no e-reader can render it, so
	// there's nothing to lose. WebP is left alone unless asked for, since
	// phone readers handle it fine and transcoding costs quality and size.
	ConvertImagesDefault = ConvertAVIF
)

// ConvertFormats is the set of source image formats to transcode to JPEG when
// packing, keyed by the extension the packer detects from the image bytes.
type ConvertFormats map[string]bool

// Has reports whether the given format should be converted. It's nil-safe, so
// an unset value simply converts nothing.
func (c ConvertFormats) Has(format string) bool {
	return c[format]
}

// ParseConvertFormats parses the --convert-images value: a comma separated
// list of source formats to convert to JPEG ("avif", "avif,webp"), or "none"
// (or an empty string) to disable conversion. Formats e-readers already
// handle (jpg/png/gif) are rejected rather than silently accepted, since
// converting them would be a lossy downgrade for no compatibility gain.
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
		case ConvertAVIF, ConvertWebP:
			formats[format] = true
		case ConvertImagesNone:
			// "none" is exclusive: pairing it with a format contradicts itself
			if len(tokens) > 1 {
				return nil, fmt.Errorf("invalid --convert-images value %q, %q cannot be combined with other formats", value, ConvertImagesNone)
			}
		default:
			return nil, fmt.Errorf("invalid --convert-images format %q, valid formats are %q, %q or %q", token, ConvertAVIF, ConvertWebP, ConvertImagesNone)
		}
	}

	return formats, nil
}
