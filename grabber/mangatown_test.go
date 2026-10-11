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

	for _, c := range cases {
		if got := mangatownChapterTitle(series, c.text, c.number); got != c.want {
			t.Errorf("mangatownChapterTitle(%q) = %q, want %q", c.text, got, c.want)
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
