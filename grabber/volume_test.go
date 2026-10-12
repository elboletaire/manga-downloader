// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestParseVolume(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"1", 1},
		{"03", 3},
		{" 12 ", 12},
		{"Volume 3", 3},
		{"Vol. 03", 3},
		{"vol.7", 7},
		{"Volumen 2", 2},
		{"v98", 98},
		{"V01", 1},
		{"v00", 0},
		{"0", 0},
		{"vTBD", 0},
		{"none", 0},
		{"", 0},
		{"10.5", 0},
		{"Volume 3: Arc", 0},
	}
	for _, tt := range tests {
		if got := parseVolume(tt.in); got != tt.want {
			t.Errorf("parseVolume(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestURLVolume(t *testing.T) {
	tests := []struct {
		href string
		want int
	}{
		{"https://www.mangahere.cc/manga/one_piece/v98/c1195/1.html", 98},
		{"/manga/one_piece/v98/c1196", 98},
		{"/manga/bleach/v03/c025.2/", 3},
		{"https://fanfox.net/manga/one_piece/v00/c001.1/1.html", 0},
		{"https://www.mangahere.cc/manga/one_piece/vTBD/c1100/1.html", 0},
		{"/manga/koi_wa_amaagari_no_you_ni/c082/", 0},
		{"/manga/v01/comments", 0},
	}
	for _, tt := range tests {
		if got := urlVolume(tt.href); got != tt.want {
			t.Errorf("urlVolume(%q) = %d, want %d", tt.href, got, tt.want)
		}
	}
}

func TestVolumeOf(t *testing.T) {
	if got := VolumeOf(&Chapter{Volume: 4}); got != 4 {
		t.Errorf("VolumeOf(Chapter) = %d, want 4", got)
	}
	if got := VolumeOf(&MangadexChapter{Chapter{Volume: 2}, "id"}); got != 2 {
		t.Errorf("VolumeOf(MangadexChapter) = %d, want 2", got)
	}
}

func TestMangadexFetchChaptersVolumes(t *testing.T) {
	const id = "a1c7c817-4e59-43b7-9365-09675a149a6f"
	mux := http.NewServeMux()
	mux.HandleFunc("/manga/"+id+"/feed", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("offset") != "0" {
			_, _ = w.Write([]byte(`{"data":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[
			{"id":"a","attributes":{"volume":"1","chapter":"1","pages":10}},
			{"id":"b","attributes":{"volume":"01","chapter":"2","pages":10}},
			{"id":"c","attributes":{"volume":"2","chapter":"3","pages":10}},
			{"id":"d","attributes":{"volume":null,"chapter":"4","pages":10}},
			{"id":"e","attributes":{"volume":"none","chapter":"5","pages":10}}
		]}`))
	})
	srv := serve(t, mux)
	overrideBase(t, &mangadexAPI, srv.URL)

	m := NewMangadex(&Grabber{URL: "https://mangadex.org/title/" + id, Settings: &Settings{}})
	chapters, errs := m.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters() errors = %v", errs)
	}

	var got []int
	for _, c := range chapters {
		got = append(got, VolumeOf(c))
	}
	if want := []int{1, 1, 2, 0, 0}; !reflect.DeepEqual(got, want) {
		t.Errorf("volumes = %v, want %v", got, want)
	}
}

func TestTcbFetchChaptersVolumes(t *testing.T) {
	flat := `<ul class="main version-chap">
		<li class="wp-manga-chapter"><a href="https://example.com/manga/x/chapter-2/">Chapter 2</a></li>
		<li class="wp-manga-chapter"><a href="https://example.com/manga/x/chapter-1/">Chapter 1</a></li>
	</ul>`
	titled := `<ul class="main version-chap volumns">
		<li class="parent has-child"><a href="javascript:void(0)" class="has-child">Vol. 4: The arc</a>
			<ul class="sub-chap list-chap"><li><ul class="sub-chap-list">
				<li class="wp-manga-chapter"><a href="https://example.com/manga/x/chapter-30/">Chapter 30</a></li>
			</ul></li></ul>
		</li>
	</ul>`

	tests := []struct {
		name string
		html string
		want []int
	}{
		{"grouped", volumeGroupedChaptersHTML, []int{10, 9, 9}},
		{"flat", flat, []int{0, 0}},
		{"titled header", titled, []int{4}},
	}
	for _, tt := range tests {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(tt.html))
		if err != nil {
			t.Fatal(err)
		}
		tcb := Tcb{
			Grabber: &Grabber{URL: "https://example.com/manga/x/"},
			chaps:   doc.Find("li.wp-manga-chapter"),
		}
		chapters, errs := tcb.FetchChapters()
		if len(errs) > 0 {
			t.Fatalf("%s: errors = %v", tt.name, errs)
		}
		var got []int
		for _, c := range chapters {
			got = append(got, VolumeOf(c))
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: volumes = %v, want %v", tt.name, got, tt.want)
		}
	}
}
