// Copyright (C) 2023-2026 Òscar Casajuana Alonso

// Package browser renders pages with a locally installed Chrome (or
// Chromium/Brave/Edge) via chromedp, for sites that cannot be scraped with
// plain HTTP requests: javascript-rendered chapter lists, TLS-fingerprint
// blocks or Cloudflare challenges.
//
// A single browser process is started lazily on first use and shared by all
// grabbers. After each page load the session cookies and real user agent are
// harvested into the http package, so images can still be downloaded with
// fast plain HTTP requests instead of through the browser.
package browser

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/elboletaire/manga-downloader/http"
	"github.com/fatih/color"
)

// visible determines if the browser window is shown (headed mode). Some
// Cloudflare challenges only pass in headed mode, where the user can also
// solve interactive captchas manually.
var visible bool

// SetVisible toggles headed mode. Must be called before the first page fetch.
func SetVisible(v bool) {
	visible = v
}

// settle is an extra wait applied after the wait selector matches, for pages
// that keep populating content after the first elements appear
var settle time.Duration

// SetSettle sets an extra wait after the wait selector matches
func SetSettle(d time.Duration) {
	settle = d
}

// NetLog, when set, is called for every network response received while
// rendering pages. Only meant for debugging/site investigation.
var NetLog func(url string, status int, mime string)

var (
	mu          sync.Mutex
	allocCtx    context.Context
	allocCancel context.CancelFunc
	browserCtx  context.Context
	browserStop context.CancelFunc
)

// ErrNoBrowser is returned when no Chrome-like browser is installed
var ErrNoBrowser = errors.New(
	"no Chrome-like browser found: this site needs a real browser to be scraped.\n" +
		"Install Google Chrome (or Chromium/Brave/Edge) and try again",
)

// start boots the shared browser instance (only once)
func start() error {
	if browserCtx != nil {
		return nil
	}

	opts := chromedp.DefaultExecAllocatorOptions[:]
	if visible {
		// not minimized: when visible it's because a challenge needs to be seen
		// (and possibly solved) by the user
		opts = append(opts, chromedp.Flag("headless", false))
	}
	// reduce automation fingerprint: this flag adds navigator.webdriver etc.
	opts = append(opts, chromedp.Flag("disable-blink-features", "AutomationControlled"))
	// chromedp's defaults pass --enable-automation (another loud automation
	// signal, plus the "Chrome is being controlled" infobar); strict cloudflare
	// configs (e.g. sakuramangas) loop the challenge forever when they see it
	opts = append(opts, chromedp.Flag("enable-automation", false))
	if inContainer() {
		// chrome's sandbox can't initialize in unprivileged containers
		// (no setuid helper, user namespaces blocked by the default seccomp
		// profile), so without this flag it crashes on startup
		opts = append(opts, chromedp.NoSandbox)
	}

	allocCtx, allocCancel = chromedp.NewExecAllocator(context.Background(), opts...)
	browserCtx, browserStop = chromedp.NewContext(allocCtx)

	// starting the browser eagerly gives a nicer error when chrome is missing
	if err := chromedp.Run(browserCtx); err != nil {
		browserStop()
		allocCancel()
		browserCtx, allocCtx = nil, nil
		if strings.Contains(err.Error(), "executable file not found") {
			return ErrNoBrowser
		}
		return fmt.Errorf("error starting browser: %w", err)
	}

	return nil
}

// inContainer reports whether we're running inside a container (docker sets
// /.dockerenv, podman /run/.containerenv). BROWSER_NO_SANDBOX=1 forces it for
// container runtimes that leave no marker.
func inContainer() bool {
	if os.Getenv("BROWSER_NO_SANDBOX") == "1" {
		return true
	}
	for _, marker := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(marker); err == nil {
			return true
		}
	}
	return false
}

// Close shuts down the shared browser instance, if it was started. Safe to
// call multiple times, must be called before exiting or the Chrome process
// is left behind.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	teardown()
}

// teardown stops the browser instance if running. Callers must hold mu.
func teardown() {
	if browserCtx == nil {
		return
	}
	// gracefully close the browser (Cancel waits for it, browserStop doesn't)
	chromedp.Cancel(browserCtx)
	browserStop()
	allocCancel()
	browserCtx, allocCtx = nil, nil
}

