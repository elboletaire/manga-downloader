// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// volumeGroupedChaptersHTML mimics the ajax/chapters response of a Madara
// theme that groups chapters under "Volume N" wrappers (e.g. gdscans.com),
// as opposed to a flat chapter list (e.g. lhtranslation.net).
const volumeGroupedChaptersHTML = `
<div class="page-content-listing single-page">
	<div class="listing-chapters_wrap cols-1 show-more">
		<ul class="main version-chap volumns">
			<li class="parent has-child active">
				<a href="javascript:void(0)" class="has-child">Volume 10</a>
				<ul class="sub-chap list-chap">
					<li>
						<ul class="sub-chap-list">
							<li class="wp-manga-chapter">
								<a href="https://gdscans.com/manga/example/volume-10/chapter-50/">Chapter 50</a>
							</li>
						</ul>
					</li>
				</ul>
			</li>
			<li class="parent has-child">
				<a href="javascript:void(0)" class="has-child">Volume 9</a>
				<ul class="sub-chap list-chap">
					<li>
						<ul class="sub-chap-list">
							<li class="wp-manga-chapter">
								<a href="https://gdscans.com/manga/example/volume-9/chapter-49/">Chapter 49</a>
							</li>
							<li class="wp-manga-chapter">
								<a href="https://gdscans.com/manga/example/volume-9/chapter-48/">Chapter 48</a>
							</li>
						</ul>
					</li>
				</ul>
			</li>
		</ul>
	</div>
</div>
`

func TestTcbFetchChaptersSkipsVolumeWrappers(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(volumeGroupedChaptersHTML))
	if err != nil {
		t.Fatalf("failed to parse fixture: %s", err)
	}

	tcb := Tcb{
		Grabber: &Grabber{URL: "https://gdscans.com/manga/example/"},
		chaps:   doc.Find("li.wp-manga-chapter"),
	}

	chapters, errs := tcb.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	// A naive body.Find("li") would also match the two "Volume N" wrapper
	// <li> elements and their nested list-wrapper <li>, yielding bogus
	// duplicate/garbage entries. Scoped to .wp-manga-chapter we must get
	// exactly the 3 real chapters, none of them "Volume N".
	if len(chapters) != 3 {
		t.Fatalf("expected 3 chapters, got %d", len(chapters))
	}

	wantNumbers := map[float64]bool{50: false, 49: false, 48: false}
	for _, c := range chapters {
		if strings.Contains(c.GetTitle(), "Volume") {
			t.Fatalf("volume wrapper leaked into chapters list: %q", c.GetTitle())
		}
		if _, ok := wantNumbers[c.GetNumber()]; !ok {
			t.Fatalf("unexpected chapter number %v", c.GetNumber())
		}
		wantNumbers[c.GetNumber()] = true
	}
	for num, seen := range wantNumbers {
		if !seen {
			t.Fatalf("expected chapter number %v not found", num)
		}
	}
}
