// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
)

// fakeSink records what the App pushes and answers the folder picker with pick
type fakeSink struct {
	mu     sync.Mutex
	events []fakeEvent
	pick   string
	picked []string
	titles []string
}

type fakeEvent struct {
	name string
	data any
}

func (f *fakeSink) Emit(event string, data any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, fakeEvent{event, data})
}

func (f *fakeSink) Close() {}

func (f *fakeSink) ChooseDirectory(title, defaultDir string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.picked = append(f.picked, defaultDir)
	f.titles = append(f.titles, title)
	return f.pick, nil
}

// named returns the payloads of the events with the given name, in order
func (f *fakeSink) named(name string) []any {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []any
	for _, e := range f.events {
		if e.name == name {
			out = append(out, e.data)
		}
	}
	return out
}

// newTestApp returns an App over a fake site with n chapters, its options
// saved into a temp file. The App's pushes are not started; tests call pump
// through launch when they need them.
func newTestApp(t *testing.T, n int) (*App, *fakeSink, string) {
	t.Helper()
	c, _, _ := newTestController(t, false, nil, chapters(n, 2)...)
	sink := &fakeSink{}
	prefs := filepath.Join(t.TempDir(), "gui.json")
	a := newApp(c, sink, prefs, "", "v1.2.3")
	a.interval = time.Millisecond
	c.notify = a.notify
	return a, sink, prefs
}

// loadedApp returns newTestApp with its series loaded
func loadedApp(t *testing.T, n int) (*App, *fakeSink) {
	t.Helper()
	a, sink, _ := newTestApp(t, n)
	if err := a.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the load", func() bool { return !a.State().Loading })
	return a, sink
}

// waitUntil polls cond for up to 5 seconds
func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestStateReflectsTheController(t *testing.T) {
	a, _, _ := newTestApp(t, 5)
	if st := a.State(); st.Loaded || st.Running || st.Summary != nil || st.Version != "v1.2.3" {
		t.Fatalf("fresh state %+v", st)
	}

	if err := a.Load("  https://example.com/manga/test  "); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the load", func() bool { return !a.State().Loading })

	st := a.State()
	if !st.Loaded || st.Title != "Test Series" || st.ChapterCount != 5 || st.URL != "https://example.com/manga/test" {
		t.Errorf("loaded state %+v", st)
	}
	if st.LoadError != "" {
		t.Errorf("unexpected load error %q", st.LoadError)
	}
}

func TestLoadRefusesAnEmptyURL(t *testing.T) {
	a, _, _ := newTestApp(t, 1)
	if err := a.Load("   "); err == nil {
		t.Fatal("an empty URL was accepted")
	}
}

func TestLoadErrorReachesTheState(t *testing.T) {
	a, _, _ := newTestApp(t, 1)
	a.ctrl.identify = func(string, *grabber.Settings) (grabber.Site, []error) { return nil, nil }
	if err := a.Load("https://example.com/unknown"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the load", func() bool { return !a.State().Loading })
	if got := a.State().LoadError; got != "site not recognised" {
		t.Errorf("LoadError = %q", got)
	}
}

func TestLoadMessageNamesTheFailingStep(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{&core.TitleError{Err: errors.New("boom")}, "Couldn't fetch the series title: boom"},
		{&core.NoChaptersError{}, (&core.NoChaptersError{}).Error()},
		{errors.New("plain"), "plain"},
	}
	for _, c := range cases {
		if got, _ := loadMessage(c.err); got != c.want {
			t.Errorf("loadMessage(%T) = %q, want %q", c.err, got, c.want)
		}
	}
}

// a bundle's error is its summary's message, and a shared one keeps its key
func TestSummaryErrorCarriesItsMessage(t *testing.T) {
	s := summaryDTO(&Summary{Err: uiErr("errors.noChapterDownloaded", nil, nil)})
	if s.ErrorMsg == nil || s.ErrorMsg.Key != "errors.noChapterDownloaded" || s.Error != "no chapter could be downloaded" {
		t.Errorf("summary error = %q, message = %+v", s.Error, s.ErrorMsg)
	}
	if s := summaryDTO(&Summary{Err: errors.New("plain")}); s.ErrorMsg != nil {
		t.Errorf("plain error got message %+v", s.ErrorMsg)
	}
}

