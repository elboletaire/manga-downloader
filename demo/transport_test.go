//go:build demo

package demo

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elboletaire/manga-downloader/http"
)

const seriesURL = "https://inmanga.com/ver/manga/One-Piece/dfc7ecb5-e9b3-4aa5-a61b-a498993cd935"

func loadFixtures(t *testing.T) *fixtures {
	t.Helper()
	files, err := fs.Sub(fixtureFiles, "fixtures")
	if err != nil {
		t.Fatal(err)
	}
	f, err := newFixtures(files)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func get(t *testing.T, rt stdhttp.RoundTripper, url string) (*stdhttp.Response, []byte) {
	t.Helper()
	req, err := stdhttp.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, body
}

func TestServesRecordedBody(t *testing.T) {
	_, body := get(t, loadFixtures(t), seriesURL)
	if !strings.Contains(string(body), "<h1>One Piece</h1>") {
		t.Errorf("series fixture doesn't hold the title: %q", body)
	}
}

func TestRejectsUnknownURL(t *testing.T) {
	const unknown = "https://inmanga.com/ver/manga/Not-A-Series/00000000-0000-0000-0000-000000000000"
	req, _ := stdhttp.NewRequest("GET", unknown, nil)
	_, err := loadFixtures(t).RoundTrip(req)
	if err == nil || !strings.Contains(err.Error(), unknown) {
		t.Errorf("unknown URL: got %v, want an error naming it", err)
	}
}

func TestPagesArePlaceholderImages(t *testing.T) {
	f := loadFixtures(t)
	var page string
	for _, e := range f.entries {
		if e.Image {
			page = e.URL
			break
		}
	}
	if page == "" {
		t.Fatal("no image entry in the fixtures")
	}

	start := time.Now()
	resp, body := get(t, f, page)
	if elapsed := time.Since(start); elapsed < pageDelay {
		t.Errorf("page answered after %v, want at least %v", elapsed, pageDelay)
	}
	if resp.Header.Get("Content-Type") != "image/jpeg" || !bytes.HasPrefix(body, []byte{0xff, 0xd8, 0xff}) {
		t.Errorf("page is not a JPEG: %s, %d bytes", resp.Header.Get("Content-Type"), len(body))
	}
}

func TestInstalledThroughHTTP(t *testing.T) {
	http.SetRoundTripper(loadFixtures(t))
	defer http.SetRoundTripper(nil)

	text, err := http.GetText(http.RequestParams{URL: seriesURL, DisableRetry: true})
	if err != nil || !strings.Contains(text, "One Piece") {
		t.Errorf("GetText through the fixtures: %v", err)
	}

	const unknown = "https://inmanga.com/unknown"
	_, err = http.GetText(http.RequestParams{URL: unknown, DisableRetry: true})
	if err == nil || !strings.Contains(err.Error(), unknown) {
		t.Errorf("unknown URL through http: got %v, want an error naming it", err)
	}
}

func TestRecordThenReplay(t *testing.T) {
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.URL.Path == "/page" {
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte("the real page, never saved"))
			return
		}
		w.Write([]byte("<h1>Recorded</h1>"))
	}))
	defer server.Close()

	dir := t.TempDir()
	rec, err := newRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, live := get(t, rec, server.URL+"/series")
	recordedPage, _ := get(t, rec, server.URL+"/page")

	if string(live) != "<h1>Recorded</h1>" {
		t.Errorf("recorder changed the body: %q", live)
	}
	// the page is answered with the placeholder, not the site's bytes
	if recordedPage.Header.Get("Content-Type") != "image/jpeg" {
		t.Errorf("recorder answered the page with %q", recordedPage.Header.Get("Content-Type"))
	}
	if files, _ := filepath.Glob(filepath.Join(dir, "bodies", "*")); len(files) != 1 {
		t.Errorf("recorded %d bodies, want only the series", len(files))
	}

	raw, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) {
		t.Fatalf("manifest is not JSON: %s", raw)
	}

	replay, err := newFixtures(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	_, body := get(t, replay, server.URL+"/series")
	if string(body) != "<h1>Recorded</h1>" {
		t.Errorf("replayed body = %q", body)
	}
	_, page := get(t, replay, server.URL+"/page")
	if !bytes.HasPrefix(page, []byte{0xff, 0xd8, 0xff}) {
		t.Errorf("replayed page is not the placeholder JPEG")
	}
}