// timeouts for the first render attempt. The headless probe is kept short:
// a page behind a challenge will never pass headless, so instead of making the
// user wait we escalate to a visible browser as soon as it times out.
const (
	headlessProbeTimeout = 30 * time.Second
	visibleTimeout       = 5 * time.Minute
)

// challengeError is returned when the wait selector never shows up, which
// almost always means the page sits behind a challenge (e.g. cloudflare) that
// a headless browser can't pass.
type challengeError struct {
	url      string
	selector string
}

func (e *challengeError) Error() string {
	return fmt.Sprintf("timed out waiting for %q at %s", e.selector, e.url)
}

// GetHTML renders the given URL and returns its HTML once waitSelector is
// visible (skipped if empty). Cookies and the browser user agent are copied to
// the http package so subsequent plain HTTP requests (e.g. image downloads)
// reuse the browser session.
//
// It first tries a headless browser. If that times out on the wait selector
// (typically a cloudflare/JS challenge) and the user didn't already ask for a
// visible browser, it transparently reopens a visible window and retries — so
// users don't need to know about --browser-visible.
func GetHTML(url, waitSelector string, timeout time.Duration) (string, error) {
	return getHTML(url, waitSelector, timeout, nil)
}

// GetHTMLWithLocalStorage is like GetHTML, but first sets a single localStorage
// key/value pair on the page's origin and reloads it before waiting on
// waitSelector. Some sites gate content behind a client-side preference that's
// only checked on load (e.g. a "load every page at once" reader mode stored in
// localStorage instead of a URL or cookie), so a plain navigation isn't enough.
func GetHTMLWithLocalStorage(url, key, value, waitSelector string, timeout time.Duration) (string, error) {
	pre := []chromedp.Action{
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Evaluate(fmt.Sprintf("localStorage.setItem(%q, %q)", key, value), nil),
		chromedp.Reload(),
	}
	return getHTML(url, waitSelector, timeout, pre)
}

// GetHTMLWithScroll is like GetHTML, but after waitSelector matches it scrolls
// the page down in increments (scrollIterations times, pausing scrollPause
// between each), then takes the final HTML snapshot. Some readers virtualize
// or lazy-mount their page images via an IntersectionObserver, so a plain
// GetHTML only captures the handful of pages near the top; scrolling lets the
// site's own JS progressively mount the rest before the snapshot is taken.
func GetHTMLWithScroll(url, waitSelector string, scrollIterations int, scrollPause time.Duration, timeout time.Duration) (string, error) {
	pre := []chromedp.Action{}
	if waitSelector != "" {
		pre = append(pre, chromedp.WaitVisible(waitSelector, chromedp.ByQuery))
	}
	for i := 1; i <= scrollIterations; i++ {
		frac := float64(i) / float64(scrollIterations)
		pre = append(pre,
			chromedp.Evaluate(fmt.Sprintf(`window.scrollTo(0, document.body.scrollHeight * %f)`, frac), nil),
			chromedp.Sleep(scrollPause),
		)
	}
	return getHTML(url, "", timeout, pre)
}

// dismissModalJS closes an open modal dialog (e.g. mkissa's "Sign in to
// recommend this to the community rail" nag) before it eats a click. The
// dialog's fixed, full-viewport `.modal-backdrop` sits above everything else
// (z-index 300), so a click aimed at whatever's underneath silently lands on
// the backdrop instead — the target element never reports being interacted
// with, and no error is raised. A no-op when no modal is open.
const dismissModalJS = `(function(){var c=document.querySelector('.modal-backdrop .modal__close');if(c)c.click();})()`

