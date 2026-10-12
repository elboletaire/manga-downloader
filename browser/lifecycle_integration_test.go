// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package browser

import (
	"context"
	"errors"
	"fmt"
	"html"
	nethttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/elboletaire/manga-downloader/http"
	"github.com/elboletaire/manga-downloader/notice"
)

// These tests drive a real headless browser, so they only run when
// MANGA_DOWNLOADER_BROWSER_TESTS=1, on Linux, with a Chrome-like browser.
// DISPLAY and WAYLAND_DISPLAY are cleared so a visible window can't open: the
// escalation to a visible browser fails instead, which is one more failure
// the shared browser has to recover from.
func integration(t *testing.T) *httptest.Server {
	t.Helper()
	if os.Getenv("MANGA_DOWNLOADER_BROWSER_TESTS") != "1" {
		t.Skip("set MANGA_DOWNLOADER_BROWSER_TESTS=1 to run the real browser tests")
	}
	if runtime.GOOS != "linux" {
		// clearing DISPLAY only keeps a visible window from opening on Linux
		t.Skip("the real browser tests run on Linux only")
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
	// the failed escalation mustn't wait the full visible timeout
	mu.Lock()
	prev := visibleTimeout
	visibleTimeout = 10 * time.Second
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		visibleTimeout = prev
		mu.Unlock()
	})
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

func TestIntegrationClosesPopupTabs(t *testing.T) {
	if os.Getenv("MANGA_DOWNLOADER_BROWSER_TESTS") != "1" {
		t.Skip("set MANGA_DOWNLOADER_BROWSER_TESTS=1 to run the real browser tests")
	}
	if runtime.GOOS != "linux" {
		t.Skip("the real browser tests run on Linux only")
	}
	if _, err := findBrowser(systemHost()); err != nil {
		t.Skip("no browser installed:", err)
	}
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Cleanup(Close)
	SetVisible(false)
	mu.Lock()
	prev := visibleTimeout
	visibleTimeout = 10 * time.Second
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		visibleTimeout = prev
		mu.Unlock()
	})

	// serve a page that opens a popup via window.open on load, plus a link
	// with target=_blank to show both patterns; the popup page itself is plain
	popupSrv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		switch r.URL.Path {
		case "/popup":
			fmt.Fprint(w, `<html><body>popup</body></html>`)
		case "/page":
			popupURL := "http://" + r.Host + "/popup"
			fmt.Fprintf(w, `<html><body>
				<div id="ready">ready</div>
				<a id="link" href="%s" target="_blank">open with link</a>
				<script>window.open(%q);</script>
			</body></html>`, popupURL, popupURL)
		default:
			nethttp.NotFound(w, r)
		}
	}))
	t.Cleanup(popupSrv.Close)

	if _, err := GetHTML(popupSrv.URL+"/page", "#ready", 30*time.Second); err != nil {
		t.Fatalf("loading a page that opens a popup: %v", err)
	}

	// the popup target is closed in a goroutine; poll until it's gone
	mu.Lock()
	b := inst
	mu.Unlock()
	if b == nil {
		t.Fatal("no browser running after load")
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(b.ctx, 5*time.Second)
		var infos []*target.Info
		err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			var e error
			infos, e = target.GetTargets().Do(ctx)
			return e
		}))
		cancel()
		if err != nil {
			t.Fatalf("listing targets: %v", err)
		}
		popupOpen := false
		for _, ti := range infos {
			if ti.Type == "page" && strings.Contains(ti.URL, "/popup") {
				popupOpen = true
			}
		}
		if !popupOpen {
			return // closed as expected
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Error("popup tab was not closed within 3 seconds")
}

// between returns the text of s between the first from and the next to
func between(s, from, to string) string {
	_, after, _ := strings.Cut(s, from)
	text, _, _ := strings.Cut(after, to)
	return text
}

func TestIntegrationSlowWorkAfterRenderIsNotAChallenge(t *testing.T) {
	srv := integration(t)

	// 3 x 1.5s of scrolling after the selector matched, with a 2s budget for
	// the render: the scroll gets its own
	html, err := GetHTMLWithScroll(srv.URL+"/ok", "#ok", 3, 1500*time.Millisecond, 2*time.Second)
	if err != nil {
		t.Fatalf("slow scroll after the page rendered: %v", err)
	}
	if !strings.Contains(html, "ready") {
		t.Fatalf("unexpected html: %s", html)
	}
}

func TestIntegrationCloseDuringStart(t *testing.T) {
	srv := integration(t)

	starting := make(chan struct{}, 1)
	notice.SetHandler(func(_ notice.Level, msg string) {
		if strings.Contains(msg, "starting it") {
			select {
			case starting <- struct{}{}:
			default:
			}
		}
	})
	t.Cleanup(func() { notice.SetHandler(nil) })

	done := make(chan error, 1)
	go func() {
		_, err := GetHTML(srv.URL+"/never", "#ok", time.Minute)
		done <- err
	}()
	<-starting
	began := time.Now()
	Close()
	if waited := time.Since(began); waited > 8*time.Second {
		t.Errorf("Close took %s", waited)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrClosed) {
			t.Errorf("load ended with %v, want ErrClosed", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the load didn't end after Close")
	}
	if kids := childProcesses(t); len(kids) > 0 {
		t.Errorf("browser processes left after Close: %v", kids)
	}
}

// childProcesses lists this process's live children (Linux)
func childProcesses(t *testing.T) []string {
	t.Helper()
	tasks, _ := filepath.Glob(fmt.Sprintf("/proc/%d/task/*/children", os.Getpid()))
	var kids []string
	for _, f := range tasks {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, pid := range strings.Fields(string(b)) {
			// a zombie is dead, just not reaped: chromedp never waits for a
			// browser killed before it was handed over
			stat, err := os.ReadFile("/proc/" + pid + "/stat")
			if err != nil || strings.Contains(string(stat), ") Z ") {
				continue
			}
			kids = append(kids, pid)
		}
	}
	return kids
}
