// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

const toonkotsuSeriesPath = "/cdn/v1/series/vuymw7"

// toonkotsuRoutes serves the Momogusa-san fixtures: the series endpoint and
// the release endpoint of the chapter 37 KDT SCANS release
func toonkotsuRoutes() map[string]string {
	return map[string]string{
		toonkotsuSeriesPath:          "toonkotsu_series.json",
		"/api/manga/chapter/4972384": "toonkotsu_chapter.json",
	}
}

func TestToonkotsuTest(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://toonkotsu.com/title/vuymw7-momogusa-san-ga-monogusa-sugiru", true},
		{"https://toonkotsu.com/title/vuymw7-momogusa-san-ga-monogusa-sugiru/4972384-chapter-37/1", true},
		{"https://www.toonkotsu.com/title/vuymw7-momogusa-san-ga-monogusa-sugiru", true},
		{"https://comix.to/title/d9l83-x", false},
		{"https://example.com/title/vuymw7-x", false},
	}
	for _, c := range cases {
		// the host alone decides, so no request is made
		tk := NewToonkotsu(&Grabber{URL: c.url})
		got, err := tk.Test()
		if err != nil {
			t.Fatalf("Test(%s): %v", c.url, err)
		}
		if got != c.want {
			t.Errorf("Test(%s) = %v, want %v", c.url, got, c.want)
		}
	}
}

func TestToonkotsuSeriesCode(t *testing.T) {
	cases := []struct {
		url, origin, code string
	}{
		{"https://toonkotsu.com/title/vuymw7-momogusa-san-ga-monogusa-sugiru", "https://toonkotsu.com", "vuymw7"},
		{"https://toonkotsu.com/title/vuymw7-momogusa-san-ga-monogusa-sugiru/4972384-chapter-37/1", "https://toonkotsu.com", "vuymw7"},
		{"https://toonkotsu.com/title/vuymw7-momogusa-san-ga-monogusa-sugiru/4972384-chapter-37", "https://toonkotsu.com", "vuymw7"},
		{"https://toonkotsu.com/title/vuymw7-momogusa-san-ga-monogusa-sugiru?x=1#top", "https://toonkotsu.com", "vuymw7"},
		{"https://www.toonkotsu.com/title/vuymw7-momogusa-san-ga-monogusa-sugiru", "https://toonkotsu.com", "vuymw7"},
	}
	for _, c := range cases {
		origin, code, err := toonkotsuSeriesCode(c.url)
		if err != nil {
			t.Fatalf("toonkotsuSeriesCode(%s): %v", c.url, err)
		}
		if origin != c.origin || code != c.code {
			t.Errorf("toonkotsuSeriesCode(%s) = %q, %q; want %q, %q", c.url, origin, code, c.origin, c.code)
		}
	}

	if _, _, err := toonkotsuSeriesCode("https://toonkotsu.com/groups"); err == nil {
		t.Error("toonkotsuSeriesCode(/groups) returned no error")
	}
}

func TestToonkotsuFetchChapters(t *testing.T) {
	base, hits := newFixtureServer(t, "https://toonkotsu.com", toonkotsuRoutes())
	tk := NewToonkotsu(&Grabber{URL: base + "/title/vuymw7-momogusa-san-ga-monogusa-sugiru"})

	title, err := tk.FetchTitle()
	if err != nil {
		t.Fatalf("FetchTitle: %v", err)
	}
	if want := "Momogusa-san ga Monogusa Sugiru!!"; title != want {
		t.Errorf("FetchTitle() = %q, want %q", title, want)
	}

	chapters, errs := tk.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters: %v", errs)
	}
	if n := hits.count(toonkotsuSeriesPath); n != 1 {
		t.Errorf("series fetched %d times, want 1 (cached)", n)
	}

	// one chapter per release: each row, then its alts
	cases := []struct {
		id     string
		number float64
		volume int
		groups []string
		label  string
	}{
		{"4972384", 37, 0, []string{"KDT SCANS"}, "Chapter 37"},
		{"4972755", 37, 0, []string{"KDT SCANS"}, "Chapter 37"},
		{"1841597", 1, 1, []string{"Vikala Scans"}, "Chapter 1"},
		{"20025", 1, 1, []string{"KDT SCANS"}, "Chapter 1"},
		{"3277279", 1, 1, []string{"Vikala Scans"}, "Chapter 1"},
		{"4578328", 1133, 111, []string{"Official"}, "Chapter 1133 - Nice Things"},
		{"4575274", 0, 0, []string{"Official"}, "Side Story 15"},
	}
	if len(chapters) != len(cases) {
		t.Fatalf("got %d chapters, want %d", len(chapters), len(cases))
	}
	for i, c := range cases {
		chap := chapters[i].(*ToonkotsuChapter)
		if chap.ID != c.id || chap.Number != c.number || chap.Volume != c.volume {
			t.Errorf("chapter %d = {ID %q, Number %v, Volume %d}, want {%q, %v, %d}",
				i, chap.ID, chap.Number, chap.Volume, c.id, c.number, c.volume)
		}
		if got := chap.GetGroups(); strings.Join(got, "|") != strings.Join(c.groups, "|") {
			t.Errorf("chapter %d groups = %q, want %q", i, got, c.groups)
		}
		if chap.GetLanguage() != "en" {
			t.Errorf("chapter %d language = %q, want en", i, chap.GetLanguage())
		}
		if label := ChapterLabel(chap); label != c.label {
			t.Errorf("chapter %d label = %q, want %q", i, label, c.label)
		}
	}
}

