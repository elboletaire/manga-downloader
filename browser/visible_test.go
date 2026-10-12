// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package browser

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// attentions records what the attention handler is told
func attentions(t *testing.T) func() []Attention {
	t.Helper()
	var amu sync.Mutex
	var got []Attention
	SetAttentionHandler(func(a Attention) {
		amu.Lock()
		got = append(got, a)
		amu.Unlock()
	})
	return func() []Attention {
		amu.Lock()
		defer amu.Unlock()
		return append([]Attention(nil), got...)
	}
}

// actives lists the Active field of each attention, in order
func actives(as []Attention) []bool {
	out := make([]bool, len(as))
	for i, a := range as {
		out[i] = a.Active
	}
	return out
}

func equalBools(a, b []bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (f *fakeBrowser) frontCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fronts
}

func TestDeclaredHostStartsVisible(t *testing.T) {
	f, notices := withFake(t)
	got := attentions(t)
	RequireVisible("example.com")

	if err := run(ok); err != nil {
		t.Fatal(err)
	}
	if l := f.launched; len(l) != 1 || !l[0] {
		t.Fatalf("launches (headed) = %v, want one visible and no headless attempt", l)
	}
	if hasNotice(notices(), "headless") {
		t.Errorf("a headless attempt was made: %q", notices())
	}
	as := got()
	if !equalBools(actives(as), []bool{true, false}) {
		t.Fatalf("attentions = %+v, want raised then ended", as)
	}
	if as[0].Host != "example.com" || as[0].Browser != "Fakefox" {
		t.Errorf("attention = %+v, want the host and the browser's name", as[0])
	}
	// brought forward in a goroutine: give it a moment
	deadline := time.Now().Add(time.Second)
	for f.frontCount() != 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if n := f.frontCount(); n != 1 {
		t.Errorf("window brought to the front %d times, want once", n)
	}
}

func TestNeedsVisibleMatchesSubdomains(t *testing.T) {
	withFake(t)
	RequireVisible("www.Example.com")

	for url, want := range map[string]bool{
		"https://example.com/a":     true,
		"https://www.example.com/a": true,
		"https://api.example.com/":  true,
		"example.com":               true,
		"https://notexample.com/":   false,
		"https://example.org/":      false,
		"":                          false,
	} {
		if got := NeedsVisible(url); got != want {
			t.Errorf("NeedsVisible(%q) = %v, want %v", url, got, want)
		}
	}
}

