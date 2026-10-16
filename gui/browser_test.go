// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"strings"
	"testing"

	"github.com/elboletaire/manga-downloader/browser"
	"github.com/elboletaire/manga-downloader/packer"
)

func TestBrowserVisibleAutoFollowsTheLoadedSite(t *testing.T) {
	a, _, _ := newTestApp(t, 3)
	c := a.ctrl
	c.needsVisible = func(url string) bool { return strings.Contains(url, "example.com") }

	if a.State().BrowserVisibleAuto {
		t.Fatal("auto with nothing loaded")
	}
	if err := a.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the load", func() bool { return !a.State().Loading })

	st := a.State()
	if !st.BrowserVisibleAuto {
		t.Error("not auto for a site that needs the window")
	}
	// the saved preference is the user's, untouched
	if st.Options.BrowserVisible || c.Settings().BrowserVisible {
		t.Error("the auto mode overwrote the option")
	}

	if err := a.Clear(); err != nil {
		t.Fatal(err)
	}
	if a.State().BrowserVisibleAuto {
		t.Error("still auto after clearing the series")
	}
}

func TestBrowserVisibleAutoForOtherSites(t *testing.T) {
	a, _ := loadedApp(t, 2)
	a.ctrl.needsVisible = func(string) bool { return false }
	if a.State().BrowserVisibleAuto {
		t.Error("auto for a site that doesn't need the window")
	}
}

func TestBrowserAlertLifecycle(t *testing.T) {
	a, sink, _ := newTestApp(t, 2)
	c := a.ctrl
	fronts := 0
	c.bringToFront = func() { fronts++ }
	a.launch()
	t.Cleanup(func() { a.shutdown(nil) })

	if a.State().BrowserAlert != "" {
		t.Fatal("an alert before any browser")
	}

	att := browser.Attention{Active: true, Host: "example.com", Browser: "Brave"}
	c.onAttention(att)
	if got := a.State().BrowserAlert; got != att.Message() {
		t.Fatalf("alert = %q, want %q", got, att.Message())
	}
	log := c.Log()
	if last := log[len(log)-1]; last.Level != LogWarn || last.Text != att.Message() {
		t.Errorf("last log line = %+v, want the message as a warning", last)
	}
	waitUntil(t, "the alert pushed", func() bool {
		for _, s := range sink.named("state") {
			if s.(State).BrowserAlert != "" {
				return true
			}
		}
		return false
	})

	a.ShowBrowser()
	if fronts != 1 {
		t.Errorf("brought to the front %d times, want once", fronts)
	}

	// the page rendered
	c.onAttention(browser.Attention{Host: "example.com", Browser: "Brave"})
	if got := a.State().BrowserAlert; got != "" {
		t.Errorf("alert = %q after the page rendered, want none", got)
	}

	// dismissed by the user, while the load goes on
	c.onAttention(att)
	a.DismissBrowserAlert()
	if got := a.State().BrowserAlert; got != "" {
		t.Errorf("alert = %q after dismissing it, want none", got)
	}
	// the end that follows is harmless
	c.onAttention(browser.Attention{Host: "example.com"})
	if got := a.State().BrowserAlert; got != "" {
		t.Errorf("alert = %q, want none", got)
	}
}

func TestClearDropsTheBrowserAlert(t *testing.T) {
	a, _ := loadedApp(t, 2)
	a.ctrl.onAttention(browser.Attention{Active: true, Host: "example.com"})
	if err := a.Clear(); err != nil {
		t.Fatal(err)
	}
	if got := a.State().BrowserAlert; got != "" {
		t.Errorf("alert = %q after clearing, want none", got)
	}
}

func TestStateCarriesTheFilenameTemplateDefaultAndWarning(t *testing.T) {
	a, _, _ := newTestApp(t, 3)
	st := a.State()
	if st.FilenameTemplateDefault != packer.FilenameTemplateDefault || st.FilenameTemplateWarning != "" {
		t.Errorf("default state: %q, %q", st.FilenameTemplateDefault, st.FilenameTemplateWarning)
	}
	o := st.Options
	o.FilenameTemplate = "{{.Series}}"
	if err := a.SetOptions(o); err != nil {
		t.Fatal(err)
	}
	if a.State().FilenameTemplateWarning == "" {
		t.Error("no warning for a template without a number or title")
	}
}

func TestFilenameTemplateWarningOnlyForChapters(t *testing.T) {
	a, _, _ := newTestApp(t, 3)
	o := a.State().Options
	o.FilenameTemplate, o.PackAs = "{{.Series}}", PackBundle
	if err := a.SetOptions(o); err != nil {
		t.Fatal(err)
	}
	if w := a.State().FilenameTemplateWarning; w != "" {
		t.Errorf("warned in bundle mode: %q", w)
	}
}
