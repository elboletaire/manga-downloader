// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"testing"
)

func TestTaiyoChapterURL(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		// standard with page number
		{"https://taiyo.moe/chapter/6b95d938-b08e-4f7e-a5bb-0f07db85246a/1", true},
		// no page number (site 307s to /1)
		{"https://taiyo.moe/chapter/6b95d938-b08e-4f7e-a5bb-0f07db85246a", true},
		// trailing slash (no page)
		{"https://taiyo.moe/chapter/6b95d938-b08e-4f7e-a5bb-0f07db85246a/", true},
		// www.
		{"https://www.taiyo.moe/chapter/6b95d938-b08e-4f7e-a5bb-0f07db85246a/1", true},
		// uppercase UUID
		{"https://taiyo.moe/chapter/6B95D938-B08E-4F7E-A5BB-0F07DB85246A/1", true},
		// series URL — must return false
		{"https://taiyo.moe/media/000bdf97-407f-4ca8-95a1-ee2a3114e73a", false},
		// unrelated domain
		{"https://example.com/chapter/6b95d938-b08e-4f7e-a5bb-0f07db85246a/1", false},
	}
	for _, c := range cases {
		if got := taiyoChapterURL(c.url); got != c.want {
			t.Errorf("taiyoChapterURL(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

func TestTaiyoMediaID(t *testing.T) {
	cases := []struct {
		url     string
		want    string
		wantErr bool
	}{
		{"https://taiyo.moe/media/000bdf97-407f-4ca8-95a1-ee2a3114e73a", "000bdf97-407f-4ca8-95a1-ee2a3114e73a", false},
		{"https://taiyo.moe/media/000bdf97-407f-4ca8-95a1-ee2a3114e73a/", "000bdf97-407f-4ca8-95a1-ee2a3114e73a", false},
		{"https://taiyo.moe/", "", true},
	}

	for _, c := range cases {
		taiyo := Taiyo{Grabber: &Grabber{URL: c.url}}
		got, err := taiyo.mediaID()
		if (err != nil) != c.wantErr {
			t.Errorf("mediaID(%q) error = %v, wantErr %v", c.url, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("mediaID(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

func TestTaiyoChapterPages(t *testing.T) {
	// a trimmed-down, escaped-quote snippet mimicking the React Server
	// Components "flight" payload embedded in a reader page's <script> tag
	html := `self.__next_f.push([1,"6:[[\"$\",\"$L19\",null,{\"mediaChapter\":{\"id\":\"0c182743-2367-4ccb-b4ba-ef1cfc33a9d7\",\"title\":\"Alvo das Mulheres (12)\",\"number\":82,\"volume\":7,\"pages\":[{\"id\":\"e0d6d1b6-ea9e-4e49-9208-d7bd09b8d14d\",\"extension\":\"jpg\"},{\"id\":\"148af180-39b9-4951-87c8-41e7e4723773\",\"extension\":\"jpg\"}]}}]"])`

	pages, err := taiyoChapterPages(html)
	if err != nil {
		t.Fatalf("taiyoChapterPages() error = %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("taiyoChapterPages() got %d pages, want 2", len(pages))
	}
	if pages[0].ID != "e0d6d1b6-ea9e-4e49-9208-d7bd09b8d14d" || pages[0].Extension != "jpg" {
		t.Errorf("taiyoChapterPages()[0] = %+v, unexpected", pages[0])
	}
	if pages[1].ID != "148af180-39b9-4951-87c8-41e7e4723773" || pages[1].Extension != "jpg" {
		t.Errorf("taiyoChapterPages()[1] = %+v, unexpected", pages[1])
	}

	if _, err := taiyoChapterPages("no data here"); err == nil {
		t.Error("taiyoChapterPages() with no mediaChapter data should error")
	}
}

func TestTaiyoMediaIDRe(t *testing.T) {
	// Actual RSC payload format: the uuid is followed by more fields, not immediately a closing brace.
	html := `self.__next_f.push([1,"6:[[\"$\",\"$L19\",null,{\"mediaChapter\":{\"id\":\"6b95d938-b08e-4f7e-a5bb-0f07db85246a\",\"title\":\"test\"},\"media\":{\"id\":\"000bdf97-407f-4ca8-95a1-ee2a3114e73a\",\"type\":\"MANGA\"}}]"])`
	m := taiyoMediaIDRe.FindStringSubmatch(html)
	if len(m) < 2 {
		t.Fatal("taiyoMediaIDRe: no match in RSC payload")
	}
	if want := "000bdf97-407f-4ca8-95a1-ee2a3114e73a"; m[1] != want {
		t.Errorf("taiyoMediaIDRe: got %q, want %q", m[1], want)
	}
}

// a chapter translated by two scans, one by a scan and an uploader only, and
// one with no scans: the uploader is never the group
func TestTaiyoReleasesGroups(t *testing.T) {
	const feedJSON = `[{"result":{"data":{"json":{"totalPages":1,"chapters":[
		{"id":"c1","title":"","number":1,"uploader":{"name":"Someone"},"scans":[{"id":"s1","name":"Alpha"},{"id":"s2","name":"Beta"}]},
		{"id":"c2","title":"Two","number":2,"uploader":{"name":"Gamma"},"scans":[{"id":"s3","name":"Gamma"}]},
		{"id":"c3","title":"","number":3,"uploader":{"name":"Delta"},"scans":null}
	]}}}}]`

	var feed taiyoChaptersFeed
	if err := json.Unmarshal([]byte(feedJSON), &feed); err != nil {
		t.Fatalf("decoding feed: %v", err)
	}

	got := taiyoReleases(feed[0].Result.Data.Json.Chapters)
	wants := []struct{ id, group string }{
		{"c1", "Alpha, Beta"},
		{"c2", "Gamma"},
		{"c3", ""},
	}
	if len(got) != len(wants) {
		t.Fatalf("got %d chapters, want %d", len(got), len(wants))
	}
	for i, w := range wants {
		c := got[i].(*TaiyoChapter)
		if c.ID != w.id || c.ChapterID != w.id || c.Group != w.group {
			t.Errorf("chapter[%d] = {ID %q, ChapterID %q, Group %q}, want {%q, %q, %q}",
				i, c.ID, c.ChapterID, c.Group, w.id, w.id, w.group)
		}
	}
}
