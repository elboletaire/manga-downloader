// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package browser

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/elboletaire/manga-downloader/notice"
)

// fakeBrowser stands in for launchChrome, recording what was started and
// closed
type fakeBrowser struct {
	mu        sync.Mutex
	launched  []bool // headed, per launch
	closed    int
	instances []*instance
	cancels   []context.CancelFunc
	failNext  error
	// slow, when set, makes launch block until aborted, closing slow once
	// it's started
	slow chan struct{}
	// halfStarted counts the slow launches killed by an abort
	halfStarted int
	// fronts counts the times a window was brought to the front
	fronts int
}

func (f *fakeBrowser) launch(abort context.Context, headed bool) (*instance, error) {
	if f.slow != nil {
		close(f.slow)
		// a browser that takes long to start: only abort ends it
		<-abort.Done()
		f.mu.Lock()
		f.halfStarted++
		f.mu.Unlock()
		return nil, ErrClosed
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext != nil {
		err := f.failNext
		f.failNext = nil
		return nil, err
	}
	f.launched = append(f.launched, headed)
	ctx, cancel := context.WithCancel(context.Background())
	var once sync.Once
	i := &instance{ctx: ctx, lost: ctx, headed: headed, name: "Fakefox"}
	if headed {
		i.front = func() {
			f.mu.Lock()
			f.fronts++
			f.mu.Unlock()
		}
	}
	i.close = func() {
		once.Do(func() {
			f.mu.Lock()
			f.closed++
			f.mu.Unlock()
			cancel()
		})
	}
	f.instances = append(f.instances, i)
	f.cancels = append(f.cancels, cancel)
	return i, nil
}

func (f *fakeBrowser) counts() (launched, closed int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.launched), f.closed
}

// withFake swaps in a fake browser and captures the notices
func withFake(t *testing.T) (*fakeBrowser, func() []string) {
	t.Helper()
	Close()
	f := &fakeBrowser{}
	prev := launch
	launch = f.launch
	SetVisible(false)

	var nmu sync.Mutex
	var notices []string
	notice.SetHandler(func(_ notice.Level, msg string) {
		nmu.Lock()
		notices = append(notices, msg)
		nmu.Unlock()
	})
	resetHosts := func() {
		hostsMu.Lock()
		required, needed = map[string]bool{}, map[string]bool{}
		hostsMu.Unlock()
	}
	resetHosts()
	t.Cleanup(func() {
		Close()
		launch = prev
		notice.SetHandler(nil)
		SetAttentionHandler(nil)
		SetVisible(false)
		resetHosts()
	})
	return f, func() []string {
		nmu.Lock()
		defer nmu.Unlock()
		return append([]string(nil), notices...)
	}
}

func run(op pageOp) error {
	return runAt("https://www.example.com/series/1", op)
}

func runAt(url string, op pageOp) error {
	abort := lock()
	defer mu.Unlock()
	return runPage(abort, url, time.Second, true, op)
}

func ok(context.Context, hooks) error { return nil }

func challenge(context.Context, hooks) error {
	return &challengeError{url: "https://www.example.com/series/1", selector: "#list"}
}

func hasNotice(notices []string, sub string) bool {
	for _, n := range notices {
		if strings.Contains(n, sub) {
			return true
		}
	}
	return false
}

func TestBrowserIsReusedWhileItWorks(t *testing.T) {
	f, notices := withFake(t)

	for i := 0; i < 3; i++ {
		if err := run(ok); err != nil {
			t.Fatal(err)
		}
	}
	if launched, closed := f.counts(); launched != 1 || closed != 0 {
		t.Errorf("launched %d, closed %d; want one browser reused", launched, closed)
	}
	if !hasNotice(notices(), "example.com needs a real browser: starting it in the background") {
		t.Errorf("no notice that the browser is starting: %q", notices())
	}
}

func TestBrowserResetsAfterAFailure(t *testing.T) {
	f, notices := withFake(t)

	boom := errors.New("boom")
	if err := run(func(context.Context, hooks) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("got %v, want the op's error", err)
	}
	if _, closed := f.counts(); closed != 1 {
		t.Errorf("the failed browser wasn't closed")
	}
	if !hasNotice(notices(), "Closing the browser after the failure") {
		t.Errorf("no notice about closing the browser: %q", notices())
	}

	if err := run(ok); err != nil {
		t.Fatal(err)
	}
	if launched, _ := f.counts(); launched != 2 {
		t.Errorf("launched %d browsers, want a fresh one after the failure", launched)
	}
}

