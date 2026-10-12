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
	// name is the browser's name for the user ("Brave"), "" when unknown
	name string
	// front brings the window to the front, best effort; nil when headless
	front func()
	// needsUser is set on a window opened because the site needs one
	// (declared, remembered or escalated), where a check may wait for the
	// user. greet is set until the first load in it raised the alert.
	needsUser bool
	greet     bool
	// close stops the browser and waits for its processes to end, within a
	// bounded time. Safe to call more than once.
	close func()
}

// launch starts a browser, giving up when abort ends; replaced in tests so
// the lifecycle can be checked without a real one
var launch = launchChrome

var (
	// visible is the user's choice of a visible window (SetVisible). Atomic,
	// as the GUI sets it while a render may hold mu.
	visible atomic.Bool

	// mu serializes everything done in the shared browser, and guards inst
	mu   sync.Mutex
	inst *instance
	// current mirrors inst for BringToFront, which mustn't wait for mu (a
	// load holds it while a check waits for the user)
	current atomic.Pointer[instance]

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
	if lockHook != nil {
		lockHook()
	}
	mu.Lock()
	return abort
}

// lockHook, when set, runs in lock between reading the abort context and
// waiting for mu. Tests only.
var lockHook func()

// start makes sure a live browser is running, replacing one that died since
// it was last used, or a headless one when host needs a visible window (see
// NeedsVisible). host names the site it's started for, for the notices.
// Callers must hold mu.
func start(abort context.Context, host string) error {
	auto := NeedsVisible(host)
	if inst != nil {
		switch {
		case inst.lost.Err() != nil:
			notice.Warn("The browser closed unexpectedly (its window was closed, or it crashed); starting a new one.")
		case auto && !inst.headed:
			// left by another site: this one needs a window
		default:
			return nil
		}
		teardown()
	}

	headed := visible.Load() || auto
	if headed {
		notice.Info("%s needs a real browser: starting it with a visible window (this can take a few seconds)...", host)
	} else {
		notice.Info("%s needs a real browser: starting it in the background (this can take a few seconds)...", host)
	}
	i, err := launch(abort, headed)
	if err != nil {
		return err
	}
	if headed && auto {
		i.needsUser, i.greet = true, true
	}
	inst = i
	current.Store(i)
	return nil
}

// teardown stops the browser, if one runs. Callers must hold mu.
func teardown() {
	if inst == nil {
		return
	}
	current.Store(nil)
	inst.close()
	inst = nil
}

// goVisible replaces the current headless browser with a visible one, and
// remembers host needs it for the rest of the process. Callers must hold mu.
func goVisible(abort context.Context, host string) error {
	teardown()
	forget := rememberVisible(host)
	err := start(abort, host)
	if err != nil {
		// a window that can't open (no display) mustn't stick
		forget()
	}
	return err
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
	teardown()

	abortMu.Lock()
	abortCtx, abortCancel = context.WithCancel(context.Background())
	abortMu.Unlock()
}

// holds counts the Hold calls not yet released, guarded by mu
var holds int

// Hold keeps the browser open until the returned release is called, for work
// (a download run) that uses it now and then. The browser closes when the last
// holder releases, so it doesn't linger after the operation is done. release is
// safe to call more than once.
func Hold() (release func()) {
	mu.Lock()
	defer mu.Unlock()
	holds++
	var once sync.Once
	return func() {
		once.Do(func() {
			mu.Lock()
			defer mu.Unlock()
			holds--
			if holds <= 0 && inst != nil {
				teardown()
			}
		})
	}
}

// pageOp is one page operation run in the shared browser's tab. ctx ends when
// the attempt times out, is aborted or the browser is lost. It returns a
// *challengeError when the wait selector never matched.
type pageOp func(ctx context.Context, h hooks) error

// hooks let a pageOp report its progress to the attempt running it
type hooks struct {
	// rearm restarts the timeout, for operations that keep working once the
	// page has rendered
	rearm func(time.Duration)
	// rendered says the page is in, which stops the "still waiting" notices:
	// what follows (scrolling, pagination) is slow by design
	rendered func()
}

// runPage runs op in the shared browser, starting it if needed. abort is the
// context from lock.
//
// If escalate is set and a headless attempt hits a challenge, it transparently
// reopens a visible window and retries. After any failure the browser is
// closed, so the next request starts a fresh one instead of reusing one that
// may be dead or stuck. That's deliberate: a failed browser is always
// discarded, at the cost of passing a check again. Callers must hold mu.
func runPage(abort context.Context, pageURL string, timeout time.Duration, escalate bool, op pageOp) error {
	host := hostOf(pageURL)

	err := attempt(host, pageURL, timeout, abort, op)
	var ce *challengeError
	if errors.As(err, &ce) && escalate && inst != nil && !inst.headed {
		notice.Warn("%s didn't load in a headless browser (probably a Cloudflare check).", host)
		notice.Warn("Opening a visible browser window to pass the check: solve it there if asked...")
		if err = goVisible(abort, host); err == nil {
			err = attempt(host, pageURL, visibleTimeout, abort, op)
		}
	}
	if err != nil {
		if inst != nil && !errors.Is(err, ErrClosed) {
			notice.Info("Closing the browser after the failure, so the next try starts a fresh one.")
		}
		teardown()

		if errors.As(err, &ce) {
			return fmt.Errorf("%w: the challenge may not have been solved in time", err)
		}
		return err
	}
	return nil
}

// attempt is a single run of op, bounded by timeout (zero picks the default
// for the browser's mode). Callers must hold mu.
func attempt(host, pageURL string, timeout time.Duration, abort context.Context, op pageOp) error {
	if abort.Err() != nil {
		return ErrClosed
	}
	if err := start(abort, host); err != nil {
		if abort.Err() != nil {
			return ErrClosed
		}
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

	// a window opened for the site tells the user at once; one reused for
	// a later page only once the page is slow to render
	al := &alert{a: Attention{Host: host, Browser: b.name}, front: b.front}
	defer al.end()
	// a site that needs the window may wait for the user in one opened for
	// another reason (the user's choice, another site) too
	raise := func() {}
	if b.headed && (b.needsUser || NeedsVisible(host)) {
		raise = al.raise
	}
	if b.greet {
		b.greet = false
		al.raise()
	}

	waiting, stopWaiting := context.WithCancel(ctx)
	rendered := func() {
		stopWaiting()
		al.end()
	}
	defer stopWaiting()
	go reportWaiting(waiting, host, b.headed, waitNoticeAfter, waitNoticeEvery, raise)

	err := op(ctx, hooks{
		rearm:    func(d time.Duration) { timer.Reset(d) },
		rendered: rendered,
	})
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
// looks stuck while a challenge is being waited out, calling slow with each
// notice. Returns when ctx ends.
func reportWaiting(ctx context.Context, host string, headed bool, after, every time.Duration, slow func()) {
	began := time.Now()
	t := time.NewTimer(after)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		slow()
		waited := time.Since(began).Round(time.Second)
		if headed {
			notice.Info("Waiting for %s to load in the browser window (%s); solve the check there if one is showing...", host, waited)
		} else {
			notice.Info("Waiting for %s to load in the browser (%s); a Cloudflare check may be showing...", host, waited)
		}
		t.Reset(every)
	}
}
