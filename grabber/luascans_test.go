// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestLuascansSeriesSlug(t *testing.T) {
	cases := []struct {
		url     string
		want    string
		wantErr bool
	}{
		{"https://luacomic.org/series/even-today-the-ranker-dreams-of-retirement", "even-today-the-ranker-dreams-of-retirement", false},
		{"https://luacomic.org/series/even-today-the-ranker-dreams-of-retirement/chapter-56", "even-today-the-ranker-dreams-of-retirement", false},
		{"https://luacomic.org/series/", "", true},
		{"https://luacomic.org/", "", true},
	}

	for _, c := range cases {
		l := Luascans{Grabber: &Grabber{URL: c.url}}
		got, err := l.seriesSlug()
		if (err != nil) != c.wantErr {
			t.Errorf("seriesSlug(%q) error = %v, wantErr %v", c.url, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("seriesSlug(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

func TestLuascansSeriesIDRe(t *testing.T) {
	// simplified excerpt of the escaped JSON payload the series page embeds
	// in an inline `self.__next_f.push(...)` React Server Components script
	body := `self.__next_f.push([1,",\"$\",\"$L24\",null,{\"series_id\":312,\"series_type\":\"Comic\",\"seasons\":[]}"])`

	matches := seriesIDRe.FindStringSubmatch(body)
	if len(matches) != 2 {
		t.Fatalf("seriesIDRe did not match, got %v", matches)
	}
	if matches[1] != "312" {
		t.Errorf("seriesIDRe = %q, want %q", matches[1], "312")
	}
}