func TestBrowserReplacedWhenItDied(t *testing.T) {
	f, notices := withFake(t)

	if err := run(ok); err != nil {
		t.Fatal(err)
	}
	// the window was closed, or it crashed
	f.cancels[0]()

	if err := run(ok); err != nil {
		t.Fatalf("a dead browser was reused: %v", err)
	}
	if launched, _ := f.counts(); launched != 2 {
		t.Errorf("launched %d browsers, want a new one", launched)
	}
	if !hasNotice(notices(), "closed unexpectedly") {
		t.Errorf("no notice about the dead browser: %q", notices())
	}
}

func TestBrowserDyingMidLoadIsNotAChallenge(t *testing.T) {
	f, notices := withFake(t)

	err := run(func(ctx context.Context, _ hooks) error {
		f.cancels[0]()
		<-ctx.Done()
		return &challengeError{url: "https://www.example.com/series/1", selector: "#list"}
	})
	var gone *goneError
	if !errors.As(err, &gone) {
		t.Fatalf("got %v, want a goneError", err)
	}
	if launched, _ := f.counts(); launched != 1 {
		t.Errorf("launched %d browsers: a dead browser must not escalate to a visible one", launched)
	}
	if hasNotice(notices(), "visible browser window") {
		t.Errorf("escalated on a dead browser: %q", notices())
	}
}