// a failure line's window wording travels as its message; a plain reason has none
func TestSummaryFailuresCarryTheirMessage(t *testing.T) {
	s := summaryDTO(&Summary{Failures: []Failure{
		windowFailure("Chapter 5", msg("job.reason.packing", map[string]any{"reason": "disk full"})),
		newFailure("Chapter 6", errors.New("boom")),
	}})
	packed, plain := s.Failures[0], s.Failures[1]
	if packed.Label != "Chapter 5" || packed.Reason != "packing: disk full" || packed.ReasonMsg == nil ||
		packed.ReasonMsg.Key != "job.reason.packing" {
		t.Errorf("packing failure = %+v", packed)
	}
	if plain.Label != "Chapter 6" || plain.Reason != "boom" || plain.ReasonMsg != nil {
		t.Errorf("plain failure = %+v", plain)
	}
	if empty := summaryDTO(&Summary{}); empty.Failures == nil {
		t.Errorf("no failures sent as null, want an empty list")
	}
}

// the load errors the window translates carry their key; the rest don't
func TestLoadMessageCarriesItsKey(t *testing.T) {
	cases := []struct {
		err error
		key string
	}{
		{&core.TitleError{Err: errors.New("boom")}, "errors.loadTitle"},
		{&core.ChaptersError{Errs: []error{errors.New("boom")}}, "errors.loadChapters"},
		{uiErr("errors.siteUnknown", nil, nil), "errors.siteUnknown"},
	}
	for _, c := range cases {
		if _, m := loadMessage(c.err); m == nil || m.Key != c.key {
			t.Errorf("loadMessage(%T) message = %+v, want key %q", c.err, m, c.key)
		}
	}
	if _, m := loadMessage(errors.New("plain")); m != nil {
		t.Errorf("a plain error got message %+v", m)
	}
}

func TestChaptersAndSelectionAreIndexed(t *testing.T) {
	a, _, _ := newTestApp(t, 4)
	if err := a.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the load", func() bool { return !a.State().Loading })

	chs := a.Chapters()
	if len(chs) != 4 || chs[2].Index != 2 || chs[2].Number != 3 || chs[2].Language != "" {
		t.Fatalf("chapters %+v", chs)
	}

	n, err := a.SelectRange("2-3")
	if err != nil || n != 2 {
		t.Fatalf("SelectRange = %d, %v", n, err)
	}
	if got := a.Selection(); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("Selection = %v", got)
	}
	if got := a.State().SelectedCount; got != 2 {
		t.Errorf("SelectedCount = %d", got)
	}
}

func TestSelectionIsEmptyNotNull(t *testing.T) {
	a, _, _ := newTestApp(t, 1)
	if got := a.Selection(); got == nil {
		t.Error("Selection returned nil, which the frontend reads as no array")
	}
	if got := a.Log(); got == nil {
		t.Error("Log returned nil")
	}
}

func TestSelectionIsPushedOnlyWhenItChanges(t *testing.T) {
	a, sink := loadedApp(t, 3)
	a.launch()
	defer a.shutdown(context.Background())

	a.ctrl.SetAll(true)
	// wait for the push that carries the change, not an earlier one from the load
	waitUntil(t, "a selection push", func() bool {
		sel := sink.named("selection")
		return len(sel) > 0 && len(sel[len(sel)-1].([]int)) == 3
	})
	pushes := len(sink.named("selection"))

	// a notify without a change must not resend the selection
	statesBefore := len(sink.named("state"))
	a.notify()
	waitUntil(t, "a state push", func() bool { return len(sink.named("state")) > statesBefore })
	if got := len(sink.named("selection")); got != pushes {
		t.Errorf("selection pushed %d more times for no change", got-pushes)
	}
}

func TestStatePushesAreThrottled(t *testing.T) {
	a, sink := loadedApp(t, 3)
	a.interval = 40 * time.Millisecond
	a.launch()
	defer a.shutdown(context.Background())

	// a burst of changes lands within one interval
	for i := 0; i < 500; i++ {
		a.ctrl.SetSelected(i%3, i%2 == 0)
		a.notify()
	}
	a.ctrl.SetAll(true)
	a.notify()

	// the trailing change is always sent, and the burst collapsed
	waitUntil(t, "the final state", func() bool {
		states := sink.named("state")
		return len(states) > 0 && states[len(states)-1].(State).SelectedCount == 3
	})
	if got := len(sink.named("state")); got > 20 {
		t.Errorf("%d state pushes for a burst inside one throttle window", got)
	}
}

