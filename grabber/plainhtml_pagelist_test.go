package grabber

import (
	"os"
	"reflect"
	"regexp"
	"testing"
)

// manhuaplusOrgSelector is the Liliana-theme entry of plainHTMLSelectors
func manhuaplusOrgSelector(t *testing.T) SiteSelector {
	t.Helper()
	for _, sel := range plainHTMLSelectors {
		if sel.PageListPath != "" {
			return sel
		}
	}
	t.Fatal("no PageListPath selector in plainHTMLSelectors")
	return SiteSelector{}
}

// TestPageListURLsOrdersByDataIndex checks that a captured AJAX answer yields
// its pages in data-index order, not the order the rows come in
func TestPageListURLsOrdersByDataIndex(t *testing.T) {
	body, err := os.ReadFile("testdata/pagelist/manhuaplusorg_pages.json")
	if err != nil {
		t.Fatal(err)
	}
	got, err := pageListURLs(string(body))
	if err != nil {
		t.Fatalf("pageListURLs() error = %v", err)
	}
	want := []string{
		"https://cdn.manhuaplus.org/2023/08/05/02-00-25-8-909f1baee01eb189dc6650b89f43677b.jpg",
		"https://cdn.manhuaplus.org/2023/08/05/02-00-26-10-fb61d01dc44e853bd6946e2726029268.jpg",
		"https://cdn.manhuaplus.org/2023/08/05/02-00-45-68-c0ef84dcb3eadf664bc73919f68ee380.jpg",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pageListURLs() = %q, want %q", got, want)
	}
}

// TestPageListURLsRefused checks that a refused answer fails the chapter with
// the site's message, and one without a message still fails
func TestPageListURLsRefused(t *testing.T) {
	cases := []struct {
		name, body, want string
	}{
		{"missing chapter", `{"msg":"Chap này không tồn tại hoặc đã bị xoá","html":""}`, "page list refused: Chap này không tồn tại hoặc đã bị xoá"},
		{"no status", `{"status":false,"html":""}`, "the page list answered no status"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := pageListURLs(c.body)
			if err == nil || err.Error() != c.want {
				t.Errorf("pageListURLs() error = %v, want %q", err, c.want)
			}
		})
	}
}

// TestPageListIDReCompiles checks that every PageListIDRe compiles, since it
// is compiled per chapter fetch
func TestPageListIDReCompiles(t *testing.T) {
	for _, sel := range plainHTMLSelectors {
		if sel.PageListIDRe == "" {
			continue
		}
		if _, err := regexp.Compile(sel.PageListIDRe); err != nil {
			t.Errorf("PageListIDRe %q: %v", sel.PageListIDRe, err)
		}
	}
}

// TestManhuaplusOrgSelectorMatchesSeries checks that the series list is
// claimed by the Liliana entry, not an earlier selector, and numbers its rows
func TestManhuaplusOrgSelectorMatchesSeries(t *testing.T) {
	body, err := os.ReadFile("testdata/pagelist/manhuaplusorg_series.html")
	if err != nil {
		t.Fatal(err)
	}
	doc := docFromHTML(t, string(body))

	var first SiteSelector
	for _, sel := range plainHTMLSelectors {
		if doc.Find(sel.Rows).Length() > 0 {
			first = sel
			break
		}
	}
	want := manhuaplusOrgSelector(t)
	if first.PageListPath != want.PageListPath {
		t.Fatalf("first matching selector has PageListPath %q, want %q", first.PageListPath, want.PageListPath)
	}

	rows := doc.Find(want.Rows)
	if rows.Length() != 3 {
		t.Fatalf("rows = %d, want 3", rows.Length())
	}
	m := PlainHTML{site: want}
	if n, ok := m.rowChapterNumber(rows.First()); !ok || n != 936 {
		t.Errorf("first row number = %v, %v; want 936", n, ok)
	}
}

// TestPageListID checks that the reader's CHAPTER_ID is what the endpoint is posted with
func TestPageListID(t *testing.T) {
	re := regexp.MustCompile(manhuaplusOrgSelector(t).PageListIDRe)
	match := re.FindStringSubmatch(`CHAPTER_ID = 11542;`)
	if len(match) < 2 || match[1] != "11542" {
		t.Errorf("CHAPTER_ID capture = %q, want 11542", match)
	}
}
