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
//
// After a failed load the browser is closed, so the next one starts fresh
// (see runPage in lifecycle.go), and Close must be called before exiting.
package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/elboletaire/manga-downloader/http"
)

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

// ErrNoBrowser is returned when no Chrome-like browser is installed
var ErrNoBrowser = errors.New(
	"no Chrome-like browser found: this site needs a real browser to be scraped.\n" +
		"Install Google Chrome, Chromium, Brave or Edge and try again, or set " +
		browserOverrideEnv + " to the path of one",
)

// timeouts for the first render attempt. The headless probe is kept short:
// a page behind a challenge will never pass headless, so instead of making the
// user wait we escalate to a visible browser as soon as it times out.
const (
	headlessProbeTimeout = 30 * time.Second
	// paginationTimeout bounds clicking through a paginated list once the page
	// has rendered
	paginationTimeout = 5 * time.Minute
	// afterRenderTimeout bounds what a render does once its wait selector
	// matched (scrolling, settling, the snapshot)
	afterRenderTimeout = 2 * time.Minute
)

// visibleTimeout bounds a load in a visible window, where a challenge may
// need solving by hand. A var for the tests, guarded by mu.
var visibleTimeout = 5 * time.Minute

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

// Scroll says how GetHTMLWithScroll scrolls a page
type Scroll struct {
	// Container selects the element that scrolls; empty (or no match)
	// scrolls the document. Readers that scroll an inner element ignore
	// window.scrollTo entirely.
	Container string
	// Done is a JS expression that's true once everything lazy has mounted.
	// Empty, or never true, stops at the bottom instead.
	Done string
	// Pause is the wait after each step
	Pause time.Duration
}

// scrollSettleSteps is how many steps a scroll keeps waiting at the bottom
// for Done before giving up on it, so the last pages can still mount
const scrollSettleSteps = 3

// GetHTMLWithScroll is like GetHTML, but after waitSelector matches it scrolls
// the page down one viewport at a time until s.Done holds or the bottom is
// reached, then takes the final HTML snapshot. Some readers virtualize or
// lazy-mount their page images via an IntersectionObserver, so a plain
// GetHTML only captures the handful of pages near the top; stepping a
// viewport at a time mounts every one of them, where bigger jumps skip some.
func GetHTMLWithScroll(url, waitSelector string, s Scroll, timeout time.Duration) (string, error) {
	return getHTML(url, waitSelector, timeout, []chromedp.Action{chromedp.ActionFunc(func(ctx context.Context) error {
		return scroll(ctx, s)
	})})
}

// scrollStepJS returns the JS for one scroll step: it checks done first,
// then scrolls the container a viewport down (instantly: a smooth scroll
// gets cut short by the next step) and reports whether it was already at
// the bottom
func scrollStepJS(s Scroll) string {
	done := "false"
	if s.Done != "" {
		done = s.Done
	}
	return fmt.Sprintf(`(function(){
		if (%s) return "done";
		var el = (%q && document.querySelector(%q)) || document.scrollingElement || document.documentElement;
		var bottom = el.scrollTop + el.clientHeight >= el.scrollHeight - 2;
		el.scrollBy({top: el.clientHeight, behavior: "instant"});
		return bottom ? "bottom" : "scrolled";
	})()`, done, s.Container, s.Container)
}

// scroll steps through the page as described by s
func scroll(ctx context.Context, s Scroll) error {
	js := scrollStepJS(s)
	atBottom := 0
	// no step cap: afterRenderTimeout bounds a page that keeps growing
	for {
		var state string
		if err := chromedp.Evaluate(js, &state).Do(ctx); err != nil {
			return err
		}
		switch state {
		case "done":
			return nil
		case "bottom":
			atBottom++
			if atBottom > scrollSettleSteps {
				return nil
			}
		default:
			atBottom = 0
		}
		if err := chromedp.Sleep(s.Pause).Do(ctx); err != nil {
			return err
		}
	}
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
	abort := lock()
	defer mu.Unlock()

	t := timeout
	if t <= 0 {
		t = visibleTimeout
	}
	var html string
	err := runPage(abort, seriesURL, t, false, func(ctx context.Context, h hooks) error {
		var err error
		html, err = readerHTML(ctx, h, seriesURL, tabSelector, paginationSelector, linkSelector, imgSelector, urlSubstr)
		return err
	})
	return html, err
}

