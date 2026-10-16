// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestMangatownTest(t *testing.T) {
	cases := []struct {
		url     string
		want    bool
		wantURL string
	}{
		{"https://www.mangatown.com/manga/koi_wa_amaagari_no_you_ni/", true, "https://www.mangatown.com/manga/koi_wa_amaagari_no_you_ni/"},
		{"http://mangatown.com/manga/koi_wa_amaagari_no_you_ni/", true, "https://www.mangatown.com/manga/koi_wa_amaagari_no_you_ni/"},
		{"https://m.mangatown.com/manga/koi_wa_amaagari_no_you_ni/", true, "https://www.mangatown.com/manga/koi_wa_amaagari_no_you_ni/"},
		{"https://m.mangatown.com/manga/koi_wa_amaagari_no_you_ni/c010/", true, "https://www.mangatown.com/manga/koi_wa_amaagari_no_you_ni/"},
		{"https://m.mangatown.com", true, "https://www.mangatown.com"},
		// chapter and reader page URLs resolve to their series
		{"https://www.mangatown.com/manga/koi_wa_amaagari_no_you_ni/c010/", true, "https://www.mangatown.com/manga/koi_wa_amaagari_no_you_ni/"},
		{"https://www.mangatown.com/manga/koi_wa_amaagari_no_you_ni/c010/2.html", true, "https://www.mangatown.com/manga/koi_wa_amaagari_no_you_ni/"},
		{"https://m.mangatown.com/manga/koi_wa_amaagari_no_you_ni/v01/c002/2.html", true, "https://www.mangatown.com/manga/koi_wa_amaagari_no_you_ni/"},
		{"https://www.mangatown.com/manga/one_piece/v98/c1196", true, "https://www.mangatown.com/manga/one_piece/"},
		{"https://www.mangahere.cc/manga/kengan_omega/", false, "https://www.mangahere.cc/manga/kengan_omega/"},
		{"https://mangatown.com.example.org/manga/x/", false, "https://mangatown.com.example.org/manga/x/"},
	}

	for _, c := range cases {
		m := Mangatown{Grabber: &Grabber{URL: c.url}}
		got, err := m.Test()
		if err != nil {
			t.Errorf("Test(%q) unexpected error: %v", c.url, err)
			continue
		}
		if got != c.want {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.want)
		}
		if m.URL != c.wantURL {
			t.Errorf("Test(%q) left URL %q, want %q", c.url, m.URL, c.wantURL)
		}
	}
}

func TestMangatownChapterNumber(t *testing.T) {
	cases := []struct {
		href string
		want float64
		ok   bool
	}{
		{"/manga/koi_wa_amaagari_no_you_ni/c082/", 82, true},
		{"/manga/koi_wa_amaagari_no_you_ni/c083", 83, true},
		{"/manga/koi_wa_amaagari_no_you_ni/c000/", 0, true},
		{"/manga/shingeki_no_kyojin/c009.5/", 9.5, true},
		{"/manga/bleach/v03/c025.2/", 25.2, true},
		{"/manga/one_piece/v98/c1196", 1196, true},
		{"/manga/bleach/v01/c001/", 1, true},
		{"/manga/bleach/comments", 0, false},
		{"/manga/koi_wa_amaagari_no_you_ni/c001/2.html", 0, false},
	}

	for _, c := range cases {
		got, ok := mangatownChapterNumber(c.href)
		if ok != c.ok || got != c.want {
			t.Errorf("mangatownChapterNumber(%q) = %v, %v; want %v, %v", c.href, got, ok, c.want, c.ok)
		}
	}
}

func TestMangatownChapterTitle(t *testing.T) {
	const series = "Koi wa Amaagari no You ni"
	cases := []struct {
		text   string
		number float64
		want   string
	}{
		{"Koi wa Amaagari no You ni 10", 10, "Chapter 10"},
		{"koi wa amaagari no you ni  10.5 ", 10.5, "Chapter 10.5"},
		{"Koi wa Amaagari no You ni 10 Extra Story", 10, "10 Extra Story"},
		{"Chapter 3", 3, "Chapter 3"},
	}

	// want is the label, as the title used to read before labels were composed
	for _, c := range cases {
		ch := Chapter{Title: mangatownChapterTitle(series, c.text), Number: c.number, Label: Label{Style: LabelNumbered}}
		if got := ChapterLabel(ch); got != c.want {
			t.Errorf("label of %q = %q, want %q", c.text, got, c.want)
		}
	}
}

