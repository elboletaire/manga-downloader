// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package browser

import (
	"errors"
	"fmt"
	"html"
	nethttp "net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/elboletaire/manga-downloader/http"
)

// These tests drive a real headless browser, so they only run when
// MANGA_DOWNLOADER_BROWSER_TESTS=1 (and a Chrome-like browser is installed).
// DISPLAY and WAYLAND_DISPLAY are cleared so a visible window can't open: the
// escalation to a visible browser fails instead, which is one more failure
// the shared browser has to recover from.
func integration(t *testing.T) *httptest.Server {
	t.Helper()
	if os.Getenv("MANGA_DOWNLOADER_BROWSER_TESTS") != "1" {
		t.Skip("set MANGA_DOWNLOADER_BROWSER_TESTS=1 to run the real browser tests")
	}
	if _, err := findBrowser(systemHost()); err != nil {
		t.Skip("no browser installed:", err)
	}
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	srv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		switch r.URL.Path {
		case "/ok":
			fmt.Fprint(w, `<html><body><div id="ok">ready</div></body></html>`)
		case "/hints":
			// the request's client hints, and the page's view of them
			fmt.Fprintf(w, `<html><body><pre id="header">%s</pre><pre id="js"></pre><script>
				document.getElementById("js").textContent = navigator.userAgentData
					? JSON.stringify(navigator.userAgentData.brands) : "unavailable";
				document.getElementById("js").id = "ok";
			</script></body></html>`, html.EscapeString(r.Header.Get("Sec-CH-UA")))
		case "/ua":
			fmt.Fprint(w, r.Header.Get("User-Agent"))
		default:
			// never shows the selector the caller waits for, like a page
			// stuck behind a challenge
			fmt.Fprint(w, `<html><body>just a moment...</body></html>`)
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(Close)
	SetVisible(false)
	return srv
}

func mustLoad(t *testing.T, url string) {
	t.Helper()
	html, err := GetHTML(url, "#ok", 30*time.Second)
	if err != nil {
		t.Fatalf("loading a working page: %v", err)
	}
	if !strings.Contains(html, "ready") {
		t.Fatalf("unexpected html: %s", html)
	}
}

func TestIntegrationRecoversAfterTimeout(t *testing.T) {
	srv := integration(t)

	if _, err := GetHTML(srv.URL+"/never", "#ok", 3*time.Second); err == nil {
		t.Fatal("a page without the selector loaded")
	} else {
		t.Logf("failing page: %v", err)
	}
	mustLoad(t, srv.URL+"/ok")
}

func TestIntegrationRecoversAfterBrowserDies(t *testing.T) {
	srv := integration(t)

	mustLoad(t, srv.URL+"/ok")
	pid := testBrowserPID(t)
	p, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	// what closing the window or a crash looks like from here
	if err := p.Kill(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	mustLoad(t, srv.URL+"/ok")
}

func TestIntegrationHarvestedUserAgentIsNotHeadless(t *testing.T) {
	srv := integration(t)

	mustLoad(t, srv.URL+"/ok")
	ua, err := http.GetText(&http.RequestParams{URL: srv.URL + "/ua"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ua, "Headless") {
		t.Errorf("plain HTTP requests now send a headless user agent: %q", ua)
	}

	page, err := GetHTML(srv.URL+"/hints", "#ok", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	header := between(page, `<pre id="header">`, "</pre>")
	brands := between(page, `<pre id="ok">`, "</pre>")
	t.Logf("Sec-CH-UA: %s; navigator.userAgentData.brands: %s", header, brands)
	if strings.Contains(header+brands, "Headless") {
		t.Errorf("the client hints still say headless: %s / %s", header, brands)
	}
	if brands == "[]" {
		t.Error("the client hints lost their brands")
	}
}

// testBrowserPID returns the process id of the running browser
func testBrowserPID(t *testing.T) int {
	t.Helper()
	mu.Lock()
	defer mu.Unlock()
	if inst == nil {
		t.Fatal("no browser running")
	}
	return chromedp.FromContext(inst.ctx).Browser.Process().Pid
}

func TestIntegrationCloseAbortsALoadAndEndsTheBrowser(t *testing.T) {
	srv := integration(t)

	mustLoad(t, srv.URL+"/ok")
	pid := testBrowserPID(t)

	done := make(chan error, 1)
	go func() {
		_, err := GetHTML(srv.URL+"/never", "#ok", time.Minute)
		done <- err
	}()
	time.Sleep(time.Second)
	began := time.Now()
	Close()
	select {
	case err := <-done:
		if !errors.Is(err, ErrClosed) {
			t.Errorf("load ended with %v, want ErrClosed", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Close didn't end the running load")
	}
	if waited := time.Since(began); waited > 8*time.Second {
		t.Errorf("Close took %s", waited)
	}
	if runtime.GOOS != "windows" {
		if p, err := os.FindProcess(pid); err == nil && p.Signal(syscall.Signal(0)) == nil {
			t.Errorf("browser process %d still running after Close", pid)
		}
	}
	mustLoad(t, srv.URL+"/ok")
}

// between returns the text of s between the first from and the next to
func between(s, from, to string) string {
	_, after, _ := strings.Cut(s, from)
	text, _, _ := strings.Cut(after, to)
	return text
}