// GetReaderHTML renders a chapter reader page for SPA sites whose reader
// route is blocked by Cloudflare on direct navigation (confirmed via a 403
// even after warming up cookies from the series page in the same browser
// context) but reachable through the app's own client-side routing
// (mkissa.to). It navigates to seriesURL, dismisses any open sign-in modal
// (see dismissModalJS), clicks tabSelector to reveal the chapter list,
// clicks through paginationSelector buttons (if present) until an element
// matching linkSelector appears, clicks it, then scrolls the page repeatedly
// to force lazy-loaded reader images to resolve.
//
// Some of these readers auto-continue into the next chapter once the current
// one is scrolled past (its pages get appended to the same DOM), so instead
// of a fixed total, scrolling stops once the count of imgSelector elements
// whose src contains urlSubstr (e.g. "/{mangaId}/{chapterNumber}/", unique to
// the requested chapter) stays stable for two checks in a row, or after a
// generous number of scrolls as a safety cap.
func GetReaderHTML(seriesURL, tabSelector, paginationSelector, linkSelector, imgSelector, urlSubstr string, timeout time.Duration) (string, error) {
	mu.Lock()
	defer mu.Unlock()

	if err := start(); err != nil {
		return "", err
	}

	t := timeout
	if t <= 0 {
		t = visibleTimeout
	}
	ctx, cancel := context.WithTimeout(browserCtx, t)
	defer cancel()

	if visible {
		startChallengeClicker(ctx)
	}

	if NetLog != nil {
		chromedp.ListenTarget(ctx, func(ev interface{}) {
			if resp, ok := ev.(*network.EventResponseReceived); ok {
				NetLog(resp.Response.URL, int(resp.Response.Status), resp.Response.MimeType)
			}
		})
	}

	if err := chromedp.Run(ctx,
		chromedp.Navigate(seriesURL),
		chromedp.WaitVisible("body", chromedp.ByQuery),
		chromedp.Sleep(2*time.Second),
	); err != nil {
		return "", fmt.Errorf("opening chapter list: %w", err)
	}

	// the sign-in nag (see dismissModalJS) pops up on a timer independent of
	// our clicks, so it can appear between the dismiss check and the actual
	// click landing; retry a couple of times rather than dismissing once
	tabSelectedJS := fmt.Sprintf(`(function(){var e=document.querySelector(%q);return !!e&&e.getAttribute('aria-selected')==='true';})()`, tabSelector)
	for attempt := 0; attempt < 3; attempt++ {
		if err := chromedp.Run(ctx,
			chromedp.Evaluate(dismissModalJS, nil),
			chromedp.Click(tabSelector, chromedp.ByQuery),
			chromedp.Sleep(time.Second),
		); err != nil {
			return "", fmt.Errorf("opening chapter list: %w", err)
		}
		var selected bool
		if err := chromedp.Run(ctx, chromedp.Evaluate(tabSelectedJS, &selected)); err != nil {
			return "", fmt.Errorf("opening chapter list: %w", err)
		}
		if selected {
			break
		}
		if attempt == 2 {
			return "", fmt.Errorf("opening chapter list: chapters tab never became selected")
		}
	}

	// click through pagination pages until the target chapter link shows up
	linkExistsJS := fmt.Sprintf(`!!document.querySelector(%q)`, linkSelector)
	for i := 0; i < 10; i++ {
		var exists bool
		if err := chromedp.Run(ctx, chromedp.Evaluate(linkExistsJS, &exists)); err != nil {
			return "", err
		}
		if exists {
			break
		}
		clickPageJS := fmt.Sprintf(
			`(function(){var b=document.querySelectorAll(%q);if(b[%d]){b[%d].click();return true;}return false;})()`,
			paginationSelector, i, i,
		)
		var clicked bool
		if err := chromedp.Run(ctx, chromedp.Evaluate(clickPageJS, &clicked)); err != nil {
			return "", err
		}
		if !clicked {
			return "", fmt.Errorf("chapter link not found on the page (and no more pagination pages to try)")
		}
		if err := chromedp.Run(ctx, chromedp.Sleep(800*time.Millisecond)); err != nil {
			return "", err
		}
	}

	if err := chromedp.Run(ctx,
		chromedp.Evaluate(dismissModalJS, nil),
		chromedp.Click(linkSelector, chromedp.ByQuery),
		chromedp.Sleep(1500*time.Millisecond),
	); err != nil {
		return "", fmt.Errorf("opening chapter reader: %w", err)
	}

	matchedCountJS := fmt.Sprintf(
		`Array.from(document.querySelectorAll(%q)).filter(function(el){return (el.getAttribute("src")||"").indexOf(%q) !== -1;}).length`,
		imgSelector, urlSubstr,
	)
	prev, stable := -1, 0
	// full-viewport scroll steps with a generous per-step settle (each page
	// image needs to both scroll into view and finish its network fetch) and
	// a 6-in-a-row stability requirement before declaring a plateau: shorter
	// requirements false-positive on the natural lull while scrolling through
	// the middle of an already-resolved (tall) page image
	const maxScrolls = 200 // safety cap for very long chapters
	for i := 0; i < maxScrolls; i++ {
		var count int
		if err := chromedp.Run(ctx, chromedp.Evaluate(matchedCountJS, &count)); err != nil {
			return "", err
		}
		if count == prev && count > 0 {
			stable++
			if stable >= 6 {
				break
			}
		} else {
			stable = 0
		}
		prev = count
		if err := chromedp.Run(ctx,
			chromedp.Evaluate(`window.scrollBy(0, window.innerHeight)`, nil),
			chromedp.Sleep(700*time.Millisecond),
		); err != nil {
			return "", err
		}
	}

	var html string
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			node, err := dom.GetDocument().Do(ctx)
			if err != nil {
				return err
			}
			html, err = dom.GetOuterHTML().WithNodeID(node.NodeID).Do(ctx)
			return err
		}),
		chromedp.ActionFunc(harvestSession),
	); err != nil {
		return "", err
	}

	return html, nil
}

