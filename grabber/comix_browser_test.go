// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	nethttp "net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/elboletaire/manga-downloader/browser"
)

// TestIntegrationComixDone evaluates the reader's Done check in a real
// headless browser, on fake reader pages. Like the browser package's real
// browser tests, it only runs with MANGA_DOWNLOADER_BROWSER_TESTS=1, on Linux.
func TestIntegrationComixDone(t *testing.T) {
	if os.Getenv("MANGA_DOWNLOADER_BROWSER_TESTS") != "1" {
		t.Skip("set MANGA_DOWNLOADER_BROWSER_TESTS=1 to run the real browser tests")
	}
	if runtime.GOOS != "linux" {
		t.Skip("the real browser tests run on Linux only")
	}
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	browser.SetVisible(false)
	t.Cleanup(browser.Close)

	slot := func(n int, inner string) string {
		return fmt.Sprintf(`<div class="rpage-page" data-page="%d">%s</div>`, n, inner)
	}
	// mounted, never loaded: the src 404s
	img := `<img class="rpage-page__img" referrerpolicy="no-referrer" src="/img.webp">`
	loader := `<div class="rpage-page__loader"></div>`
	readers := map[string]string{
		"/none":    ``,
		"/missing": slot(1, img+loader) + slot(2, loader),
		"/empty":   slot(1, img) + slot(2, `<img class="rpage-page__img" src="">`),
		"/full":    slot(1, img+loader) + slot(2, img),
	}
	srv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		reader, ok := readers[r.URL.Path]
		if !ok {
			nethttp.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `<html><body><main class="rpage-main">%s</main><script>
			var done = %s;
			var out = document.createElement("pre");
			out.id = "ok";
			out.textContent = String(done);
			document.body.appendChild(out);
		</script></body></html>`, reader, comixReaderScroll.Done)
	}))
	t.Cleanup(srv.Close)

	want := map[string]string{"/none": "false", "/missing": "false", "/empty": "false", "/full": "true"}
	for path, w := range want {
		html, err := browser.GetHTML(srv.URL+path, "#ok", 30*time.Second)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		_, after, _ := strings.Cut(html, `<pre id="ok">`)
		got, _, _ := strings.Cut(after, "</pre>")
		if got != w {
			t.Errorf("%s: Done = %q, want %s", path, got, w)
		}
	}
}