// readerHTML is GetReaderHTML's single attempt
func readerHTML(ctx context.Context, h hooks, seriesURL, tabSelector, paginationSelector, linkSelector, imgSelector, urlSubstr string) (string, error) {
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
	h.rendered()

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
// setupJS, if non-empty, runs once the initial responses are in. It resolves to
// {"wait": bool, "result": any} (see parseSetup): result is returned as setup
// for the caller to read, and wait says the script made the page issue a new
// request, so the responses are awaited before the call returns.
//
// Like GetHTML it tries headless first and transparently escalates to a visible
// window if the wait selector times out (typically a challenge).
func GetAPIResponses(pageURL, waitSelector, urlSubstr, nextSelector string, maxClicks int, timeout time.Duration, setupJS string) (responses []APIResponse, setup json.RawMessage, err error) {
	abort := lock()
	defer mu.Unlock()

	err = runPage(abort, pageURL, timeout, true, func(ctx context.Context, h hooks) error {
		var err error
		responses, setup, err = captureAPI(ctx, h, pageURL, waitSelector, urlSubstr, nextSelector, maxClicks, setupJS)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	return responses, setup, nil
}

// captureAPI performs a single render capturing the response bodies whose URL
// contains urlSubstr, optionally clicking through nextSelector pagination.
//
// The attempt's timeout only has to bound the initial render (that's what
// tells a challenge apart), so it's re-armed with a full paginationTimeout
// once the page has loaded: clicking through hundreds of chapters takes far
// longer than the headless probe is allowed to.
func captureAPI(ctx context.Context, h hooks, pageURL, waitSelector, urlSubstr, nextSelector string, maxClicks int, setupJS string) ([]APIResponse, json.RawMessage, error) {
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
			return nil, nil, &challengeError{url: pageURL, selector: waitSelector}
		}
		return nil, nil, err
	}

	count := func() int {
		cmu.Lock()
		defer cmu.Unlock()
		return len(entries)
	}

	// wait for the initial matching response(s) the SPA fires on load, so we
	// don't fetch bodies before they exist
	waitForGrowth(ctx, count, 0)
	h.rendered()
	h.rearm(paginationTimeout)

	// page-specific setup (e.g. switching the chapter list's language); the
	// script is async and resolves to what parseSetup reads
	var setupResult json.RawMessage
	if setupJS != "" {
		before := count()
		var raw string
		if err := chromedp.Run(ctx, chromedp.Evaluate(setupJS, &raw, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		})); err != nil {
			return nil, nil, err
		}
		wait, result, err := parseSetup(raw)
		if err != nil {
			return nil, nil, err
		}
		setupResult = result
		if wait {
			waitForGrowth(ctx, count, before)
		}
	}

	if nextSelector != "" {
		clickJS := fmt.Sprintf(
			`(function(){var b=document.querySelector(%q);if(!b)return "gone";if(b.disabled||b.getAttribute("aria-disabled")==="true")return "disabled";b.click();return "clicked";})()`,
			nextSelector,
		)
		for i := 0; i < maxClicks; i++ {
			before := count()
			var status string
			if err := chromedp.Run(ctx, chromedp.Evaluate(clickJS, &status)); err != nil {
				return nil, nil, err
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
	return out, setupResult, nil
}

// parseSetup reads what a setup script resolved to: whether it caused a new
// request (wait), and the result the caller reads
func parseSetup(raw string) (wait bool, result json.RawMessage, err error) {
	var env struct {
		Wait   bool            `json:"wait"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		return false, nil, fmt.Errorf("the page setup didn't resolve to JSON: %w", err)
	}

	return env.Wait, env.Result, nil
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

// getHTML is the shared implementation behind the GetHTML variants: it renders
// url in a headless browser and, if the wait selector times out (typically a
// cloudflare/JS challenge), transparently reopens a visible window and
// retries.
func getHTML(url, waitSelector string, timeout time.Duration, afterWait []chromedp.Action) (string, error) {
	abort := lock()
	defer mu.Unlock()

	var html string
	err := runPage(abort, url, timeout, true, func(ctx context.Context, h hooks) error {
		var err error
		html, err = render(ctx, h, url, waitSelector, afterWait)
		return err
	})
	return html, err
}

// render performs a single navigation in the shared browser, reusing its
// initial tab (a new tab would spawn in the background, leaving the blank
// first tab in front and hiding the page). afterWait, if any, runs once
// waitSelector matches, before the snapshot.
func render(ctx context.Context, h hooks, url, waitSelector string, afterWait []chromedp.Action) (string, error) {
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
	if waitSelector != "" {
		actions = append(actions, chromedp.WaitVisible(waitSelector, chromedp.ByQuery))
	}
	matched := false
	actions = append(actions, chromedp.ActionFunc(func(context.Context) error {
		matched = true
		h.rendered()
		// what follows (scrolling, settling) gets its own budget, so it
		// can't eat the probe's and pass for a challenge
		h.rearm(afterRenderTimeout)
		return nil
	}))
	actions = append(actions, afterWait...)
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
		return "", renderError(ctx, err, url, waitSelector, matched)
	}

	return html, nil
}

// renderError explains a failed render: a timeout before waitSelector matched
// is a challenge, one after it is just a slow page
func renderError(ctx context.Context, err error, url, waitSelector string, matched bool) error {
	if ctx.Err() == nil || waitSelector == "" {
		return err
	}
	if !matched {
		return &challengeError{url: url, selector: waitSelector}
	}
	return fmt.Errorf("%s loaded but didn't finish in time: %w", hostOf(url), err)
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
