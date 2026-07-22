// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestComixParsePaging(t *testing.T) {
	cases := []struct {
		text      string
		wantShown int
		wantTotal int
		wantOk    bool
	}{
		{"Showing 1 to 20 of 248 items", 20, 248, true},
		{"Showing 0 to 0 of 248 items", 0, 248, true},
		{"Showing 241 to 248 of 248 items", 248, 248, true},
		{"", 0, 0, false},
		{"nothing useful here", 0, 0, false},
	}

	for _, c := range cases {
		shown, total, ok := comixParsePaging(c.text)
		if ok != c.wantOk {
			t.Errorf("comixParsePaging(%q) ok = %v, want %v", c.text, ok, c.wantOk)
			continue
		}
		if !ok {
			continue
		}
		if shown != c.wantShown || total != c.wantTotal {
			t.Errorf("comixParsePaging(%q) = (%d, %d), want (%d, %d)", c.text, shown, total, c.wantShown, c.wantTotal)
		}
	}
}

func TestComixWithPage(t *testing.T) {
	cases := []struct {
		seriesURL string
		page      int
		want      string
	}{
		{"https://comix.to/title/e1wlg-the-scandal-maker-has-returned", 1, "https://comix.to/title/e1wlg-the-scandal-maker-has-returned?page=1"},
		{"https://comix.to/title/e1wlg-the-scandal-maker-has-returned", 3, "https://comix.to/title/e1wlg-the-scandal-maker-has-returned?page=3"},
	}

	for _, c := range cases {
		got, err := comixWithPage(c.seriesURL, c.page)
		if err != nil {
			t.Errorf("comixWithPage(%q, %d) unexpected error: %v", c.seriesURL, c.page, err)
			continue
		}
		if got != c.want {
			t.Errorf("comixWithPage(%q, %d) = %q, want %q", c.seriesURL, c.page, got, c.want)
		}
	}
}

func TestComixTest(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://comix.to/title/e1wlg-the-scandal-maker-has-returned", true},
		{"https://www.comix.to/title/e1wlg-the-scandal-maker-has-returned", true},
		{"https://notcomix.to/title/foo", false},
		{"https://comix.to.evil.com/title/foo", false},
		{"https://mangak.io/some-manga", false},
	}

	for _, c := range cases {
		comix := &Comix{Grabber: &Grabber{URL: c.url}}
		got, err := comix.Test()
		if err != nil {
			t.Errorf("Test(%q) unexpected error: %v", c.url, err)
			continue
		}
		if got != c.want {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}
