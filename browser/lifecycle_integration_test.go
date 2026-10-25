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
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/chromedp/cdproto/input"
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
	headlessOnly(t)
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
		case "/init":
			// what the page's own first script sees of an init script
			fmt.Fprint(w, `<html><head><script>
				var seeded = localStorage.getItem("seed") || "none", flag = window.__seeded || "none";
			</script></head><body><pre id="ok"></pre><script>
				document.getElementById("ok").textContent = seeded + "/" + flag;
				localStorage.removeItem("seed");
			</script></body></html>`)
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

// headlessOnly refuses to launch a visible browser, whatever the display, so
// an escalation always fails instead of opening a window on the desktop, and
// forgets the hosts that escalated, so none starts visible
func headlessOnly(t *testing.T) {
	t.Helper()
	prev := launch
	launch = func(abort context.Context, headed bool) (*instance, error) {
		if headed {
			return nil, errors.New("no visible browser in the tests")
		}
		return launchChrome(abort, headed)
	}
	t.Cleanup(func() {
		launch = prev
		hostsMu.Lock()
		needed = map[string]bool{}
		hostsMu.Unlock()
	})
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
	headlessOnly(t)
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

	var (
		noticesMu sync.Mutex
		gotNotice string
	)
	notice.SetHandler(func(_ notice.Level, msg string) {
		noticesMu.Lock()
		defer noticesMu.Unlock()
		if strings.Contains(msg, "pop-up") && gotNotice == "" {
			gotNotice = msg
		}
	})
	t.Cleanup(func() { notice.SetHandler(nil) })

	// the page opens a popup on load (script), from a button's onclick and
	// from a link with target=_blank; the popup page itself is plain. The
	// button and link are absolutely placed so the clicks have known targets
	popupSrv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		switch r.URL.Path {
		case "/popup":
			fmt.Fprint(w, `<html><body>popup</body></html>`)
		case "/warmup":
			fmt.Fprint(w, `<html><body><div id="ok">ok</div></body></html>`)
		case "/page":
			fmt.Fprint(w, `<html><body>
				<div id="ready">ready</div>
				<a id="link" href="/popup" target="_blank" rel="noopener"
					style="position:absolute;left:100px;top:300px;display:block;width:200px;height:50px">open with link</a>
				<button id="btn" onclick="window.open('/popup')"
					style="position:absolute;left:100px;top:200px;width:200px;height:50px">open popup</button>
				<script>window.open('/popup');</script>
			</body></html>`)
		default:
			nethttp.NotFound(w, r)
		}
	}))
	t.Cleanup(popupSrv.Close)

	// starts the browser, so the listener below sees the tab it creates
	if _, err := GetHTML(popupSrv.URL+"/warmup", "#ok", 30*time.Second); err != nil {
		t.Fatalf("starting the browser: %v", err)
	}

	mu.Lock()
	b := inst
	mu.Unlock()
	if b == nil {
		t.Fatal("no browser running after load")
	}
	popupCount := countPopupsFrom(b.ctx)

	if _, err := GetHTML(popupSrv.URL+"/page", "#ready", 30*time.Second); err != nil {
		t.Fatalf("loading a page that opens a popup: %v", err)
	}

	// a popup that closed before we looked would pass the target count alone,
	// so each case first waits for its popup to be seen
	waitUntil(t, func() bool { return popupCount.Load() >= 1 }, 3*time.Second, "script popup never opened")
	if !waitPageTargets(t, b, 1, 3*time.Second) {
		t.Error("script popup was not closed")
	}

	before := popupCount.Load()
	clickAt(t, b, 200, 225) // button
	waitUntil(t, func() bool { return popupCount.Load() > before }, 3*time.Second, "button popup never opened")
	if !waitPageTargets(t, b, 1, 3*time.Second) {
		t.Error("button popup was not closed")
	}

	before = popupCount.Load()
	clickAt(t, b, 200, 325) // link with target=_blank
	waitUntil(t, func() bool { return popupCount.Load() > before }, 3*time.Second, "link popup never opened")
	if !waitPageTargets(t, b, 1, 3*time.Second) {
		t.Error("link popup was not closed")
	}

	// the notice is emitted from the closing goroutine, so it may trail the
	// target going away by a moment
	deadline := time.Now().Add(2 * time.Second)
	for {
		noticesMu.Lock()
		got := gotNotice
		noticesMu.Unlock()
		if got != "" || time.Now().After(deadline) {
			if got != popupClosedNotice {
				t.Errorf("popup notice = %q, want %q", got, popupClosedNotice)
			}
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// countPopupsFrom registers a ListenTarget on ctx and returns an atomic counter
// that increments each time a popup page target whose OpenerID is ctx's tab is created.
func countPopupsFrom(ctx context.Context) *atomic.Int32 {
	var n atomic.Int32
	c := chromedp.FromContext(ctx)
	tabID := c.Target.TargetID
	listen(chromedp.Events(ctx, target.TargetCreated), func(e target.EventTargetCreated) {
		if isPopupOf(e.TargetInfo, tabID) {
			n.Add(1)
		}
	})
	return &n
}

// waitUntil polls cond until it holds (or deadline), failing the test with msg otherwise.
func waitUntil(t *testing.T, cond func() bool, deadline time.Duration, msg string) {
	t.Helper()
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Error(msg)
}

// waitPageTargets polls until exactly n page targets remain (or deadline).
func waitPageTargets(t *testing.T, b *instance, n int, deadline time.Duration) bool {
	t.Helper()
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		ctx, cancel := context.WithTimeout(b.ctx, 5*time.Second)
		infos, err := chromedp.Targets(ctx)
		cancel()
		if err != nil {
			t.Logf("listing targets: %v", err)
			time.Sleep(50 * time.Millisecond)
			continue
		}
		count := 0
		for _, ti := range infos {
			if ti.Type == "page" {
				count++
			}
		}
		if count == n {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// clickAt dispatches a trusted mouse click at (x, y) via CDP input events.
func clickAt(t *testing.T, b *instance, x, y float64) {
	t.Helper()
	ctx, cancel := context.WithTimeout(b.ctx, 5*time.Second)
	defer cancel()
	err := chromedp.Do(ctx,
		send(input.DispatchMouseEvent, input.DispatchMouseEventParams{Type: input.DispatchMouseEventTypeMousePressed, X: x, Y: y, Button: input.MouseButtonLeft, ClickCount: 1}),
		send(input.DispatchMouseEvent, input.DispatchMouseEventParams{Type: input.DispatchMouseEventTypeMouseReleased, X: x, Y: y, Button: input.MouseButtonLeft, ClickCount: 1}),
	)
	if err != nil {
		t.Fatalf("clicking at (%v, %v): %v", x, y, err)
	}
}

// between returns the text of s between the first from and the next to
func between(s, from, to string) string {
	_, after, _ := strings.Cut(s, from)
	text, _, _ := strings.Cut(after, to)
	return text
}

func TestIntegrationSlowWorkAfterRenderIsNotAChallenge(t *testing.T) {
	srv := integration(t)

	// waiting out the bottom (3 x 1.5s) after the selector matched, with a 2s
	// budget for the render: the scroll gets its own
	html, err := GetHTMLWithScroll(srv.URL+"/ok", "#ok", Scroll{Pause: 1500 * time.Millisecond}, 2*time.Second)
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

func TestIntegrationInitScriptRunsBeforeThePageOnce(t *testing.T) {
	srv := integration(t)

	s := Scroll{Done: "true", Init: `localStorage.setItem("seed", "all"); window.__seeded = "yes";`}
	page, err := GetHTMLWithScroll(srv.URL+"/init", "#ok", s, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got := between(page, `<pre id="ok">`, "</pre>"); got != "all/yes" {
		t.Errorf("the page saw %q, want the init script's all/yes", got)
	}

	// the tab is reused: the script mustn't run on the next page
	page, err = GetHTML(srv.URL+"/init", "#ok", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got := between(page, `<pre id="ok">`, "</pre>"); got != "none/none" {
		t.Errorf("the next page saw %q, want none/none", got)
	}
}
