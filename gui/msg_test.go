// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/elboletaire/manga-downloader/core"
)

// withCatalog swaps the English catalog for the test and restores it after
func withCatalog(t *testing.T, text map[string]string) {
	t.Helper()
	saved := catalog
	catalog = text
	t.Cleanup(func() { catalog = saved })
}

func TestMsgFillsEnglishText(t *testing.T) {
	m := msg("errors.enterURL", nil)
	if m.Key != "errors.enterURL" {
		t.Errorf("Key = %q", m.Key)
	}
	if want := "enter the URL of a series first"; m.Text != want {
		t.Errorf("Text = %q, want %q", m.Text, want)
	}
}

func TestMsgInterpolates(t *testing.T) {
	withCatalog(t, map[string]string{"x.hello": "hello {{name}}, {{name}}!"})

	m := msg("x.hello", map[string]any{"name": "Ana"})
	if want := "hello Ana, Ana!"; m.Text != want {
		t.Errorf("Text = %q, want %q", m.Text, want)
	}
	if m.Args["name"] != "Ana" {
		t.Errorf("Args = %v, want the args kept", m.Args)
	}
}

func TestMsgInterpolatesOnce(t *testing.T) {
	withCatalog(t, map[string]string{"x.tricky": "{{a}} and {{b}} {{missing}}"})

	m := msg("x.tricky", map[string]any{"a": "{{b}}", "b": "B"})
	if want := "{{b}} and B {{missing}}"; m.Text != want {
		t.Errorf("Text = %q, want %q", m.Text, want)
	}
}

func TestMsgPlurals(t *testing.T) {
	withCatalog(t, map[string]string{
		"x.files_one":   "{{count}} file",
		"x.files_other": "{{count}} files",
		"x.plain":       "plain",
	})

	cases := []struct {
		key  string
		args map[string]any
		want string
	}{
		{"x.files", map[string]any{"count": 1}, "1 file"},
		{"x.files", map[string]any{"count": 0}, "0 files"},
		{"x.files", map[string]any{"count": 3}, "3 files"},
		{"x.files", map[string]any{"count": int64(1)}, "1 file"},
		{"x.plain", map[string]any{"count": 5}, "plain"},
	}
	for _, c := range cases {
		if got := msg(c.key, c.args).Text; got != c.want {
			t.Errorf("msg(%q, %v).Text = %q, want %q", c.key, c.args, got, c.want)
		}
	}
}

func TestMsgUnknownKeyIsText(t *testing.T) {
	m := msg("nope.missing", nil)
	if m.Text != "nope.missing" {
		t.Errorf("Text = %q, want the key", m.Text)
	}
}

func TestUIErr(t *testing.T) {
	base := errors.New("boom")
	err := uiErr("errors.enterURL", nil, base)

	if want := "enter the URL of a series first"; err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
	if !errors.Is(err, base) {
		t.Error("errors.Is does not reach the wrapped error")
	}
	if errors.Unwrap(err) != base {
		t.Errorf("Unwrap() = %v, want %v", errors.Unwrap(err), base)
	}
	var ue *uiError
	if !errors.As(err, &ue) || ue.Key != "errors.enterURL" {
		t.Errorf("errors.As gave %v", ue)
	}
}

func TestWindowError(t *testing.T) {
	t.Run("uiError", func(t *testing.T) {
		want := msg("errors.enterURL", map[string]any{"n": 1})
		got := windowError(uiErr("errors.enterURL", map[string]any{"n": 1}, errors.New("x")))
		var back Message
		if err := json.Unmarshal([]byte(got), &back); err != nil {
			t.Fatalf("windowError = %q, not JSON: %v", got, err)
		}
		if !reflect.DeepEqual(back, Message{Key: want.Key, Args: map[string]any{"n": float64(1)}, Text: want.Text}) {
			t.Errorf("windowError round-trips to %#v, want %#v", back, want)
		}
	})
	t.Run("wrapped uiError", func(t *testing.T) {
		wrapped := errors.Join(errors.New("other"), uiErr("errors.enterURL", nil, nil))
		var back Message
		if err := json.Unmarshal([]byte(windowError(wrapped)), &back); err != nil || back.Key != "errors.enterURL" {
			t.Errorf("windowError(%v) = %q, want the message", wrapped, windowError(wrapped))
		}
	})
	t.Run("plain error", func(t *testing.T) {
		if got := windowError(errors.New("plain")); got != "plain" {
			t.Errorf("windowError = %q, want the text", got)
		}
	})
}

