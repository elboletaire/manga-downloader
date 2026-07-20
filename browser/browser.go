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
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/elboletaire/manga-downloader/http"
)

// visible determines if the browser window is shown (headed mode). Some
// Cloudflare challenges only pass in headed mode, where the user can also
// solve interactive captchas manually.
var visible bool

// SetVisible toggles headed mode. Must be called before the first page fetch.
func SetVisible(v bool) {
	visible = v
}

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
		opts = append(opts,
			chromedp.Flag("headless", false),
			chromedp.Flag("start-minimized", true),
		)
	}
	// reduce automation fingerprint: this flag adds navigator.webdriver etc.
	opts = append(opts, chromedp.Flag("disable-blink-features", "AutomationControlled"))

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
	if browserCtx == nil {
		return
	}
	// gracefully close the browser (Cancel waits for it, browserStop doesn't)
	chromedp.Cancel(browserCtx)
	browserStop()
	allocCancel()
	browserCtx, allocCtx = nil, nil
}

// GetHTML navigates to the given URL in a new tab, waits until waitSelector
// is visible (skipped if empty) and returns the rendered HTML. Cookies and
// the browser user agent are copied to the http package so subsequent plain
// HTTP requests (e.g. image downloads) reuse the browser session.
//
// A timeout of 0 uses a sensible default: 1 minute headless, 5 minutes in
// visible mode (leaving time for the user to solve interactive challenges).
func GetHTML(url, waitSelector string, timeout time.Duration) (string, error) {
	mu.Lock()
	defer mu.Unlock()

	if timeout <= 0 {
		timeout = time.Minute
		if visible {
			timeout = 5 * time.Minute
		}
	}

	if err := start(); err != nil {
		return "", err
	}

	// new tab sharing the browser (and thus cookies/clearance tokens)
	tab, closeTab := chromedp.NewContext(browserCtx)
	defer closeTab()
	ctx, cancel := context.WithTimeout(tab, timeout)
	defer cancel()

	var html string
	actions := []chromedp.Action{
		chromedp.Navigate(url),
	}
	if waitSelector != "" {
		actions = append(actions, chromedp.WaitVisible(waitSelector, chromedp.ByQuery))
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
			hint := ""
			if !visible {
				hint = ": some protections (e.g. cloudflare) only pass in a visible browser, try again with --browser-visible"
			}
			return "", fmt.Errorf("timed out waiting for %q at %s%s", waitSelector, url, hint)
		}
		return "", err
	}

	return html, nil
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