func TestEscalatedHostStartsVisibleAfterClose(t *testing.T) {
	f, _ := withFake(t)
	got := attentions(t)

	calls := 0
	if err := run(func(ctx context.Context, h hooks) error {
		calls++
		if calls == 1 {
			return challenge(ctx, h)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !NeedsVisible("https://example.com/") {
		t.Fatal("the escalated host wasn't remembered")
	}
	if !equalBools(actives(got()), []bool{true, false}) {
		t.Errorf("attentions = %+v, want the escalation raised then ended", got())
	}

	// the GUI closes the browser after every download
	Close()
	if err := run(ok); err != nil {
		t.Fatal(err)
	}
	if l := f.launched; len(l) != 3 || l[0] || !l[1] || !l[2] {
		t.Fatalf("launches (headed) = %v, want headless, visible, then visible straight away", l)
	}
	if !equalBools(actives(got()), []bool{true, false, true, false}) {
		t.Errorf("attentions = %+v, want the new window to raise one too", got())
	}

	// other sites still go headless
	if err := runAt("https://other.org/x", ok); err != nil {
		t.Fatal(err)
	}
	if l := f.launched; len(l) != 3 {
		t.Errorf("launches (headed) = %v, want the open window reused", l)
	}
	Close()
	if err := runAt("https://other.org/x", ok); err != nil {
		t.Fatal(err)
	}
	if l := f.launched; len(l) != 4 || l[3] {
		t.Errorf("launches (headed) = %v, want another site to start headless", l)
	}
}

func TestHeadlessBrowserIsReplacedForADeclaredHost(t *testing.T) {
	f, _ := withFake(t)
	RequireVisible("example.com")

	if err := runAt("https://other.org/x", ok); err != nil {
		t.Fatal(err)
	}
	if err := run(ok); err != nil {
		t.Fatal(err)
	}
	if l, closed := f.launched, f.closed; len(l) != 2 || l[0] || !l[1] || closed != 1 {
		t.Errorf("launches (headed) = %v, closed %d; want the headless one swapped for a window", l, closed)
	}
}

func TestAttentionEndsWhenThePageRenders(t *testing.T) {
	_, _ = withFake(t)
	got := attentions(t)
	RequireVisible("example.com")

	if err := run(func(_ context.Context, h hooks) error {
		h.rendered()
		// scrolling, pagination: the user has nothing to do any more
		if as := got(); !equalBools(actives(as), []bool{true, false}) {
			t.Errorf("attentions after rendering = %+v, want it ended", as)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if as := got(); len(as) != 2 {
		t.Errorf("attentions = %+v, want one raise and one end", as)
	}
}

func TestAttentionEndsWhenTheLoadFails(t *testing.T) {
	_, _ = withFake(t)
	got := attentions(t)
	RequireVisible("example.com")

	boom := errors.New("boom")
	if err := run(func(context.Context, hooks) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("got %v, want the op's error", err)
	}
	if as := got(); !equalBools(actives(as), []bool{true, false}) {
		t.Errorf("attentions = %+v, want raised then ended", as)
	}
}

func TestAttentionEndsWhenClosed(t *testing.T) {
	_, _ = withFake(t)
	got := attentions(t)
	RequireVisible("example.com")

	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- run(func(ctx context.Context, _ hooks) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		})
	}()
	<-started
	Close()
	if err := <-done; !errors.Is(err, ErrClosed) {
		t.Fatalf("got %v, want ErrClosed", err)
	}
	if as := got(); !equalBools(actives(as), []bool{true, false}) {
		t.Errorf("attentions = %+v, want raised then ended", as)
	}
}

func TestReusedWindowAlertsOnlyWhenSlow(t *testing.T) {
	_, _ = withFake(t)
	got := attentions(t)
	RequireVisible("example.com")
	mu.Lock()
	prevAfter, prevEvery := waitNoticeAfter, waitNoticeEvery
	waitNoticeAfter, waitNoticeEvery = 20*time.Millisecond, time.Hour
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		waitNoticeAfter, waitNoticeEvery = prevAfter, prevEvery
		mu.Unlock()
	})

	if err := run(ok); err != nil {
		t.Fatal(err)
	}
	// a quick page in the same window: nothing for the user to do
	if err := run(ok); err != nil {
		t.Fatal(err)
	}
	if as := got(); len(as) != 2 {
		t.Fatalf("attentions = %+v, want only the first window's", as)
	}

	// a page that doesn't render may be showing a check
	if err := run(func(context.Context, hooks) error {
		time.Sleep(100 * time.Millisecond)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if as := got(); !equalBools(actives(as), []bool{true, false, true, false}) {
		t.Errorf("attentions = %+v, want the slow page to raise one", as)
	}
}

func TestUserChosenWindowRaisesNoAttention(t *testing.T) {
	f, _ := withFake(t)
	got := attentions(t)
	SetVisible(true)

	if err := run(ok); err != nil {
		t.Fatal(err)
	}
	if l := f.launched; len(l) != 1 || !l[0] {
		t.Fatalf("launches (headed) = %v, want one visible", l)
	}
	if as := got(); len(as) != 0 {
		t.Errorf("attentions = %+v, want none for a window the user asked for", as)
	}
}

func TestBringToFront(t *testing.T) {
	f, _ := withFake(t)

	BringToFront() // nothing open: harmless
	if err := run(ok); err != nil {
		t.Fatal(err)
	}
	BringToFront() // headless: nothing to raise
	SetVisible(true)
	Close()
	if err := run(ok); err != nil {
		t.Fatal(err)
	}
	BringToFront()
	if n := f.frontCount(); n != 1 {
		t.Errorf("brought to the front %d times, want once", n)
	}
}

func TestAttentionMessage(t *testing.T) {
	named := Attention{Active: true, Host: "example.com", Browser: "Brave"}.Message()
	if want := "Brave opened a window to pass a check on example.com. There may be a captcha or puzzle to solve there; the download continues by itself once it's done."; named != want {
		t.Errorf("got %q, want %q", named, want)
	}
	unnamed := Attention{Active: true, Host: "example.com"}.Message()
	if want := "A browser window opened to pass a check on example.com."; unnamed[:len(want)] != want {
		t.Errorf("got %q, want it to start with %q", unnamed, want)
	}
}

func TestBrowserName(t *testing.T) {
	for path, want := range map[string]string{
		`C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`: "Brave",
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`:       "Microsoft Edge",
		`C:\Program Files\Google\Chrome\Application\chrome.exe`:              "Google Chrome",
		`C:\Users\me\AppData\Local\Chromium\Application\chrome.exe`:          "Chromium",
		"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser":       "Brave",
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome":       "Google Chrome",
		"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge":     "Microsoft Edge",
		"/usr/bin/chromium-browser":                                          "Chromium",
		"/usr/bin/google-chrome-stable":                                      "Google Chrome",
		"/usr/bin/brave-browser":                                             "Brave",
		"/usr/bin/headless_shell":                                            "",
		"/opt/something/else":                                                "",
	} {
		if got := browserName(path); got != want {
			t.Errorf("browserName(%q) = %q, want %q", path, got, want)
		}
	}
}