func TestChallengeEscalatesToVisibleAndIsRemembered(t *testing.T) {
	f, notices := withFake(t)

	calls := 0
	err := run(func(ctx context.Context, h hooks) error {
		calls++
		if calls == 1 {
			return challenge(ctx, h)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.launched; len(got) != 2 || got[0] || !got[1] {
		t.Fatalf("launches (headed) = %v, want headless then visible", got)
	}
	if !hasNotice(notices(), "Opening a visible browser window") {
		t.Errorf("no notice about the visible window: %q", notices())
	}

	// the visible browser is kept for the next pages of the site...
	if err := run(ok); err != nil {
		t.Fatal(err)
	}
	if launched, _ := f.counts(); launched != 2 {
		t.Errorf("launched %d, want the visible browser reused", launched)
	}

	// ...and the site is remembered: after a failure closed it, the next
	// browser opens visible straight away
	if err := run(challenge); err == nil || !strings.Contains(err.Error(), "may not have been solved") {
		t.Fatalf("got %v, want a challenge error", err)
	}
	if err := run(ok); err != nil {
		t.Fatal(err)
	}
	if got := f.launched; len(got) != 3 || !got[2] {
		t.Errorf("launches (headed) = %v, want a visible one after the failure", got)
	}
}

func TestFailedVisibleStartDoesNotStick(t *testing.T) {
	f, _ := withFake(t)

	calls := 0
	err := run(func(ctx context.Context, h hooks) error {
		calls++
		f.failNext = errors.New("no display")
		return challenge(ctx, h)
	})
	if err == nil || !strings.Contains(err.Error(), "no display") {
		t.Fatalf("got %v, want the start error", err)
	}

	if err := run(ok); err != nil {
		t.Fatal(err)
	}
	if got := f.launched; len(got) != 2 || got[1] {
		t.Errorf("launches (headed) = %v, want headless again after the visible one failed to start", got)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	f, _ := withFake(t)

	Close() // nothing started
	if err := run(ok); err != nil {
		t.Fatal(err)
	}
	Close()
	Close()
	if launched, closed := f.counts(); launched != 1 || closed != 1 {
		t.Errorf("launched %d, closed %d; want one of each", launched, closed)
	}

	// and the browser starts again afterwards
	if err := run(ok); err != nil {
		t.Fatal(err)
	}
	if launched, _ := f.counts(); launched != 2 {
		t.Errorf("launched %d, want a new browser after Close", launched)
	}
}

func TestCloseAbortsARunningLoad(t *testing.T) {
	f, notices := withFake(t)

	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- run(func(ctx context.Context, _ hooks) error {
			close(started)
			<-ctx.Done()
			return &challengeError{url: "https://www.example.com/series/1", selector: "#list"}
		})
	}()
	<-started

	closed := make(chan struct{})
	go func() {
		Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Close waited for the running load")
	}
	if err := <-done; !errors.Is(err, ErrClosed) {
		t.Errorf("got %v, want ErrClosed", err)
	}
	if launched, closedN := f.counts(); launched != 1 || closedN != 1 {
		t.Errorf("launched %d, closed %d; want the browser closed without escalating", launched, closedN)
	}
	if hasNotice(notices(), "visible browser window") {
		t.Errorf("escalated after Close: %q", notices())
	}
}

func TestBrowserClosesWhenHoldIsReleased(t *testing.T) {
	f, _ := withFake(t)

	release := Hold()
	if err := run(ok); err != nil {
		t.Fatal(err)
	}
	if launched, closed := f.counts(); launched != 1 || closed != 0 {
		t.Errorf("launched %d, closed %d; want browser open while held", launched, closed)
	}

	release()
	if _, closed := f.counts(); closed != 1 {
		t.Errorf("closed %d after release; want browser closed immediately", closed)
	}

	// next run starts a fresh browser
	if err := run(ok); err != nil {
		t.Fatal(err)
	}
	if launched, _ := f.counts(); launched != 2 {
		t.Errorf("launched %d, want a new browser after the held one was released", launched)
	}
}

func TestSlowLoadReportsWaiting(t *testing.T) {
	_, notices := withFake(t)
	mu.Lock()
	prevAfter, prevEvery := waitNoticeAfter, waitNoticeEvery
	waitNoticeAfter, waitNoticeEvery = 20*time.Millisecond, time.Hour
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		waitNoticeAfter, waitNoticeEvery = prevAfter, prevEvery
		mu.Unlock()
	})

	if err := run(func(context.Context, hooks) error {
		time.Sleep(100 * time.Millisecond)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !hasNotice(notices(), "Waiting for example.com to load in the browser") {
		t.Errorf("no waiting notice: %q", notices())
	}
}

func TestCloseAbortsALoadQueuedBehindIt(t *testing.T) {
	f, _ := withFake(t)

	started := make(chan struct{})
	first := make(chan error, 1)
	go func() {
		first <- run(func(ctx context.Context, _ hooks) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		})
	}()
	<-started

	queued := make(chan struct{})
	lockHook = func() { close(queued) }
	t.Cleanup(func() { lockHook = nil })
	second := make(chan error, 1)
	go func() { second <- run(ok) }()
	// the second load has its abort context and is about to wait for mu
	<-queued

	Close()
	if err := <-first; !errors.Is(err, ErrClosed) {
		t.Errorf("running load: got %v, want ErrClosed", err)
	}
	if err := <-second; !errors.Is(err, ErrClosed) {
		t.Errorf("queued load: got %v, want ErrClosed", err)
	}
	if launched, closed := f.counts(); launched != 1 || closed != 1 {
		t.Errorf("launched %d, closed %d; a queued load started a browser after Close", launched, closed)
	}
}

func TestRenderedPageStopsTheWaitingNotice(t *testing.T) {
	_, notices := withFake(t)
	mu.Lock()
	prevAfter, prevEvery := waitNoticeAfter, waitNoticeEvery
	waitNoticeAfter, waitNoticeEvery = 20*time.Millisecond, 20*time.Millisecond
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		waitNoticeAfter, waitNoticeEvery = prevAfter, prevEvery
		mu.Unlock()
	})

	// rendered at once, then a long scroll or pagination
	if err := run(func(_ context.Context, h hooks) error {
		h.rendered()
		time.Sleep(100 * time.Millisecond)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if hasNotice(notices(), "Waiting for") {
		t.Errorf("waiting notice after the page rendered: %q", notices())
	}

	// never renders in time
	if err := run(func(context.Context, hooks) error {
		time.Sleep(100 * time.Millisecond)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !hasNotice(notices(), "Waiting for example.com") {
		t.Errorf("no waiting notice for a page that didn't render: %q", notices())
	}
}

func TestCloseAbortsASlowStart(t *testing.T) {
	f, _ := withFake(t)
	f.slow = make(chan struct{})

	done := make(chan error, 1)
	go func() { done <- run(ok) }()
	<-f.slow

	closed := make(chan struct{})
	go func() {
		Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Close waited for the browser to start")
	}
	if err := <-done; !errors.Is(err, ErrClosed) {
		t.Errorf("got %v, want ErrClosed", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.halfStarted != 1 || len(f.instances) != 0 {
		t.Errorf("half started %d, instances %d; want the start killed and none kept", f.halfStarted, len(f.instances))
	}
}

func TestReleaseHoldsAreIdempotent(t *testing.T) {
	f, _ := withFake(t)

	release := Hold()
	if err := run(ok); err != nil {
		t.Fatal(err)
	}
	if _, closed := f.counts(); closed != 0 {
		t.Fatal("browser was closed while still held")
	}

	// calling release multiple times is safe; the browser only closes once
	release()
	if _, closed := f.counts(); closed != 1 {
		t.Errorf("closed %d after first release; want 1", closed)
	}
	release() // harmless; already released

	if _, closed := f.counts(); closed != 1 {
		t.Errorf("closed %d after second release; want still 1", closed)
	}
}

func TestVisibleRetryRunsTheInitScriptAgain(t *testing.T) {
	f, _ := withFake(t)
	prev := renderPage
	t.Cleanup(func() { renderPage = prev })

	var inits []string
	renderPage = func(ctx context.Context, h hooks, url, waitSelector, init string, _ []chromedp.Action) (string, error) {
		inits = append(inits, init)
		if len(inits) == 1 {
			return "", challenge(ctx, h)
		}
		return "<html></html>", nil
	}

	s := Scroll{Init: "window.seeded = true"}
	if _, err := GetHTMLWithScroll("https://www.example.com/series/1", "#list", s, time.Second); err != nil {
		t.Fatal(err)
	}
	if got := f.launched; len(got) != 2 || got[0] || !got[1] {
		t.Fatalf("launches (headed) = %v, want headless then visible", got)
	}
	if len(inits) != 2 || inits[0] != s.Init || inits[1] != s.Init {
		t.Errorf("init scripts per render = %q, want it on both the headless and the visible one", inits)
	}
}

func TestRetryStatus(t *testing.T) {
	var waits []time.Duration
	old := waitFor
	waitFor = func(_ context.Context, d time.Duration) bool { waits = append(waits, d); return true }
	defer func() { waitFor = old }()

	closed := 0
	kept := &instance{close: func() { closed++ }}
	inst = kept
	defer func() { inst = nil }()

	calls := 0
	err := retryStatus(context.Background(), "x.test", func() error {
		calls++
		switch calls {
		case 1:
			return &StatusError{Code: 502, RetryAfter: 7 * time.Second}
		case 2:
			return &StatusError{Code: 429}
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Fatalf("retryStatus = %v after %d calls, want success after 3", err, calls)
	}
	if len(waits) != 2 || waits[0] != 7*time.Second || waits[1] != statusMinDelay {
		t.Errorf("waits = %v, want Retry-After then the minimum", waits)
	}
	// a restart would drop the session's cookies, a solved check too
	if closed != 0 || inst != kept {
		t.Errorf("closed the browser %d times, want it kept for an undeclared host", closed)
	}

	// a host that rate-limits per session gets a fresh browser for each retry
	RestartOnRateLimit("restart.test")
	defer func() { hostsMu.Lock(); delete(restart, "restart.test"); hostsMu.Unlock() }()
	waits, calls = nil, 0
	err = retryStatus(context.Background(), "cdn.restart.test", func() error {
		calls++
		inst = &instance{close: func() { closed++ }}
		if calls < 3 {
			return &StatusError{Code: 502}
		}
		return nil
	})
	if err != nil || calls != 3 || len(waits) != 2 {
		t.Fatalf("retryStatus = %v after %d calls and %d waits, want success after 3 and 2", err, calls, len(waits))
	}
	if closed != 2 {
		t.Errorf("closed the browser %d times, want before each of the 2 retries", closed)
	}
	inst = nil

	waits = nil
	retryStatus(context.Background(), "x.test", func() error { return &StatusError{Code: 502, RetryAfter: time.Minute} })
	if len(waits) != statusRetries || waits[0] != statusMaxDelay {
		t.Errorf("a long Retry-After waited %v, want %d waits capped at %s", waits, statusRetries, statusMaxDelay)
	}

	calls = 0
	err = retryStatus(context.Background(), "x.test", func() error { calls++; return &StatusError{Code: 503} })
	var se *StatusError
	if !errors.As(err, &se) || calls != statusRetries+1 {
		t.Errorf("persistent 503: %v after %d calls, want a StatusError after %d", err, calls, statusRetries+1)
	}

	calls = 0
	boom := errors.New("boom")
	if err := retryStatus(context.Background(), "x.test", func() error { calls++; return boom }); err != boom || calls != 1 {
		t.Errorf("other errors must not retry: %v after %d calls", err, calls)
	}

	waitFor = func(context.Context, time.Duration) bool { return false }
	if err := retryStatus(context.Background(), "x.test", func() error { return &StatusError{Code: 502} }); !errors.Is(err, ErrClosed) {
		t.Errorf("an aborted wait = %v, want ErrClosed", err)
	}
}
