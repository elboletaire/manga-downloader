// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package browser

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	cdpbrowser "github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

var (
	// hostsMu guards required and needed
	hostsMu sync.Mutex
	// required holds the hosts grabbers declared never pass a headless
	// browser (RequireVisible)
	required = map[string]bool{}
	// needed holds the hosts that escalated to a visible window, for the
	// rest of the process: closing the browser doesn't make them pass
	// headless any better
	needed = map[string]bool{}
)

// RequireVisible declares that host (and its subdomains) never passes a
// headless browser, e.g. it serves a captcha of its own to it, so its pages
// start in a visible window right away instead of after a wasted headless
// probe. Grabbers call it from an init function.
func RequireVisible(host string) {
	hostsMu.Lock()
	defer hostsMu.Unlock()
	required[normalHost(host)] = true
}

// NeedsVisible reports whether pages of rawURL's site (a URL or a bare host)
// start in a visible window on their own: the site was declared with
// RequireVisible, or it needed the escalation earlier in this process.
func NeedsVisible(rawURL string) bool {
	host := normalHost(hostOf(rawURL))
	if host == "" {
		return false
	}
	hostsMu.Lock()
	defer hostsMu.Unlock()
	return matchHost(required, host) || matchHost(needed, host)
}

// rememberVisible records that host needed a visible window. It returns a
// func that forgets it again, for a visible start that failed.
func rememberVisible(host string) (forget func()) {
	host = normalHost(host)
	hostsMu.Lock()
	defer hostsMu.Unlock()
	if needed[host] {
		return func() {}
	}
	needed[host] = true
	return func() {
		hostsMu.Lock()
		defer hostsMu.Unlock()
		delete(needed, host)
	}
}

// normalHost lowercases host and drops a www. prefix
func normalHost(host string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(host)), "www.")
}

// matchHost reports whether host, or a domain it's under, is in set
func matchHost(set map[string]bool, host string) bool {
	for h := range set {
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	return false
}

// Attention is reported when a visible browser window may need the user (a
// check or captcha to solve there), and again with Active unset once it no
// longer does: the page rendered, or its load ended.
type Attention struct {
	Active bool
	// Host is the site the page is from
	Host string
	// Browser is the browser's name ("Brave"), "" when unknown
	Browser string
}

// Message is the text shown to the user for an active Attention
func (a Attention) Message() string {
	who := "A browser window opened"
	if a.Browser != "" {
		who = a.Browser + " opened a window"
	}
	return fmt.Sprintf("%s to pass a check on %s. There may be a captcha or puzzle to solve there; the download continues by itself once it's done.", who, a.Host)
}

var (
	attentionMu sync.Mutex
	attentionFn func(Attention)
)

// SetAttentionHandler sets the function told about Attention changes, nil for
// none. It's called from the goroutine loading the page, and must not call
// back into the package's page loads.
func SetAttentionHandler(fn func(Attention)) {
	attentionMu.Lock()
	defer attentionMu.Unlock()
	attentionFn = fn
}

// alert is the Attention of one page load: raised at most once, and ended
// once, after which raising it does nothing
type alert struct {
	mu     sync.Mutex
	a      Attention
	front  func()
	raised bool
	ended  bool
}

// raise reports the attention and brings the window to the front, unless
// it was already raised or the load is over
func (al *alert) raise() {
	al.mu.Lock()
	defer al.mu.Unlock()
	if al.raised || al.ended {
		return
	}
	al.raised = true
	if al.front != nil {
		go al.front()
	}
	report(Attention{Active: true, Host: al.a.Host, Browser: al.a.Browser})
}

// end reports the attention is over, if it was raised
func (al *alert) end() {
	al.mu.Lock()
	defer al.mu.Unlock()
	if al.ended {
		return
	}
	al.ended = true
	if al.raised {
		report(Attention{Host: al.a.Host, Browser: al.a.Browser})
	}
}

func report(a Attention) {
	attentionMu.Lock()
	fn := attentionFn
	attentionMu.Unlock()
	if fn != nil {
		fn(a)
	}
}

// BringToFront brings the visible browser window to the front, if one is
// open. Best effort: some systems (Windows) may refuse to steal the focus.
func BringToFront() {
	if i := current.Load(); i != nil && i.headed && i.front != nil {
		i.front()
	}
}

// bringToFront restores the window of the tab in ctx and raises it
func bringToFront(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		// a minimized window has to be restored before it can come forward
		if id, bounds, err := cdpbrowser.GetWindowForTarget().Do(ctx); err == nil && needsRestore(bounds) {
			_ = cdpbrowser.SetWindowBounds(id, &cdpbrowser.Bounds{WindowState: cdpbrowser.WindowStateNormal}).Do(ctx)
		}
		return page.BringToFront().Do(ctx)
	}))
}

// needsRestore reports whether a window must be restored to come forward:
// only a minimized one, so a maximized or fullscreen window stays as it is
func needsRestore(b *cdpbrowser.Bounds) bool {
	return b != nil && b.WindowState == cdpbrowser.WindowStateMinimized
}

// browserName is the name users know the browser at execPath by, "" when
// it isn't one of the usual ones
func browserName(execPath string) string {
	full := strings.ToLower(strings.ReplaceAll(execPath, `\`, "/"))
	base := strings.TrimSuffix(filepath.Base(full), ".exe")
	switch {
	case strings.Contains(base, "brave"):
		return "Brave"
	case strings.Contains(base, "msedge"), strings.Contains(base, "edge"):
		return "Microsoft Edge"
	// Chromium's Windows build is a chrome.exe too
	case strings.Contains(full, "chromium"):
		return "Chromium"
	case strings.Contains(base, "chrome") && !strings.Contains(base, "headless"):
		return "Google Chrome"
	}
	return ""
}