func TestShutdownStopsPushes(t *testing.T) {
	a, sink, _ := newTestApp(t, 1)
	a.launch()
	a.shutdown(context.Background())
	// a push that was already underway may still land; let it finish first
	time.Sleep(20 * time.Millisecond)
	before := len(sink.named("state"))
	a.notify()
	time.Sleep(20 * time.Millisecond)
	if after := len(sink.named("state")); after != before {
		t.Errorf("pushed %d state events after shutdown", after-before)
	}
}

func TestSetOptionsAppliesAndSaves(t *testing.T) {
	a, _, prefs := newTestApp(t, 1)
	o := a.Options()
	o.Format = "raw"
	o.ConvertImages = "webp"
	if err := a.SetOptions(o); err != nil {
		t.Fatal(err)
	}
	if got := a.Options(); got.Format != "raw" || got.ConvertImages != "webp" {
		t.Errorf("options %+v", got)
	}

	saved := defaultSettings()
	if err := loadOptions(prefs, saved); err != nil {
		t.Fatal(err)
	}
	if saved.Format != "raw" || saved.ConvertImages != "webp" {
		t.Errorf("saved options %+v", optionsOf(*saved))
	}
}

// the prefs and the options share gui.json, so each setter must keep the other half
func TestPrefSettersKeepEveryOption(t *testing.T) {
	a, _, prefs := newTestApp(t, 1)
	o := a.Options()
	o.Format = "raw"
	o.Language = "es"
	o.ReadingDirection = "ltr"
	if err := a.SetOptions(o); err != nil {
		t.Fatal(err)
	}
	if err := a.SetUILanguage("ca"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetCheckUpdates(false); err != nil {
		t.Fatal(err)
	}
	if got := a.Options(); got != o {
		t.Errorf("options %+v after the pref setters, want %+v", got, o)
	}
	if got := loadPrefs(prefs); got != (PrefsDTO{UILanguage: "ca", SimpleStart: true}) {
		t.Errorf("saved prefs %+v after the pref setters", got)
	}

	// and SetOptions keeps the prefs
	o.Format = "epub"
	if err := a.SetOptions(o); err != nil {
		t.Fatal(err)
	}
	if got := loadPrefs(prefs); got != (PrefsDTO{UILanguage: "ca", SimpleStart: true}) {
		t.Errorf("saved prefs %+v after SetOptions", got)
	}
	saved := defaultSettings()
	if err := loadOptions(prefs, saved); err != nil || optionsOf(*saved) != o {
		t.Errorf("saved options %+v (%v) after SetOptions, want %+v", optionsOf(*saved), err, o)
	}
}

func TestPrefSettersWorkWhileBusy(t *testing.T) {
	a, _, prefs := newTestApp(t, 1)
	if err := a.ctrl.BeginLoad("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	before := a.ctrl.Settings()
	if err := a.SetUILanguage("es"); err != nil {
		t.Errorf("SetUILanguage while loading = %v", err)
	}
	if err := a.SetCheckUpdates(false); err != nil {
		t.Errorf("SetCheckUpdates while loading = %v", err)
	}

	if after := a.ctrl.Settings(); !reflect.DeepEqual(before, after) {
		t.Errorf("the pref setters changed the settings:\n%+v\n%+v", before, after)
	}
	if st := a.State(); st.Prefs != (PrefsDTO{UILanguage: "es", SimpleStart: true}) {
		t.Errorf("State().Prefs = %+v", st.Prefs)
	}
	if got := loadPrefs(prefs); got != (PrefsDTO{UILanguage: "es", SimpleStart: true}) {
		t.Errorf("saved prefs %+v", got)
	}
}

func TestSetSimpleStartIsSavedAndKeepsTheOptions(t *testing.T) {
	a, _, prefs := newTestApp(t, 1)
	if err := a.ctrl.BeginLoad("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	o := a.Options()
	if err := a.SetSimpleStart(false); err != nil {
		t.Errorf("SetSimpleStart while loading = %v", err)
	}
	if st := a.State(); st.Prefs != (PrefsDTO{CheckUpdates: true}) {
		t.Errorf("State().Prefs = %+v", st.Prefs)
	}
	if got := loadPrefs(prefs); got.SimpleStart {
		t.Errorf("saved prefs %+v", got)
	}
	if got := a.Options(); got != o {
		t.Errorf("options %+v after SetSimpleStart, want %+v", got, o)
	}
}

func TestTurningUpdatesOffClearsTheUpdate(t *testing.T) {
	a, _, _ := newTestApp(t, 1)
	a.ctrl.update = &Update{Latest: "v9.9.9"}
	if err := a.SetCheckUpdates(false); err != nil {
		t.Fatal(err)
	}
	if st := a.State(); st.Update != nil || st.Prefs.CheckUpdates {
		t.Errorf("after turning updates off: update %+v, prefs %+v", st.Update, st.Prefs)
	}
}

// a check still running when updates are turned off must not record its answer,
// and turning them on again must check afresh
func TestUpdateCheckFinishingAfterTurnOffIsDropped(t *testing.T) {
	a, _, _ := newTestApp(t, 1)
	gate := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-gate
		_, _ = w.Write([]byte(`{"tag_name":"v1.13.0"}`))
	}))
	defer srv.Close()
	a.tag, a.updateSource = "v1.12.0", testSource(srv)

	if !a.ctrl.claimUpdateCheck() {
		t.Fatal("no check was running yet")
	}
	done := make(chan struct{})
	go func() {
		a.ctrl.CheckUpdate(context.Background(), a.tag, a.updateSource)
		close(done)
	}()
	if err := a.SetCheckUpdates(false); err != nil {
		t.Fatal(err)
	}
	close(gate)
	<-done
	if u := a.ctrl.Update(); u != nil {
		t.Fatalf("the answer of a check that ended after turning off was recorded: %+v", u)
	}

	if err := a.SetCheckUpdates(true); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the update check after turning updates on", func() bool { return a.State().Update != nil })
	if got := a.State().Update.Latest; got != "v1.13.0" {
		t.Errorf("update after turning updates on = %q", got)
	}
}

