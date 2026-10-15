// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

// hijalaSeriesFixture is the seroval tail of en-hijala.com's series page,
// trimmed to the series object and three of its 252 chapters. The count
// stays at the page's 252, as parseTanstackSeries does not check it.
const hijalaSeriesFixture = `$R[52]={id:11,slug:"double-click",postTitle:"Double Click",postType:"SERIES",_count:$R[57]={bookmarks:4,ratings:0,chapters:252,covers:0}},initialChapters:$R[59]=[$R[60]={id:5935,slug:"chapter-252",number:252,title:null,featuredImage:null,createdAt:"2026-07-12T21:50:29.125Z",updatedAt:"2026-07-12T21:50:29.125Z",mangaPostId:11,unlockAt:null,becameFreeAt:null,isPermanentlyLocked:!1,price:0,isShortLinkLocked:!1,isLocked:!1,likesCount:0,mangaPost:$R[61]={id:void 0,slug:"double-click",featuredImage:"https://storage.en-hijala.com/upload/2025/06/24/9672b20b-a60a-47b9-aca7-d989fcabac4c.webp",saleActive:!1,salePercentage:null,saleEndDate:null}},$R[62]={id:5934,slug:"chapter-251",number:251,title:null,featuredImage:null,createdAt:"2026-07-12T21:50:28.923Z",updatedAt:"2026-07-12T21:50:28.923Z",mangaPostId:11,unlockAt:null,becameFreeAt:null,isPermanentlyLocked:!1,price:0,isShortLinkLocked:!1,isLocked:!1,likesCount:0,mangaPost:$R[63]={id:void 0,slug:"double-click",featuredImage:"https://storage.en-hijala.com/upload/2025/06/24/9672b20b-a60a-47b9-aca7-d989fcabac4c.webp",saleActive:!1,salePercentage:null,saleEndDate:null}},$R[562]={id:466,slug:"chapter-1",number:1,title:"",featuredImage:"https://storage.en-hijala.com/upload/2025/06/27/Double-Click_episode_1_3e5175f3.webp",createdAt:"2025-06-27T16:57:56.743Z",updatedAt:"2026-04-01T18:43:11.786Z",mangaPostId:11,unlockAt:null,becameFreeAt:null,isPermanentlyLocked:!1,price:0,isShortLinkLocked:!1,isLocked:!1,likesCount:0,mangaPost:$R[563]={id:void 0,slug:"double-click",featuredImage:"https://storage.en-hijala.com/upload/2025/06/24/9672b20b-a60a-47b9-aca7-d989fcabac4c.webp",saleActive:!1,salePercentage:null,saleEndDate:null}}],totalChapterCount:252,firstChapter:$R[564]={number:1,slug:"chapter-1"}`

func TestHijalaTest(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://en-hijala.com/series/double-click", true},
		{"https://EN-HIJALA.COM/series/double-click", true},
		{"https://En-Hijala.Com/series/double-click", true},
		{"https://www.en-hijala.com/series/double-click", true},
		{"https://othermanga.com/series/double-click", false},
	}
	for _, c := range cases {
		h := NewHijala(&Grabber{URL: c.url})
		got, _ := h.Test()
		if got != c.want {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

func TestHijalaParseSeries(t *testing.T) {
	series, err := parseTanstackSeries(hijalaSeriesFixture)
	if err != nil {
		t.Fatalf("parseTanstackSeries: %v", err)
	}
	if series.Title != "Double Click" {
		t.Errorf("Title = %q, want %q", series.Title, "Double Click")
	}
	if series.TotalChapterCount != 252 {
		t.Errorf("TotalChapterCount = %d, want 252", series.TotalChapterCount)
	}
	want := []tanstackChapter{
		{Number: 252, Slug: "chapter-252"},
		{Number: 251, Slug: "chapter-251"},
		{Number: 1, Slug: "chapter-1"},
	}
	if len(series.Chapters) != len(want) {
		t.Fatalf("got %d chapters, want %d", len(series.Chapters), len(want))
	}
	for i, w := range want {
		if got := series.Chapters[i]; got.Number != w.Number || got.Slug != w.Slug {
			t.Errorf("chapter %d = %+v, want number %v slug %q", i, got, w.Number, w.Slug)
		}
	}
}

func TestHijalaResolveSeriesURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// chapter URLs: strip the /chapter-N suffix
		{"https://en-hijala.com/series/double-click/chapter-248", "https://en-hijala.com/series/double-click"},
		{"https://en-hijala.com/series/my-series/chapter-1", "https://en-hijala.com/series/my-series"},
		{"https://en-hijala.com/series/my-series/chapter-10.5", "https://en-hijala.com/series/my-series"},
		{"https://en-hijala.com/series/my-series/chapter-10.5/", "https://en-hijala.com/series/my-series"},
		// series URLs: unchanged
		{"https://en-hijala.com/series/double-click", "https://en-hijala.com/series/double-click"},
		{"https://en-hijala.com/series/double-click/", "https://en-hijala.com/series/double-click/"},
		// query and fragment on chapter URL: stripped
		{"https://en-hijala.com/series/double-click/chapter-248?x=1", "https://en-hijala.com/series/double-click"},
		{"https://en-hijala.com/series/double-click/chapter-1#page2", "https://en-hijala.com/series/double-click"},
		// www prefix, query, fragment on series URL: unchanged (no chapter suffix)
		{"https://www.en-hijala.com/series/double-click?foo=bar", "https://www.en-hijala.com/series/double-click?foo=bar"},
		// uppercase host: chapter stripped
		{"https://EN-HIJALA.COM/series/double-click/chapter-248", "https://EN-HIJALA.COM/series/double-click"},
	}

	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			h := NewHijala(&Grabber{URL: c.in})
			h.resolveSeriesURL()
			if h.URL != c.want {
				t.Errorf("resolveSeriesURL(%q) URL = %q, want %q", c.in, h.URL, c.want)
			}
		})
	}
}
