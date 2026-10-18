// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// lrsScrambleForTest builds the scrambled grid the reader unscrambles: cell k
// of the result is the original's cell shuffle[k], mirrored by turns[k]
func lrsScrambleForTest(src *image.RGBA, p lrsPage) *image.RGBA {
	b := src.Bounds()
	tileW, tileH := b.Dx()/p.cols, b.Dy()/p.rows
	out := image.NewRGBA(b)
	n := p.cols * p.rows
	shuffle := lrsPermutation(n, p.shuffleSeed)
	turns := lrsTurns(n, p.transformSeed)
	for k := range n {
		fromX, fromY := (shuffle[k]%p.cols)*tileW, (shuffle[k]/p.cols)*tileH
		toX, toY := (k%p.cols)*tileW, (k/p.cols)*tileH
		for y := range tileH {
			sy := y
			if turns[k] == 2 || turns[k] == 3 {
				sy = tileH - 1 - y
			}
			for x := range tileW {
				sx := x
				if turns[k] == 1 || turns[k] == 3 {
					sx = tileW - 1 - x
				}
				di := out.PixOffset(toX+x, toY+y)
				si := src.PixOffset(fromX+sx, fromY+sy)
				copy(out.Pix[di:di+4], src.Pix[si:si+4])
			}
		}
	}
	return out
}

// lrsSynthetic is a test image whose every pixel is distinct enough to tell
// tiles apart
func lrsSynthetic(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{uint8(x*7 + y), uint8(y*3 + x/2), uint8(x ^ y), 255})
		}
	}
	return img
}

func TestLRSUnscrambleRoundTrip(t *testing.T) {
	cases := []lrsPage{
		{cols: 4, rows: 5, shuffleSeed: 0x9e3779b9, transformSeed: 0x7f4a7c15},
		{cols: 3, rows: 1, shuffleSeed: 1, transformSeed: 2},
		{cols: 1, rows: 1, shuffleSeed: 3, transformSeed: 4},
	}
	for _, p := range cases {
		t.Run(fmt.Sprintf("%dx%d", p.cols, p.rows), func(t *testing.T) {
			orig := lrsSynthetic(p.cols*15, p.rows*16)
			scrambled := lrsScrambleForTest(orig, p)

			buf := &bytes.Buffer{}
			if err := png.Encode(buf, scrambled); err != nil {
				t.Fatal(err)
			}
			data, err := lrsUnscramble(buf.Bytes(), p)
			if err != nil {
				t.Fatalf("lrsUnscramble() error = %v", err)
			}
			got, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(toRGBA(got).Pix, orig.Pix) {
				t.Error("unscrambled image differs from the original")
			}
		})
	}
}

// toRGBA copies any image into an *image.RGBA over the same bounds
func toRGBA(src image.Image) *image.RGBA {
	dst := image.NewRGBA(src.Bounds())
	for y := src.Bounds().Min.Y; y < src.Bounds().Max.Y; y++ {
		for x := src.Bounds().Min.X; x < src.Bounds().Max.X; x++ {
			dst.Set(x, y, src.At(x, y))
		}
	}
	return dst
}

// lrsEncryptForTest builds an inline payload the way the reader's script
// expects it: the key is the obs bytes minus offset, the JSON is XOR'd over
// it cyclically and both are base64 encoded
func lrsEncryptForTest(t *testing.T, key string, offset int, payload any) string {
	t.Helper()
	obsRaw := make([]byte, len(key))
	for i := range key {
		obsRaw[i] = key[i] + byte(offset)
	}
	js, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	enc := make([]byte, len(js))
	for i := range js {
		enc[i] = js[i] ^ key[i%len(key)]
	}
	return fmt.Sprintf(`<script>
    (function(){
      var obs = atob("%s");
      var offset = %d;
      var enc = "%s";
      window.lrsChapter = JSON.parse(decrypt(enc, key));
    })();
    </script>`, base64.StdEncoding.EncodeToString(obsRaw), offset, base64.StdEncoding.EncodeToString(enc))
}

func TestLRSScrambles(t *testing.T) {
	const scrambledKey = "154f05f80ec137380599"
	payload := map[string]any{
		"pages": map[string]any{
			scrambledKey:           map[string]any{"cols": 4, "rows": 31, "w": 800, "h": 9403, "scrambled": true},
			"26fe1fdfebc3d9ca1e6b": map[string]any{"cols": 0, "rows": 0, "scrambled": false},
		},
	}
	html := lrsEncryptForTest(t, "VG5BOIF7Uj5rfE", 8, payload)

	got := lrsScrambles(html)
	want := map[string]lrsPage{
		scrambledKey: {cols: 4, rows: 31, shuffleSeed: 0x154f05f8, transformSeed: 0x0ec13738},
	}
	if len(got) != len(want) || got[scrambledKey] != want[scrambledKey] {
		t.Errorf("lrsScrambles() = %+v, want %+v", got, want)
	}

	if s, ok := lrsFor(got, "https://bloomscans.com/reader-image/15693/"+scrambledKey+"/96b313877effcabebc130597.webp"); !ok || s != want[scrambledKey] {
		t.Errorf("lrsFor() = %+v, %v, want the scrambled page", s, ok)
	}
	if _, ok := lrsFor(got, "https://bloomscans.com/reader-image/15693/26fe1fdfebc3d9ca1e6b/x.webp"); ok {
		t.Error("lrsFor() matched a page the payload doesn't scramble")
	}
	if lrsScrambles("<html></html>") != nil {
		t.Error("lrsScrambles() found a payload in a page without one")
	}
}

func TestLRSTransformKeepsUndecodablePage(t *testing.T) {
	junk := []byte("not an image")
	got, err := lrsTransform(lrsPage{cols: 2, rows: 2})(junk)
	if err != nil {
		t.Fatalf("lrsTransform() error = %v, want the page kept without a retry", err)
	}
	if !bytes.Equal(got, junk) {
		t.Error("lrsTransform() changed a page it couldn't decode")
	}
}