// TestMangahereChapterURL covers mangahereChapterURL (shared by mangahere and
// fanfox). The function is path-only, so domain doesn't affect the result.
func TestMangahereChapterURL(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		// mangahere.cc
		{"https://www.mangahere.cc/manga/kengan_omega/c369/1.html", true},
		{"https://www.mangahere.cc/manga/one_piece/v98/c1195/1.html", true},
		{"https://www.mangahere.cc/manga/one_piece/v00/c001.1/1.html", true},
		{"https://mangahere.cc/manga/kengan_omega/c369/1.html", true},
		{"https://m.mangahere.cc/manga/a_villainous_baby_killer_whale/c116/1.html", true},
		{"https://newm.mangahere.cc/manga/hinako_note/c034/1.html", true},
		// fanfox.net uses the same path format
		{"https://fanfox.net/manga/chainsaw_man/c228/1.html", true},
		{"https://fanfox.net/manga/onepunch_man/vTBD/c237/1.html", true},
		{"https://fanfox.net/manga/one_piece/v00/c001.1/1.html", true},
		{"https://www.fanfox.net/manga/chainsaw_man/c228/1.html", true},
		// series URLs — must return false
		{"https://www.mangahere.cc/manga/kengan_omega/", false},
		{"https://www.mangahere.cc/manga/kengan_omega", false},
		{"https://fanfox.net/manga/chainsaw_man/", false},
		{"https://fanfox.net/manga/chainsaw_man", false},
		// path without /manga/ prefix
		{"https://www.mangahere.cc/other/kengan_omega/c1/1.html", false},
		{"https://fanfox.net/other/chainsaw_man/c1/", false},
	}
	for _, c := range cases {
		if got := mangahereChapterURL(c.url); got != c.want {
			t.Errorf("mangahereChapterURL(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

// TestMangahereSeriesURL covers mangahereSeriesURL (shared by mangahere and
// fanfox). Any *.mangahere.cc host is normalised to www.mangahere.cc; fanfox
// hosts are passed through unchanged.
func TestMangahereSeriesURL(t *testing.T) {
	cases := []struct {
		url     string
		wantURL string
		wantOK  bool
	}{
		// mangahere.cc — www. preserved
		{"https://www.mangahere.cc/manga/kengan_omega/c369/1.html", "https://www.mangahere.cc/manga/kengan_omega/", true},
		{"https://www.mangahere.cc/manga/one_piece/v98/c1195/1.html", "https://www.mangahere.cc/manga/one_piece/", true},
		{"https://www.mangahere.cc/manga/one_piece/v00/c001.1/1.html", "https://www.mangahere.cc/manga/one_piece/", true},
		// bare and mobile hosts rewritten to www.mangahere.cc
		{"https://mangahere.cc/manga/kengan_omega/c369/1.html", "https://www.mangahere.cc/manga/kengan_omega/", true},
		{"https://m.mangahere.cc/manga/a_villainous_baby_killer_whale/c116/1.html", "https://www.mangahere.cc/manga/a_villainous_baby_killer_whale/", true},
		{"https://newm.mangahere.cc/manga/hinako_note/c034/1.html", "https://www.mangahere.cc/manga/hinako_note/", true},
		// fanfox.net — host unchanged
		{"https://fanfox.net/manga/chainsaw_man/c228/1.html", "https://fanfox.net/manga/chainsaw_man/", true},
		{"https://fanfox.net/manga/onepunch_man/vTBD/c237/1.html", "https://fanfox.net/manga/onepunch_man/", true},
		{"https://fanfox.net/manga/one_piece/v00/c001.1/1.html", "https://fanfox.net/manga/one_piece/", true},
		{"https://www.fanfox.net/manga/chainsaw_man/c228/1.html", "https://www.fanfox.net/manga/chainsaw_man/", true},
		// series URLs — no-op
		{"https://www.mangahere.cc/manga/kengan_omega/", "", false},
		{"https://www.mangahere.cc/manga/kengan_omega", "", false},
		{"https://fanfox.net/manga/chainsaw_man/", "", false},
		{"https://fanfox.net/manga/chainsaw_man", "", false},
	}
	for _, c := range cases {
		got, ok := mangahereSeriesURL(c.url)
		if ok != c.wantOK || got != c.wantURL {
			t.Errorf("mangahereSeriesURL(%q) = (%q, %v), want (%q, %v)", c.url, got, ok, c.wantURL, c.wantOK)
		}
	}
}

// reader page markup as served by mangatown.com (image attributes in the
// same order as the site, plus a page whose <img> carries no src)
const (
	mangatownSamplePage  = `<div class="page_select"><img src="//zjcdn.mangahere.org/store/manga/18495/010.0/compressed/c002.jpg" id="image" alt="Koi wa Amaagari no You ni 10 Page 2"   style="width:800px"></div>`
	mangatownSampleNoImg = `<div><img style="display:none" class="reader-main-img"/></div>`
)

func TestMangatownPageImage(t *testing.T) {
	got, err := mangatownPageImage(mangatownSamplePage)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "https://zjcdn.mangahere.org/store/manga/18495/010.0/compressed/c002.jpg"
	if got != want {
		t.Errorf("mangatownPageImage() = %q, want %q", got, want)
	}

	if _, err := mangatownPageImage(mangatownSampleNoImg); err == nil {
		t.Error("expected an error for a page without #image, got nil")
	}
}
