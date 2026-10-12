// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package browser

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

// closeTimeout bounds the graceful close of the browser before its process is
// killed
const closeTimeout = 5 * time.Second

// launchChrome starts a Chrome-like browser and its first tab. If abort ends
// first, the half-started browser is killed and ErrClosed returned.
func launchChrome(abort context.Context, headed bool) (*instance, error) {
	execPath, err := findBrowser(systemHost())
	if err != nil {
		return nil, err
	}

	opts := []chromedp.ExecAllocatorOption{chromedp.ExecPath(execPath)}
	opts = append(opts, chromedp.DefaultExecAllocatorOptions[:]...)
	if headed {
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

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	browserCtx, browserStop := chromedp.NewContext(allocCtx)
	// killing the browser ends the Run and hideHeadless below early
	unwatch := context.AfterFunc(abort, func() {
		browserStop()
		allocCancel()
	})

	// starting the browser eagerly gives a nicer error when chrome is missing
	if err := chromedp.Run(browserCtx); err != nil {
		unwatch()
		browserStop()
		allocCancel()
		if abort.Err() != nil {
			return nil, ErrClosed
		}
		if strings.Contains(err.Error(), "executable file not found") {
			return nil, ErrNoBrowser
		}
		return nil, fmt.Errorf("error starting browser: %w", err)
	}

	c := chromedp.FromContext(browserCtx)
	// on Windows, ties the browser's process tree to this process, so it
	// can't outlive it (see proc_windows.go); a no-op elsewhere
	guard := guardProcess(c.Browser.Process())

	// lost ends when the browser goes away: chromedp cancels browserCtx when
	// the connection drops (the process exited), but a closed tab in a
	// browser that keeps running in the background only shows as an event
	lost, lose := context.WithCancel(context.Background())
	context.AfterFunc(browserCtx, lose)
	tabID := c.Target.TargetID
	chromedp.ListenBrowser(browserCtx, func(ev any) {
		switch e := ev.(type) {
		case *target.EventTargetDestroyed:
			if e.TargetID == tabID {
				lose()
			}
		case *target.EventTargetCrashed:
			if e.TargetID == tabID {
				lose()
			}
		}
	})

	if !headed {
		hideHeadless(browserCtx)
	}
	if !unwatch() {
		// aborted: the browser is being killed
		browserStop()
		guard.release()
		allocCancel()
		lose()
		return nil, ErrClosed
	}

	var once sync.Once
	return &instance{
		ctx:    browserCtx,
		lost:   lost,
		headed: headed,
		close: func() {
			once.Do(func() {
				// graceful first (Cancel waits for the process to exit, up
				// to the deadline), then the process is killed regardless
				ctx, cancel := context.WithTimeout(browserCtx, closeTimeout)
				_ = chromedp.Cancel(ctx)
				cancel()
				browserStop()
				// helpers first, so they're gone before chromedp removes
				// the profile dir once the process exits
				guard.release()
				// cancels the process context (killing it, if it's still
				// up) and waits for it to exit
				allocCancel()
				lose()
			})
		},
	}, nil
}

// hideHeadless strips "Headless" from the user agent of a headless browser,
// and from its client hints (Sec-CH-UA). HeadlessChrome is a giveaway
// Cloudflare blocks on, and the user agent is harvested for plain HTTP
// requests too (see harvestSession), where it got later requests to any site
// 403s. Best effort: the page still loads without it.
func hideHeadless(browserCtx context.Context) {
	ctx, cancel := context.WithTimeout(browserCtx, 10*time.Second)
	defer cancel()
	_ = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var ua string
		if err := chromedp.Evaluate(`navigator.userAgent`, &ua).Do(ctx); err != nil {
			return err
		}
		if !strings.Contains(ua, "Headless") {
			return nil
		}
		override := emulation.SetUserAgentOverride(strings.ReplaceAll(ua, "HeadlessChrome", "Chrome"))
		// an override without metadata empties the client hints, and
		// about:blank doesn't expose them (not a secure context), so they're
		// read from an internal page
		var md *emulation.UserAgentMetadata
		err := chromedp.Run(ctx,
			chromedp.Navigate("chrome://version"),
			chromedp.Evaluate(clientHintsJS, &md, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
				return p.WithAwaitPromise(true)
			}),
			chromedp.Navigate("about:blank"),
		)
		if err == nil && md != nil {
			unheadBrands(md.Brands)
			unheadBrands(md.FullVersionList)
			override = override.WithUserAgentMetadata(md)
		}
		return override.Do(ctx)
	}))
}

// clientHintsJS reads the browser's client hints in the shape of
// emulation.UserAgentMetadata, or null where they aren't exposed
const clientHintsJS = `(async () => {
	const d = navigator.userAgentData;
	if (!d) return null;
	const h = await d.getHighEntropyValues(["platform", "platformVersion", "architecture", "model", "bitness", "fullVersionList", "wow64"]);
	return {brands: d.brands, mobile: d.mobile, platform: h.platform || "", platformVersion: h.platformVersion || "",
		architecture: h.architecture || "", model: h.model || "", bitness: h.bitness || "",
		fullVersionList: h.fullVersionList || [], wow64: !!h.wow64};
})()`

// unheadBrands renames the HeadlessChrome brand to the one Chrome reports
func unheadBrands(brands []*emulation.UserAgentBrandVersion) {
	for _, b := range brands {
		if b != nil && b.Brand == "HeadlessChrome" {
			b.Brand = "Google Chrome"
		}
	}
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