// the first locale report starts the startup check, sending that locale, and a
// later report starts nothing
func TestStartupCheckStartsOnTheFirstLocaleReport(t *testing.T) {
	a, _, _ := newTestApp(t, 1)
	var mu sync.Mutex
	var syslangs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		syslangs = append(syslangs, r.URL.Query().Get("syslang"))
		mu.Unlock()
		_, _ = w.Write([]byte(`{"tag_name":"v1.13.0"}`))
	}))
	defer srv.Close()
	a.tag, a.updateSource = "v1.12.0", testSource(srv)

	a.SetSystemLocale("fr-FR")
	waitUntil(t, "the startup update check", func() bool { return a.State().Update != nil })
	a.SetSystemLocale("de-DE")
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(syslangs) != 1 || syslangs[0] != "fr-FR" {
		t.Errorf("update requests sent syslang %q, want one request with fr-FR", syslangs)
	}
}

func TestSetUILanguageRefusesAnUnknownLanguage(t *testing.T) {
	a, _, _ := newTestApp(t, 1)
	if err := a.SetUILanguage("fr"); err == nil {
		t.Error("an unknown language was accepted")
	}
	if got := a.State().Prefs; got.UILanguage != "" {
		t.Errorf("prefs %+v after a refused language", got)
	}
}

func TestSetOptionsRefusedWhileBusy(t *testing.T) {
	a, _, _ := newTestApp(t, 1)
	if err := a.ctrl.BeginLoad("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetOptions(a.Options()); !errors.Is(err, errBusy) {
		t.Errorf("SetOptions while loading = %v, want errBusy", err)
	}
}

func TestChooseFolderSetsTheOutputFolder(t *testing.T) {
	a, sink, prefs := newTestApp(t, 1)
	sink.pick = t.TempDir()
	got, err := a.ChooseFolder("Choose the output folder")
	if err != nil || got != sink.pick {
		t.Fatalf("ChooseFolder = %q, %v", got, err)
	}
	if a.Options().OutputDir != sink.pick {
		t.Errorf("output folder %q, want %q", a.Options().OutputDir, sink.pick)
	}
	if len(sink.titles) != 1 || sink.titles[0] != "Choose the output folder" {
		t.Errorf("dialog titles = %q, want the one passed", sink.titles)
	}
	if loaded := defaultSettings(); loadOptions(prefs, loaded) != nil || loaded.OutputDir != sink.pick {
		t.Errorf("the picked folder wasn't saved")
	}
}

