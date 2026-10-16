// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// The fixture is `openssl enc -aes-256-cbc -md md5 -S 0123456789abcdef
// -pass pass:fb087b0bc9` over a JSON string holding a JSON array.
func protectorFixtureScript(t *testing.T, nonce string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/wpmangaprotector.json")
	if err != nil {
		t.Fatal(err)
	}
	return "var chapter_data='" + strings.TrimSpace(string(b)) + "';var chapter_image_gap='on';var wpmangaprotectornonce='" + nonce + "';"
}

func TestDecodeWPMangaProtector(t *testing.T) {
	got, err := decodeWPMangaProtector(protectorFixtureScript(t, "fb087b0bc9"))
	if err != nil {
		t.Fatal(err)
	}
	// notice pages go, the credits page stays
	want := []string{"https://x.test/a/1.jpg", "https://x.test/a/0-HPM10-cp.jpg"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDecodeWPMangaProtectorWrongNonce(t *testing.T) {
	if _, err := decodeWPMangaProtector(protectorFixtureScript(t, "0000000000")); err == nil {
		t.Error("expected an error for the wrong passphrase")
	}
}

func TestWPMangaProtectorImages(t *testing.T) {
	html := `<html><body><script id="chapter-protector-data">` + protectorFixtureScript(t, "fb087b0bc9") + `</script></body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	urls, ok, err := wpMangaProtectorImages(doc)
	if err != nil || !ok || len(urls) != 2 {
		t.Errorf("got %v %v %v", urls, ok, err)
	}

	plain, _ := goquery.NewDocumentFromReader(strings.NewReader(`<html><body></body></html>`))
	if _, ok, _ := wpMangaProtectorImages(plain); ok {
		t.Error("a page without the script must not be claimed")
	}
}