// APIResponse is a network response captured while rendering a page (its URL
// and raw body), returned by GetAPIResponses.
type APIResponse struct {
	URL  string
	Body string
}

// GetAPIResponses renders pageURL and returns the raw bodies of every network
// response whose URL contains urlSubstr. It's for SPA sites that sign their own
// JSON API calls client-side (e.g. a per-session token the endpoints can't be
// replayed without), so plain HTTP gets rejected: the rendered page makes the
// signed calls itself and we harvest the responses.
//
// If nextSelector is non-empty, after the initial load it repeatedly clicks
// that selector (a pagination "next" button) and waits for each click's
// response, until the button reports itself gone/disabled, a click produces no
// new matching response, or maxClicks is reached — letting a paginated list be
// collected page by page. The browser session (cookies + user agent) is
// harvested afterwards so images still download over plain HTTP.
//
// Like GetHTML it tries headless first and transparently escalates to a visible
// window if the wait selector times out (typically a challenge).
func GetAPIResponses(pageURL, waitSelector, urlSubstr, nextSelector string, maxClicks int, timeout time.Duration) ([]APIResponse, error) {
	mu.Lock()
	defer mu.Unlock()

	t := timeout
	if t <= 0 {
		t = headlessProbeTimeout
		if visible {
			t = visibleTimeout
		}
	}

	res, err := captureAPI(pageURL, waitSelector, urlSubstr, nextSelector, maxClicks, t)
	if err == nil {
		return res, nil
	}

	var ce *challengeError
	if errors.As(err, &ce) && !visible {
		color.Yellow("%s didn't load in a headless browser (likely a challenge).", hostOf(pageURL))
		color.Yellow("opening a visible browser window — solve the challenge there if one appears...")
		if rerr := goVisible(); rerr != nil {
			return nil, rerr
		}
		if res, err = captureAPI(pageURL, waitSelector, urlSubstr, nextSelector, maxClicks, visibleTimeout); err == nil {
			return res, nil
		}
	}

	if errors.As(err, &ce) {
		return nil, fmt.Errorf("%w: the challenge may not have been solved in time", err)
	}
	return nil, err
}

