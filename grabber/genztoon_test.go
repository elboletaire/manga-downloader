// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGenzToonResolveSeriesURL(t *testing.T) {
	// Serve a fake chapter page with a series link in the h1, and a fake
	// series page with just a plain h1 title. resolveSeriesURL must only
	// fire on chapter paths and must extract the series URL + title from
	// the link in the h1.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/chapter/"):
			fmt.Fprintf(w, `<html><body><h1><span><span>
				<a href="/series/my-manga/">My Manga</a>
				</span> - Chapter 5</span></h1>
				<div id="chapters"><a href="/chapter/abc/" title="Chapter 5">Chapter 5</a></div>
			</body></html>`)
		default:
			fmt.Fprintf(w, `<html><body><h1>My Manga</h1>
				<div id="chapters"><a href="/chapter/abc/" title="Chapter 5">Chapter 5</a></div>
			</body></html>`)
		}
	}))
	defer srv.Close()

	t.Run("chapter URL resolves to series URL and extracts title", func(t *testing.T) {
		chapterURL := srv.URL + "/chapter/abc123/"
		g := NewGenzToon(&Grabber{URL: chapterURL})
		if err := g.resolveSeriesURL(); err != nil {
			t.Fatalf("resolveSeriesURL() error = %v", err)
		}
		wantURL := srv.URL + "/series/my-manga/"
		if g.URL != wantURL {
			t.Errorf("URL = %q, want %q", g.URL, wantURL)
		}
		if g.title != "My Manga" {
			t.Errorf("title = %q, want %q", g.title, "My Manga")
		}
	})

	t.Run("series URL is a no-op", func(t *testing.T) {
		seriesURL := srv.URL + "/series/my-manga/"
		g := NewGenzToon(&Grabber{URL: seriesURL})
		if err := g.resolveSeriesURL(); err != nil {
			t.Fatalf("resolveSeriesURL() error = %v", err)
		}
		if g.URL != seriesURL {
			t.Errorf("URL changed to %q, want %q", g.URL, seriesURL)
		}
		if g.title != "" {
			t.Errorf("title was set to %q, want empty for series URL", g.title)
		}
	})
}

func TestGenzToonTestHost(t *testing.T) {
	for url, want := range map[string]bool{
		"https://genztoons.org/series/x/":          true,
		"https://genzupdates.com/series/x/":        true,
		"https://example.com/series/x/":            false,
		"https://www.genztoons.org/series/x/":      true,
		"https://notgenztoons.org/series/x/":       false,
		"https://genztoons.org.evil.com/series/x/": false,
	} {
		got, _ := NewGenzToon(&Grabber{URL: url}).Test()
		if got != want {
			t.Errorf("Test(%q) = %v, want %v", url, got, want)
		}
	}
}

func TestGenzToonFetchChaptersLocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body><h1>My Manga</h1><div id="chapters">
			<a href="/chapter/b/" title="Chapter 12" c="50"><div><img src="https://api.iconify.design/material-symbols:lock.svg?color=white" class="w-6"></div>Chapter 12</a>
			<a href="/chapter/a/" title="Chapter 11" c="1"><div></div>Chapter 11</a>
		</div></body></html>`)
	}))
	defer srv.Close()

	chapters, errs := NewGenzToon(&Grabber{URL: srv.URL + "/series/my-manga/"}).FetchChapters()
	if len(errs) != 0 || len(chapters) != 2 {
		t.Fatalf("chapters = %d, errs = %v", len(chapters), errs)
	}
	if !LockedOf(chapters[0]) || LockedOf(chapters[1]) {
		t.Errorf("locked = [%v %v], want [true false]", LockedOf(chapters[0]), LockedOf(chapters[1]))
	}
}
