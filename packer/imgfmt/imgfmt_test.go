// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package imgfmt

import "testing"

// TestSniff pins the shared format sniff that packer's page naming and
// verify-cbz both rely on. These are our magic-byte choices and our AVIF box
// parsing, not upstream codec behaviour, so they belong here.
func TestSniff(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"jpeg", append([]byte{0xff, 0xd8, 0xff, 0xe0}, []byte("jpeg-data")...), "jpg"},
		{"png", append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, []byte("png-data")...), "png"},
		{"gif", []byte("GIF89a-net-scape"), "gif"},
		{"webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8 stub"), "webp"},
		{"avif", append([]byte{0x00, 0x00, 0x00, 0x1c, 'f', 't', 'y', 'p', 'a', 'v', 'i', 'f'}, []byte("avif-data")...), "avif"},
		{"avif sequence brand", append([]byte{0x00, 0x00, 0x00, 0x1c, 'f', 't', 'y', 'p', 'a', 'v', 'i', 's'}, []byte("avis-data")...), "avif"},
		{"unknown", []byte("not a recognisable image"), ""},
		{"empty", []byte{}, ""},
		{"truncated ftyp box", []byte("\x00\x00\x00\x1cftyp"), ""}, // ftyp but no brand yet (< 12 bytes)
	}
	for _, c := range cases {
		if got := Sniff(c.data); got != c.want {
			t.Errorf("Sniff(%s) = %q, want %q", c.name, got, c.want)
		}
	}
}
