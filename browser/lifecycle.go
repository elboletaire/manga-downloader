// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package browser

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elboletaire/manga-downloader/notice"
)

// instance is a running browser and the tab every page is rendered in
type instance struct {
	// ctx is the chromedp context of the browser's initial tab
	ctx context.Context
	// lost is done once the browser process or its tab is gone (the window
	// was closed, it crashed, or close ran)
	lost context.Context
	// headed tells a visible window from a headless browser
	headed bool
	// close stops the browser and waits for its processes to end, within a
	// bounded time. Safe to call more than once.
	close func()
}

// launch starts a browser; replaced in tests so the lifecycle can be checked
// without a real one
var launch = launchChrome

var (
	// visible is the user's choice of a visible window (SetVisible). Atomic,
	// as the GUI sets it while a render may hold mu.
	visible atomic.Bool

	// mu serializes everything done in the shared browser, and guards inst
	// and escalated
	mu   sync.Mutex
	inst *instance
	// escalated is set when a headless attempt hit a challenge and the
	// browser was reopened visible. It lasts as long as that browser does,
	// so the next pages of the same site skip the wasted headless probe.
	escalated bool

	// abortCtx is cancelled by Close, so whatever runs in the browser ends
	// at once instead of Close waiting behind it for mu (a visible window
	// waits up to visibleTimeout for a challenge). Guarded by abortMu, not
	// mu, for that very reason.
	abortMu     sync.Mutex
	abortCtx    context.Context
	abortCancel context.CancelFunc
)

func init() {
	abortCtx, abortCancel = context.WithCancel(context.Background())
}

// ErrClosed is returned by a page load that was still running when Close was
// called (the app quitting, or the CLI interrupted)
var ErrClosed = errors.New("the browser was closed before the page finished loading")

// goneError is returned when the browser went away mid-load: its window was
// closed, or it crashed
type goneError struct{ url string }

func (e *goneError) Error() string {
	return fmt.Sprintf("the browser closed before %s finished loading (was its window closed?)", hostOf(e.url))
}

// SetVisible toggles headed mode for the next browser started. A browser that
// is already running keeps its mode until it is closed.
func SetVisible(v bool) {
	visible.Store(v)
}

// lock takes mu for a page load and returns the abort context to run it
// with. The context is read before waiting for mu, so a load queued behind
// one that Close is ending gets aborted too, instead of starting a new
// browser after Close returned.
func lock() context.Context {
	abortMu.Lock()
	abort := abortCtx
	abortMu.Unlock()
	mu.Lock()
	return abort
}

// start makes sure a live browser is running, replacing one that died since
// it was last used. host names the site it's started for, for the notices.
// Callers must hold mu.
func start(host string) error {
	if inst != nil {
		if inst.lost.Err() == nil {
			return nil
		}
		notice.Warn("The browser closed unexpectedly (its window was closed, or it crashed); starting a new one.")
		teardown()
	}

	headed := visible.Load() || escalated
	if headed {
		notice.Info("%s needs a real browser: starting it with a visible window (this can take a few seconds)...", host)
	} else {
		notice.Info("%s needs a real browser: starting it in the background (this can take a few seconds)...", host)
	}
	i, err := launch(headed)
	if err != nil {
		return err
	}
	inst = i
	return nil
}

// teardown stops the browser, if one runs, and forgets any escalation to a
// visible window, so the next start begins from the user's choice again.
// Callers must hold mu.
func teardown() {
	escalated = false
	if inst == nil {
		return
	}
	inst.close()
	inst = nil
}

// goVisible replaces the current headless browser with a visible one.
// Callers must hold mu.
func goVisible(host string) error {
	teardown()
	escalated = true
	return start(host)
}

// Close shuts down the shared browser, if it was started, ending any page
// load still running in it. Safe to call multiple times; must be called
// before exiting, or the browser may be left running in the background.
// The browser starts again on the next page load.
func Close() {
	abortMu.Lock()
	abortCancel()
	abortMu.Unlock()

	mu.Lock()
	defer mu.Unlock()
	stopIdle()
	teardown()

	abortMu.Lock()
	abortCtx, abortCancel = context.WithCancel(context.Background())
	abortMu.Unlock()
}

// idle closes the browser once it's been unused for idleTimeout (zero keeps
// it until Close). idleGen tells the timer whether the browser was used since
// it was armed. Both guarded by mu.
var (
	idleTimeout time.Duration
	idleTimer   *time.Timer
	idleGen     uint64
)

