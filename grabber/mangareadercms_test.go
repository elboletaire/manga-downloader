// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"os"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestMangaReaderCMS(t *testing.T) {
	read := func(name string) *goquery.Document {
		b, err := os.ReadFile("testdata/identify/" + name)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(b)))
		if err != nil {
			t.Fatal(err)
		}
		return doc
	}

	var site SiteSelector
	for _, s := range plainHTMLSelectors {
		if s.NumberRe != "" {
			site = s
		}
	}
	series := read("mangareadercms_series.html")
	m := PlainHTML{Grabber: &Grabber{URL: "https://onma.top/manga/black-clover"}, doc: series, rows: series.Find(site.Rows), site: site}

	if title, _ := m.FetchTitle(); title != "Black Clover" {
		t.Errorf("FetchTitle() = %q", title)
	}
	chapters, errs := m.FetchChapters()
	if len(errs) != 0 || len(chapters) != 2 {
		t.Fatalf("FetchChapters() = %d chapters, errors %v; want 2", len(chapters), errs)
	}
	// the number comes from the URL, since the text may lead with a title
	if chapters[0].GetNumber() != 392 || chapters[1].GetNumber() != 391.5 {
		t.Errorf("numbers = %v, %v; want 392, 391.5", chapters[0].GetNumber(), chapters[1].GetNumber())
	}
	if u := chapters[0].(*PlainHTMLChapter).URL; u != "https://onma.top/manga/black-clover/392" {
		t.Errorf("chapter URL = %q (the ad-shortened download link must not be taken)", u)
	}

	chapter := m.chapterFromDoc(&PlainHTMLChapter{Chapter: Chapter{Number: 391}}, read("mangareadercms_chapter.html"))
	if len(chapter.Pages) != 2 || chapter.Pages[0].URL != "https://www.onma.top/uploads/manga/black-clover/chapters/391/01.jpg" {
		t.Errorf("pages = %+v, want the two trimmed data-src URLs", chapter.Pages)
	}
}