// captureAPI performs a single render capturing the response bodies whose URL
// contains urlSubstr, optionally clicking through nextSelector pagination.
// Callers must hold mu.
func captureAPI(pageURL, waitSelector, urlSubstr, nextSelector string, maxClicks int, timeout time.Duration) ([]APIResponse, error) {
	if err := start(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(browserCtx, timeout)
	defer cancel()

	if visible {
		startChallengeClicker(ctx)
	}

	type entry struct {
		id  network.RequestID
		url string
	}
	var (
		cmu     sync.Mutex
		entries []entry
	)
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		resp, ok := ev.(*network.EventResponseReceived)
		if !ok {
			return
		}
		if NetLog != nil {
			NetLog(resp.Response.URL, int(resp.Response.Status), resp.Response.MimeType)
		}
		if urlSubstr != "" && strings.Contains(resp.Response.URL, urlSubstr) {
			cmu.Lock()
			entries = append(entries, entry{resp.RequestID, resp.Response.URL})
			cmu.Unlock()
		}
	})

	nav := []chromedp.Action{network.Enable(), chromedp.Navigate(pageURL)}
	if waitSelector != "" {
		nav = append(nav, chromedp.WaitVisible(waitSelector, chromedp.ByQuery))
	}
	if err := chromedp.Run(ctx, nav...); err != nil {
		if ctx.Err() != nil && waitSelector != "" {
			return nil, &challengeError{url: pageURL, selector: waitSelector}
		}
		return nil, err
	}

	count := func() int {
		cmu.Lock()
		defer cmu.Unlock()
		return len(entries)
	}

	// wait for the initial matching response(s) the SPA fires on load, so we
	// don't fetch bodies before they exist
	waitForGrowth(ctx, count, 0)

	if nextSelector != "" {
		clickJS := fmt.Sprintf(
			`(function(){var b=document.querySelector(%q);if(!b)return "gone";if(b.disabled||b.getAttribute("aria-disabled")==="true")return "disabled";b.click();return "clicked";})()`,
			nextSelector,
		)
		for i := 0; i < maxClicks; i++ {
			before := count()
			var status string
			if err := chromedp.Run(ctx, chromedp.Evaluate(clickJS, &status)); err != nil {
				return nil, err
			}
			if status != "clicked" {
				break
			}
			// stop once a click yields no new matching response (last page)
			if !waitForGrowth(ctx, count, before) {
				break
			}
		}
	}

	// harvest cookies + UA so image downloads reuse the browser session
	_ = chromedp.Run(ctx, chromedp.ActionFunc(harvestSession))

	cmu.Lock()
	defer cmu.Unlock()
	out := make([]APIResponse, 0, len(entries))
	for _, e := range entries {
		id := e.id
		var body []byte
		_ = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			b, berr := network.GetResponseBody(id).Do(ctx)
			if berr == nil {
				body = b
			}
			return nil
		}))
		if len(body) > 0 {
			out = append(out, APIResponse{URL: e.url, Body: string(body)})
		}
	}
	return out, nil
}

// waitForGrowth polls until count() exceeds baseline or a short grace period
// elapses, returning whether it grew. Used to wait for an SPA fetch triggered
// by a click/navigation to actually land before moving on.
func waitForGrowth(ctx context.Context, count func() int, baseline int) bool {
	deadline := time.Now().Add(5 * time.Second)
	for {
		if count() > baseline {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		if err := chromedp.Run(ctx, chromedp.Sleep(200*time.Millisecond)); err != nil {
			return count() > baseline
		}
	}
}

// getHTML is the shared implementation behind GetHTML and
// GetHTMLWithLocalStorage: it renders url in a headless browser and, if the
// wait selector times out (typically a cloudflare/JS challenge), transparently
// reopens a visible window and retries.
func getHTML(url, waitSelector string, timeout time.Duration, preActions []chromedp.Action) (string, error) {
	mu.Lock()
	defer mu.Unlock()

	t := timeout
	if t <= 0 {
		t = headlessProbeTimeout
		if visible {
			t = visibleTimeout
		}
	}

	html, err := render(url, waitSelector, t, preActions)
	if err == nil {
		return html, nil
	}

	var ce *challengeError
	if errors.As(err, &ce) && !visible {
		color.Yellow("%s didn't load in a headless browser (likely a challenge).", hostOf(url))
		color.Yellow("opening a visible browser window — solve the challenge there if one appears...")
		if rerr := goVisible(); rerr != nil {
			return "", rerr
		}
		if html, err = render(url, waitSelector, visibleTimeout, preActions); err == nil {
			return html, nil
		}
	}

	if errors.As(err, &ce) {
		// visible (or just escalated to it) and still no luck
		return "", fmt.Errorf("%w: the challenge may not have been solved in time", err)
	}
	return "", err
}

// render performs a single navigation in the shared browser, reusing its
// initial tab (a new tab would spawn in the background, leaving the blank
// first tab in front and hiding the page). preActions, if any, run right
// after the initial navigation and before waitSelector is awaited (e.g. to
// set a localStorage flag and reload). Callers must hold mu.
func render(url, waitSelector string, timeout time.Duration, preActions []chromedp.Action) (string, error) {
	if err := start(); err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(browserCtx, timeout)
	defer cancel()

	if visible {
		startChallengeClicker(ctx)
	}

	if NetLog != nil {
		chromedp.ListenTarget(ctx, func(ev interface{}) {
			if resp, ok := ev.(*network.EventResponseReceived); ok {
				NetLog(resp.Response.URL, int(resp.Response.Status), resp.Response.MimeType)
			}
		})
	}

	var html string
	actions := []chromedp.Action{
		chromedp.Navigate(url),
	}
	actions = append(actions, preActions...)
	if waitSelector != "" {
		actions = append(actions, chromedp.WaitVisible(waitSelector, chromedp.ByQuery))
	}
	if settle > 0 {
		actions = append(actions, chromedp.Sleep(settle))
	}
	actions = append(actions,
		chromedp.ActionFunc(func(ctx context.Context) error {
			node, err := dom.GetDocument().Do(ctx)
			if err != nil {
				return err
			}
			html, err = dom.GetOuterHTML().WithNodeID(node.NodeID).Do(ctx)
			return err
		}),
		chromedp.ActionFunc(harvestSession),
	)

	if err := chromedp.Run(ctx, actions...); err != nil {
		if ctx.Err() != nil && waitSelector != "" {
			return "", &challengeError{url: url, selector: waitSelector}
		}
		return "", err
	}

	return html, nil
}

// startChallengeClicker polls the page in the background and, whenever a
// Cloudflare interstitial with a "Verify you are human" checkbox is showing,
// clicks the checkbox with CDP-dispatched mouse events. Those produce
// isTrusted:true events, which Turnstile accepts — so interactive challenges
// resolve without a human, which is what makes headless-less environments
// (the docker :browser image, where the "visible" window is an Xvfb
// framebuffer nobody can see) able to pass them. Stops when ctx is done.
func startChallengeClicker(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				clickChallenge(ctx)
			}
		}
	}()
}

