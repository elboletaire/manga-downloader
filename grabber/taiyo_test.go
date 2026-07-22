// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

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