// a uiErr keeps its key and English text, and its wrapped error
func TestUIErrKeysAndTexts(t *testing.T) {
	base := errors.New("boom")
	cases := []struct {
		err  error
		key  string
		text string
	}{
		{errBusy, "errors.busy", "a series is loading or a download is running"},
		{uiErr("errors.folderMissing", map[string]any{"path": "/x"}, nil), "errors.folderMissing", "the folder /x doesn't exist"},
		{uiErr("errors.chapterMissing", map[string]any{"index": 3}, nil), "errors.chapterMissing", "chapter 3 is not in the list"},
		{uiErr("errors.updateDownload", map[string]any{"reason": "boom"}, base), "errors.updateDownload", "couldn't download the update: boom"},
	}
	for _, c := range cases {
		var ue *uiError
		if !errors.As(c.err, &ue) || ue.Key != c.key {
			t.Errorf("%v: message key = %+v, want %q", c.err, ue, c.key)
			continue
		}
		if c.err.Error() != c.text {
			t.Errorf("Error() = %q, want %q", c.err.Error(), c.text)
		}
	}
	if !errors.Is(uiErr("errors.updateDownload", nil, base), base) {
		t.Error("errors.Is does not reach the wrapped error")
	}
}

// a shared error the window translates keeps its English text under its key
func TestWindowErrorTranslatesSharedErrors(t *testing.T) {
	got := windowError(fmt.Errorf("grouping: %w", core.ErrNoVolumes))
	var back Message
	if err := json.Unmarshal([]byte(got), &back); err != nil || back.Key != "errors.noVolumes" || back.Text != core.ErrNoVolumes.Error() {
		t.Errorf("windowError = %q, want the errors.noVolumes message", got)
	}
}

// messageOf is nil for an error the window doesn't translate, and the message
// of a shared sentinel wrapped or not
func TestMessageOf(t *testing.T) {
	if m := messageOf(errors.New("plain")); m != nil {
		t.Errorf("plain error got message %+v", m)
	}
	if m := messageOf(fmt.Errorf("x: %w", core.ErrLocked)); m == nil || m.Key != "job.reason.locked" {
		t.Errorf("wrapped ErrLocked = %+v, want job.reason.locked", m)
	}
	if m := messageOf(core.ErrNoPages); m == nil || m.Key != "job.reason.noPages" {
		t.Errorf("ErrNoPages = %+v, want job.reason.noPages", m)
	}
}

// messageKeyRE matches a literal key passed to msg or uiErr
var messageKeyRE = regexp.MustCompile(`\b(?:msg|uiErr)\("([^"]+)"`)

func scanMessageKeys(src string) []string {
	var keys []string
	for _, m := range messageKeyRE.FindAllStringSubmatch(src, -1) {
		keys = append(keys, m[1])
	}

	return keys
}

func inCatalog(key string) bool {
	for _, k := range []string{key, key + "_one", key + "_other"} {
		if _, ok := catalog[k]; ok {
			return true
		}
	}

	return false
}

func TestScanMessageKeys(t *testing.T) {
	src := `msg("a.b", nil)
uiErr("c.d", nil, err)
msg(key, nil)
other("e.f")`
	got := scanMessageKeys(src)
	if want := []string{"a.b", "c.d"}; !reflect.DeepEqual(got, want) {
		t.Errorf("scanMessageKeys = %v, want %v", got, want)
	}
}

func TestMessageKeysExistInEnglish(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range scanMessageKeys(string(src)) {
			checked++
			if !inCatalog(key) {
				t.Errorf("%s uses message key %q, missing from frontend/src/locales/en", file, key)
			}
		}
	}
	t.Logf("checked %d message keys", checked)
}