// SetIdleTimeout closes the browser after it's gone unused for d, so a
// long-running app doesn't keep it (and possibly its window) open after the
// pages that needed it are done. Zero, the default, keeps it until Close.
func SetIdleTimeout(d time.Duration) {
	mu.Lock()
	defer mu.Unlock()
	idleTimeout = d
	stopIdle()
}

// stopIdle disarms the idle timer. Callers must hold mu.
func stopIdle() {
	idleGen++
	if idleTimer != nil {
		idleTimer.Stop()
		idleTimer = nil
	}
}

// armIdle starts the idle countdown after a page load. Callers must hold mu.
func armIdle() {
	stopIdle()
	if idleTimeout <= 0 || inst == nil {
		return
	}
	gen := idleGen
	idleTimer = time.AfterFunc(idleTimeout, func() {
		mu.Lock()
		defer mu.Unlock()
		if gen == idleGen {
			teardown()
		}
	})
}

// pageOp is one page operation run in the shared browser's tab. ctx ends when
// the attempt times out, is aborted or the browser is lost; rearm restarts
// the timeout (for operations that keep working once the page has rendered).
// It returns a *challengeError when the wait selector never matched.
type pageOp func(ctx context.Context, rearm func(time.Duration)) error

// runPage runs op in the shared browser, starting it if needed. abort is the
// context from lock.
//
// If escalate is set and a headless attempt hits a challenge, it transparently
// reopens a visible window and retries. After any failure the browser is
// closed, so the next request starts a fresh one instead of reusing one that
// may be dead or stuck. That's deliberate: a failed browser is always
// discarded, at the cost of passing a check again. Callers must hold mu.
func runPage(abort context.Context, pageURL, waitSelector string, timeout time.Duration, escalate bool, op pageOp) error {
	stopIdle()
	host := hostOf(pageURL)

	err := attempt(host, pageURL, timeout, abort, op)
	var ce *challengeError
	if errors.As(err, &ce) && escalate && inst != nil && !inst.headed {
		notice.Warn("%s didn't load in a headless browser (probably a Cloudflare check).", host)
		notice.Warn("Opening a visible browser window to pass the check: solve it there if asked...")
		if err = goVisible(host); err == nil {
			err = attempt(host, pageURL, visibleTimeout, abort, op)
		}
	}
	if err == nil {
		armIdle()
		return nil
	}

	if inst != nil && !errors.Is(err, ErrClosed) {
		notice.Info("Closing the browser after the failure, so the next try starts a fresh one.")
	}
	teardown()

	if errors.As(err, &ce) {
		return fmt.Errorf("%w: the challenge may not have been solved in time", err)
	}
	return err
}

// attempt is a single run of op, bounded by timeout (zero picks the default
// for the browser's mode). Callers must hold mu.
func attempt(host, pageURL string, timeout time.Duration, abort context.Context, op pageOp) error {
	if abort.Err() != nil {
		return ErrClosed
	}
	if err := start(host); err != nil {
		return err
	}
	b := inst

	if timeout <= 0 {
		timeout = headlessProbeTimeout
		if b.headed {
			timeout = visibleTimeout
		}
	}

	ctx, cancel := context.WithCancel(b.ctx)
	defer cancel()
	timer := time.AfterFunc(timeout, cancel)
	defer timer.Stop()
	defer context.AfterFunc(abort, cancel)()
	defer context.AfterFunc(b.lost, cancel)()

	if b.headed {
		startChallengeClicker(ctx)
	}
	go reportWaiting(ctx, host, b.headed, waitNoticeAfter, waitNoticeEvery)

	err := op(ctx, func(d time.Duration) { timer.Reset(d) })
	switch {
	case err == nil:
		return nil
	case abort.Err() != nil:
		return ErrClosed
	case b.lost.Err() != nil:
		return &goneError{url: pageURL}
	}
	return err
}

// waitNotice is when the first "still waiting" notice shows, and how often it
// repeats after that. Vars for the tests, guarded by mu.
var (
	waitNoticeAfter = 10 * time.Second
	waitNoticeEvery = 30 * time.Second
)

// reportWaiting tells the user a slow page load is still going, so nothing
// looks stuck while a challenge is being waited out. Returns when ctx ends.
func reportWaiting(ctx context.Context, host string, headed bool, after, every time.Duration) {
	began := time.Now()
	t := time.NewTimer(after)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		waited := time.Since(began).Round(time.Second)
		if headed {
			notice.Info("Waiting for %s to load in the browser window (%s); solve the check there if one is showing...", host, waited)
		} else {
			notice.Info("Waiting for %s to load in the browser (%s); a Cloudflare check may be showing...", host, waited)
		}
		t.Reset(every)
	}
}
