// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"testing"
)

func TestMangamoinsScanSlug(t *testing.T) {
	cases := []struct {
		input string
		want  string
		ok    bool
	}{
		{"https://mangamoins.com/scan/OP01", "OP01", true},
		{"https://www.mangamoins.com/scan/G172_2?x=1#a", "G172_2", true},
		{"https://MANGAMOINS.com/Scan/OP1150/", "OP1150", true},
		{"https://mangamoins.com/manga/one_piece", "", false},
		{"https://example.com/scan/OP01", "", false},
	}

	for _, c := range cases {
		got, ok := mangamoinsScanSlug(c.input)
		if got != c.want || ok != c.ok {
			t.Errorf("mangamoinsScanSlug(%q) = %q, %v; want %q, %v", c.input, got, ok, c.want, c.ok)
		}
	}
}

func TestParseMangamoinsSeries(t *testing.T) {
	body := `<html><body>
<h1 id="manga-title">One Piece</h1>
<a class="manga-detail__btn" href="/scan/OP01">Premier chapitre (1)</a>
<a class="manga-detail__btn" href="/scan/OP1200">Dernier chapitre (1200)</a>
<a class="manga-detail__chapter " href="/scan/OP1200">
  <span class="manga-detail__chapter-num">1200</span>
  <span class="manga-detail__chapter-title">La lettre de Lili</span>
</a>
<a class="manga-detail__chapter " href="/scan/G172_2">
  <span class="manga-detail__chapter-num">172.2</span>
  <span class="manga-detail__chapter-title">Étranges affinités</span>
</a>
<a class="manga-detail__chapter " href="/scan/OP01">
  <span class="manga-detail__chapter-num">1</span>
  <span class="manga-detail__chapter-title">Romance Dawn - À l&#39;aube d&#39;une grande aventure</span>
</a>
</body></html>`

	series, err := parseMangamoinsSeries(body)
	if err != nil {
		t.Fatal(err)
	}

	if series.title != "One Piece" {
		t.Errorf("title = %q, want One Piece", series.title)
	}
	if len(series.chapters) != 3 {
		t.Fatalf("got %d chapters, want 3", len(series.chapters))
	}

	want := []mangamoinsListed{
		{slug: "OP1200", number: "1200", title: "La lettre de Lili"},
		{slug: "G172_2", number: "172.2", title: "Étranges affinités"},
		{slug: "OP01", number: "1", title: "Romance Dawn - À l'aube d'une grande aventure"},
	}
	for i, w := range want {
		if series.chapters[i] != w {
			t.Errorf("chapter %d = %+v, want %+v", i, series.chapters[i], w)
		}
	}
}

func TestParseMangamoinsSeriesNoChapters(t *testing.T) {
	if _, err := parseMangamoinsSeries(`<h1 id="manga-title">One Piece</h1>`); err == nil {
		t.Error("expected an error for a series page with no chapters")
	}
}

func TestMangamoinsFetchChaptersDecimal(t *testing.T) {
	m := NewMangamoins(&Grabber{URL: "https://mangamoins.com/manga/gachiakuta", Settings: &Settings{}})
	m.resolved = true
	m.series = &mangamoinsSeries{
		title:    "Gachiakuta",
		chapters: []mangamoinsListed{{slug: "G172_2", number: "172.2", title: "Étranges affinités"}},
	}

	chapters, errs := m.FetchChapters()
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	if len(chapters) != 1 {
		t.Fatalf("got %d chapters, want 1", len(chapters))
	}

	c := chapters[0].(*MangamoinsChapter)
	if c.Number != 172.2 {
		t.Errorf("Number = %v, want 172.2", c.Number)
	}
	if c.ID != "G172_2" || c.Slug != "G172_2" {
		t.Errorf("ID, Slug = %q, %q; want G172_2", c.ID, c.Slug)
	}
	if c.Language != "fr" {
		t.Errorf("Language = %q, want fr", c.Language)
	}
	if c.Label.Number != "172.2" {
		t.Errorf("Label.Number = %q, want 172.2", c.Label.Number)
	}
}

func TestMangamoinsPages(t *testing.T) {
	scan := &mangamoinsScan{
		PageNumbers:  52,
		PagesBaseURL: "https://mangamoinsScans.mangamoins.com/56bcdb0cd39e/",
		Manga:        "one_piece",
	}

	pages, err := mangamoinsPages(scan)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 52 {
		t.Fatalf("got %d pages, want 52", len(pages))
	}
	if pages[0].URL != "https://mangamoinsScans.mangamoins.com/56bcdb0cd39e/01.webp" {
		t.Errorf("first page = %q", pages[0].URL)
	}
	if pages[9].URL != "https://mangamoinsScans.mangamoins.com/56bcdb0cd39e/10.webp" {
		t.Errorf("page 10 = %q", pages[9].URL)
	}
	if pages[51].URL != "https://mangamoinsScans.mangamoins.com/56bcdb0cd39e/52.webp" {
		t.Errorf("last page = %q", pages[51].URL)
	}
	if pages[0].Number != 1 {
		t.Errorf("first page Number = %d, want 1", pages[0].Number)
	}

	if _, err := mangamoinsPages(&mangamoinsScan{PagesBaseURL: scan.PagesBaseURL}); err == nil {
		t.Error("expected an error for no pages")
	}
	if _, err := mangamoinsPages(&mangamoinsScan{PageNumbers: 3}); err == nil {
		t.Error("expected an error for no pages base URL")
	}
}