// clickChallenge performs a single detect-and-click attempt. The Turnstile
// widget lives in a closed shadow root, so the iframe itself can't be queried;
// instead the click targets its host row inside the interstitial's
// .main-content (a full-width, ~68px-tall div), where the checkbox sits at
// the left edge. Clicking a stale or wrong spot is harmless — the poller just
// tries again on the next tick, and stops matching once the real page loads.
func clickChallenge(ctx context.Context) {
	var box []float64
	js := `(() => {
		if (!/just a moment|verify|attention required/i.test(document.title) &&
			!document.querySelector('#challenge-stage, #challenge-form')) return null;
		for (const d of document.querySelectorAll('div.main-content div')) {
			const r = d.getBoundingClientRect();
			if (r.width > 200 && r.height > 55 && r.height < 90) {
				return [r.x, r.y, r.width, r.height];
			}
		}
		return null;
	})()`
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &box)); err != nil || len(box) < 4 {
		return
	}
	x, y := box[0]+30, box[1]+box[3]/2
	_ = chromedp.Run(ctx,
		input.DispatchMouseEvent(input.MouseMoved, x, y),
		input.DispatchMouseEvent(input.MousePressed, x, y).
			WithButton(input.Left).WithClickCount(1),
		input.DispatchMouseEvent(input.MouseReleased, x, y).
			WithButton(input.Left).WithClickCount(1),
	)
}

// goVisible tears down the current headless browser and forces the next start
// into visible mode. Callers must hold mu.
func goVisible() error {
	teardown()
	visible = true
	return start()
}

// hostOf returns the host of a URL without the www. prefix, for user messages.
func hostOf(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Hostname() != "" {
		return strings.TrimPrefix(u.Hostname(), "www.")
	}
	return raw
}

// harvestSession copies the browser cookies and user agent into the http
// package, so image downloads can go through fast plain HTTP requests
func harvestSession(ctx context.Context) error {
	var ua string
	if err := chromedp.Evaluate(`navigator.userAgent`, &ua).Do(ctx); err == nil && ua != "" {
		http.SetUserAgent(ua)
	}

	cookies, err := network.GetCookies().Do(ctx)
	if err != nil {
		return nil // cookies are best-effort, don't fail the whole fetch
	}
	for _, c := range cookies {
		http.SetCookie(strings.TrimPrefix(c.Domain, "."), c.Name, c.Value)
	}

	return nil
}
