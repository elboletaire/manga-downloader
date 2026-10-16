// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// TestChainsawManOnlineTest smoke-tests Test() against the real domain, subdomain and lookalike.
func TestChainsawManOnlineTest(t *testing.T) {
	yes := func(url string) {
		t.Helper()
		got, err := NewChainsawManOnline(&Grabber{URL: url, Settings: &Settings{}}).Test()
		if err != nil || !got {
			t.Errorf("Test(%q) = %v, %v; want true, nil", url, got, err)
		}
	}
	no := func(url string) {
		t.Helper()
		got, err := NewChainsawManOnline(&Grabber{URL: url, Settings: &Settings{}}).Test()
		if err != nil || got {
			t.Errorf("Test(%q) = %v, %v; want false, nil", url, got, err)
		}
	}

	yes("https://chainsawman-chapter.online/")
	yes("https://w2.chainsawman-chapter.online/")
	yes("https://CHAINSAWMAN-CHAPTER.ONLINE/")
	no("https://mychainsawman-chapter.online/")
	no("https://example.com/?url=chainsawman-chapter.online")
	no("https://mangadex.org/")
}

// TestChainsawManOnlineParseRoot verifies chapter list and title parsing from the root page.
func TestChainsawManOnlineParseRoot(t *testing.T) {
	const html = `<!doctype html>
<html><body>
<!-- Nav anchors have no class: must be excluded -->
<a href="/comic/chainsaw-man-chapter-1/">First chapter (nav)</a>
<a href="/comic/chainsaw-man-chapter-232/">Last chapter (nav)</a>

<a class="chapter-list-item" href="/comic/chainsaw-man-chapter-231/"
   aria-label="Chainsaw Man, Chapter 231"></a>
<a class="chapter-list-item" href="/comic/chainsaw-man-chapter-232/"
   aria-label="Chainsaw Man, Chapter 232"></a>
<!-- entry with no number: must be skipped -->
<a class="chapter-list-item" href="/comic/bonus/"
   aria-label="Chainsaw Man, Bonus Chapter"></a>
</body></html>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parse HTML: %v", err)
	}

	title, chapters := chainsawManOnlineParseRoot(doc)

	if title != "Chainsaw Man" {
		t.Errorf("title = %q, want %q", title, "Chainsaw Man")
	}

	if len(chapters) != 2 {
		t.Fatalf("got %d chapters, want 2", len(chapters))
	}

	want := []struct {
		title  string
		number float64
	}{
		{"Chapter 231", 231},
		{"Chapter 232", 232},
	}
	for i, w := range want {
		if ChapterLabel(chapters[i]) != w.title {
			t.Errorf("chapter[%d].title = %q, want %q", i, ChapterLabel(chapters[i]), w.title)
		}
		if chapters[i].GetNumber() != w.number {
			t.Errorf("chapter[%d].number = %v, want %v", i, chapters[i].GetNumber(), w.number)
		}
	}
}
