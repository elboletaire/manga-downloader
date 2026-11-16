// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// the series page's placeholder, with the chapter volumes the site writes: a
// string, "" for none, one with spaces around it, and mistyped ones (a decimal, a
// negative, null, a bare number), which must not fail the whole series
const bigsoloTestPage = `<html><body>
<script id="series-data-placeholder" type="application/json">
{"title":"Some Series","chapters":{
	"1":{"title":"One","volume":"1","source":{"service":"imgchest","id":"a1"}},
	"2":{"title":"Two","volume":"","source":{"service":"imgchest","id":"a2"}},
	"3":{"title":"Three","volume":" 2 ","source":{"service":"imgchest","id":"a3"}},
	"4":{"title":"Four","source":{"service":"imgchest","id":"a4"}},
	"5":{"title":"Five","volume":10.5,"source":{"service":"imgchest","id":"a5"}},
	"6":{"title":"Six","volume":-1,"source":{"service":"imgchest","id":"a6"}},
	"7":{"title":"Seven","volume":null,"source":{"service":"imgchest","id":"a7"}},
	"8":{"title":"Eight","volume":3,"source":{"service":"imgchest","id":"a8"}}
}}
</script></body></html>`

// the chapters' volumes come from the series JSON, 0 when it's empty or missing
func TestBigsoloVolumes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(bigsoloTestPage))
	}))
	defer srv.Close()

	b := NewBigsolo(&Grabber{URL: srv.URL + "/manga/some-series", Settings: &Settings{}})
	chapters, errs := b.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters() errors = %v", errs)
	}

	want := map[float64]int{1: 1, 2: 0, 3: 2, 4: 0, 5: 0, 6: 0, 7: 0, 8: 3}
	if len(chapters) != len(want) {
		t.Fatalf("FetchChapters() got %d chapters, want %d", len(chapters), len(want))
	}
	for _, c := range chapters {
		if got := VolumeOf(c); got != want[c.GetNumber()] {
			t.Errorf("chapter %v volume = %d, want %d", c.GetNumber(), got, want[c.GetNumber()])
		}
	}
}

// the old single-segment series links move to /manga/{slug}; the rest are left alone
func TestBigsoloSeriesURL(t *testing.T) {
	tests := []struct{ in, want string }{
		{"https://bigsolo.org/wind-breaker", "https://bigsolo.org/manga/wind-breaker"},
		{"https://bigsolo.org/wind-breaker/", "https://bigsolo.org/manga/wind-breaker"},
		{"https://www.bigsolo.org/wind-breaker", "https://www.bigsolo.org/manga/wind-breaker"},
		{"https://bigsolo.org/manga/wind-breaker", "https://bigsolo.org/manga/wind-breaker"},
		{"https://bigsolo.org/manga/wind-breaker/episodes", "https://bigsolo.org/manga/wind-breaker/episodes"},
		{"https://bigsolo.org/manga", "https://bigsolo.org/manga"},
		{"https://bigsolo.org/", "https://bigsolo.org/"},
		{"https://bigsolo.org", "https://bigsolo.org"},
		{"https://example.org/wind-breaker", "https://example.org/wind-breaker"},
	}
	for _, tt := range tests {
		if got := bigsoloSeriesURL(tt.in); got != tt.want {
			t.Errorf("bigsoloSeriesURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
