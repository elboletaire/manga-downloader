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
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/dom"
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
