// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package browser

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

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
}

func (f *fakeBrowser) launch(headed bool) (*instance, error) {
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
	i := &instance{ctx: ctx, lost: ctx, headed: headed}
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
	t.Cleanup(func() {
		Close()
		launch = prev
		notice.SetHandler(nil)
		SetVisible(false)
		SetIdleTimeout(0)
	})
	return f, func() []string {
		nmu.Lock()
		defer nmu.Unlock()
		return append([]string(nil), notices...)
	}
}

func run(op pageOp) error {
	abort := lock()
	defer mu.Unlock()
	return runPage(abort, "https://www.example.com/series/1", "#list", time.Second, true, op)
}

func ok(context.Context, func(time.Duration)) error { return nil }

func challenge(context.Context, func(time.Duration)) error {
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
	if err := run(func(context.Context, func(time.Duration)) error { return boom }); !errors.Is(err, boom) {
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

	err := run(func(ctx context.Context, _ func(time.Duration)) error {
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

func TestChallengeEscalatesToVisibleThenResets(t *testing.T) {
	f, notices := withFake(t)

	calls := 0
	err := run(func(ctx context.Context, r func(time.Duration)) error {
		calls++
		if calls == 1 {
			return challenge(ctx, r)
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

	// ...but a failure there goes back to the user's choice, headless
	if err := run(challenge); err == nil || !strings.Contains(err.Error(), "may not have been solved") {
		t.Fatalf("got %v, want a challenge error", err)
	}
	if err := run(ok); err != nil {
		t.Fatal(err)
	}
	if got := f.launched; len(got) != 3 || got[2] {
		t.Errorf("launches (headed) = %v, want a headless one after the failure", got)
	}
}

func TestFailedVisibleStartDoesNotStick(t *testing.T) {
	f, _ := withFake(t)

	calls := 0
	err := run(func(ctx context.Context, r func(time.Duration)) error {
		calls++
		f.failNext = errors.New("no display")
		return challenge(ctx, r)
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
		done <- run(func(ctx context.Context, _ func(time.Duration)) error {
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

func TestIdleTimeoutClosesTheBrowser(t *testing.T) {
	f, _ := withFake(t)
	SetIdleTimeout(50 * time.Millisecond)

	if err := run(ok); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, closed := f.counts(); closed == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the idle browser was never closed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := run(ok); err != nil {
		t.Fatal(err)
	}
	if launched, _ := f.counts(); launched != 2 {
		t.Errorf("launched %d, want a new browser after the idle close", launched)
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

	if err := run(func(context.Context, func(time.Duration)) error {
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
		first <- run(func(ctx context.Context, _ func(time.Duration)) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		})
	}()
	<-started

	second := make(chan error, 1)
	go func() { second <- run(ok) }()
	// let the second load reach the lock and wait there
	time.Sleep(50 * time.Millisecond)

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