func TestChooseFolderCancelledKeepsTheFolder(t *testing.T) {
	a, sink, _ := newTestApp(t, 1)
	before := a.Options().OutputDir
	sink.pick = ""
	got, err := a.ChooseFolder("Choose the output folder")
	if err != nil || got != "" {
		t.Fatalf("cancelled ChooseFolder = %q, %v", got, err)
	}
	if a.Options().OutputDir != before {
		t.Error("a cancelled picker changed the output folder")
	}
}

func TestChooseFolderStartsAtAFolderThatExists(t *testing.T) {
	if got := existingDir(filepath.Join(t.TempDir(), "gone")); got == "" {
		t.Error("existingDir gave no fallback for a missing folder")
	}
	dir := t.TempDir()
	if got := existingDir(dir); got != dir {
		t.Errorf("existingDir(%q) = %q", dir, got)
	}
}

func TestStartRefusalsComeBackAsErrors(t *testing.T) {
	a, _, _ := newTestApp(t, 2)
	if err := a.Start(); err == nil {
		t.Error("Start without a series succeeded")
	}
	if err := a.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the load", func() bool { return !a.State().Loading })
	if err := a.Start(); err == nil {
		t.Error("Start with nothing selected succeeded")
	}
}

func TestSummaryAndJobDTOs(t *testing.T) {
	if summaryDTO(nil) != nil {
		t.Error("summaryDTO(nil) should be nil so the frontend sees null")
	}
	s := summaryDTO(&Summary{Dir: "/out", Saved: 1, Err: errors.New("disk full")})
	if s.Error != "disk full" || s.Failures == nil {
		t.Errorf("summary %+v", s)
	}
	if jobStateName(JobArchiving) != "archiving" || jobStateName(JobSkipped) != "skipped" {
		t.Error("job state names drifted from the frontend's")
	}
	if logLevelName(LogWarn) != "warn" || logLevelName(LogError) != "error" || logLevelName(LogInfo) != "info" {
		t.Error("log level names drifted from the frontend's")
	}
}

func TestPanickingParserEndsTheLoad(t *testing.T) {
	a, sink, _ := newTestApp(t, 1)
	a.ctrl.identify = func(string, *grabber.Settings) (grabber.Site, []error) { panic("nil deref") }
	if err := a.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the load to end", func() bool { return !a.State().Loading })
	if got := a.State().LoadError; got == "" {
		t.Error("a panicking load left no error behind")
	}
	// the chapter list is still announced, so the window isn't waiting on it
	waitUntil(t, "the chapters event", func() bool { return len(sink.named("chapters")) == 1 })
}

func TestStateCarriesTheLoadedFilters(t *testing.T) {
	a, _, _ := newTestApp(t, 2)
	if err := a.ctrl.Configure(func(s *grabber.Settings) { s.Language = "es"; s.Scanlator = "grp" }); err != nil {
		t.Fatal(err)
	}
	if err := a.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the load", func() bool { return !a.State().Loading })
	if st := a.State(); st.LoadedLanguage != "es" || st.LoadedScanlator != "grp" || st.Stale {
		t.Fatalf("after load: loaded %q/%q, stale %v", st.LoadedLanguage, st.LoadedScanlator, st.Stale)
	}

	if err := a.ctrl.Configure(func(s *grabber.Settings) { s.Language = "fr" }); err != nil {
		t.Fatal(err)
	}
	st := a.State()
	if st.LoadedLanguage != "es" || st.LoadedScanlator != "grp" || !st.Stale {
		t.Errorf("after a language change: loaded %q/%q, stale %v", st.LoadedLanguage, st.LoadedScanlator, st.Stale)
	}
}

func TestSetSelectedManyThroughTheApp(t *testing.T) {
	a, _ := loadedApp(t, 4)
	if err := a.SetSelectedMany([]int{1, 3}, true); err != nil {
		t.Fatal(err)
	}
	if got := a.Selection(); len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Errorf("Selection = %v", got)
	}
	if err := a.SetSelectedMany([]int{7}, true); err == nil {
		t.Error("App accepted an index out of range")
	}
}