func TestToonkotsuFetchChapter(t *testing.T) {
	base, _ := newFixtureServer(t, "https://toonkotsu.com", toonkotsuRoutes())
	tk := NewToonkotsu(&Grabber{URL: base + "/title/vuymw7-momogusa-san-ga-monogusa-sugiru"})

	chapters, _ := tk.FetchChapters()
	chap, err := tk.FetchChapter(chapters[0])
	if err != nil {
		t.Fatalf("FetchChapter: %v", err)
	}

	if chap.ID != "4972384" || chap.Language != "en" || chap.Group != "KDT SCANS" {
		t.Errorf("chapter = {ID %q, Language %q, Group %q}, want {4972384, en, KDT SCANS}", chap.ID, chap.Language, chap.Group)
	}
	if chap.PagesCount != 2 || len(chap.Pages) != 2 {
		t.Fatalf("got %d pages (count %d), want 2", len(chap.Pages), chap.PagesCount)
	}

	encrypted := base + "/cdn/e/0WqJsHg0hTo_0WKXXQZZJgZ8sb-hcobteaw8OnDUU4k.bin"
	if chap.Pages[0].URL != encrypted {
		t.Errorf("page 1 URL = %q, want %q", chap.Pages[0].URL, encrypted)
	}
	if chap.Pages[0].Transform == nil {
		t.Error("encrypted page has no Transform")
	}

	plain := base + "/cdn/e/plain.bin"
	if chap.Pages[1].URL != plain {
		t.Errorf("page 2 URL = %q, want %q", chap.Pages[1].URL, plain)
	}
	if chap.Pages[1].Transform != nil {
		t.Error("page without a key has a Transform")
	}
}

func TestToonkotsuDecrypt(t *testing.T) {
	// page 1 of release 4972384: the key and nonce as the API sends them, the
	// first 32 bytes of the encrypted image and of the JPEG it decrypts to
	const (
		k          = "Vx8c_G3sgzftrbsBrPPvgI1BvtbdAnWuhtY7SRkfPCI"
		n          = "TvxgPYpcy2E"
		ciphertext = "a170a02f91d437d68a8b97d184e69dc8c4d42f3284e22b783dbbfa85e590f6c7"
		plaintext  = "ffd8ffe000104a46494600010100000100010000ffdb00430008060607060508"
	)
	in, _ := hex.DecodeString(ciphertext)
	want, _ := hex.DecodeString(plaintext)

	decrypt, err := toonkotsuDecrypter(k, n)
	if err != nil {
		t.Fatalf("toonkotsuDecrypter: %v", err)
	}
	for range 2 {
		// a retry reruns the Transform, which must give the same bytes again
		got, err := decrypt(in)
		if err != nil {
			t.Fatalf("decrypt: %v", err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("decrypt() = %x, want %x", got, want)
		}
	}

	if _, err := toonkotsuDecrypter("AAAA", n); err == nil {
		t.Error("a 3-byte key didn't error")
	}
	if _, err := toonkotsuDecrypter(k, "AAAA"); err == nil {
		t.Error("a 3-byte nonce didn't error")
	}
}

func TestToonkotsuTitle(t *testing.T) {
	cases := []struct {
		label, want string
	}{
		{"Chapter 37", ""},
		{"Chapter 35.5", ""},
		{"Chapter 1133: Nice Things", "Nice Things"},
		{"Chapter 1133 - Volume 111", ""},
		{"Chapter 40.6 - Volume 15", ""},
		{"ROMANCE DAWN –Dawn of Adventure", "ROMANCE DAWN –Dawn of Adventure"},
		{"Side Story 15", "Side Story 15"},
	}
	for _, c := range cases {
		if got := toonkotsuTitle(c.label); got != c.want {
			t.Errorf("toonkotsuTitle(%q) = %q, want %q", c.label, got, c.want)
		}
	}
}
