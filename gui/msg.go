// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/elboletaire/manga-downloader/core"
)

// Message is a string the window translates from Key and Args. Text is the
// English rendering, for logs and for a key the window's catalog lacks.
type Message struct {
	Key  string         `json:"key"`
	Args map[string]any `json:"args,omitempty"`
	Text string         `json:"text"`
}

// enLocales is the English catalog. Each file is a namespace, and nested keys
// flatten to ns.a.b.
//
//go:embed frontend/src/locales/en/*.json
var enLocales embed.FS

// catalog maps flattened keys (plurals as ns.key_one and ns.key_other) to text.
var catalog = loadCatalog()

func loadCatalog() map[string]string {
	const dir = "frontend/src/locales/en"
	entries, err := enLocales.ReadDir(dir)
	if err != nil {
		panic(fmt.Sprintf("%s: %v", dir, err))
	}

	out := map[string]string{}
	for _, e := range entries {
		path := dir + "/" + e.Name()
		raw, err := enLocales.ReadFile(path)
		if err != nil {
			panic(fmt.Sprintf("%s: %v", path, err))
		}
		var tree map[string]any
		if err := json.Unmarshal(raw, &tree); err != nil {
			panic(fmt.Sprintf("%s: %v", path, err))
		}
		flatten(out, strings.TrimSuffix(e.Name(), ".json"), tree)
	}

	return out
}

func flatten(out map[string]string, prefix string, tree map[string]any) {
	for k, v := range tree {
		key := prefix + "." + k
		switch v := v.(type) {
		case string:
			out[key] = v
		case map[string]any:
			flatten(out, key, v)
		}
	}
}

// msg returns key with its English Text, {{name}} filled from args. A "count"
// arg picks the _one form for 1 and _other otherwise.
func msg(key string, args map[string]any) Message {
	return Message{Key: key, Args: args, Text: render(key, args)}
}

// placeholderRE matches a {{name}} placeholder in one pass, so an arg value
// that itself contains one isn't expanded again
var placeholderRE = regexp.MustCompile(`\{\{(\w+)\}\}`)

func render(key string, args map[string]any) string {
	text, ok := lookup(key, args)
	if !ok {
		return key
	}

	return placeholderRE.ReplaceAllStringFunc(text, func(placeholder string) string {
		name := placeholder[2 : len(placeholder)-2]
		if value, ok := args[name]; ok {
			return fmt.Sprint(value)
		}

		return placeholder
	})
}

func lookup(key string, args map[string]any) (string, bool) {
	if count, ok := args["count"]; ok {
		// English has only one and other, so 1 is the only singular
		form := "_other"
		if fmt.Sprint(count) == "1" {
			form = "_one"
		}
		if text, ok := catalog[key+form]; ok {
			return text, true
		}
	}
	text, ok := catalog[key]

	return text, ok
}

// uiError is an error the window shows translated. Error keeps the English
// text for logs.
type uiError struct {
	Message
	err error
}

func (e *uiError) Error() string { return e.Text }

func (e *uiError) Unwrap() error { return e.err }

// uiErr wraps err with a message for the window.
func uiErr(key string, args map[string]any, err error) error {
	return &uiError{Message: msg(key, args), err: err}
}

// sharedMessages are the shared errors the window shows translated, by key
var sharedMessages = []struct {
	err error
	key string
}{
	{core.ErrNoVolumes, "errors.noVolumes"},
	{core.ErrLocked, "job.reason.locked"},
	{core.ErrEmptyVolume, "job.reason.emptyVolume"},
	{core.ErrSkippedVolume, "job.reason.skipped"},
}

// messageOf is the message of err when the window translates it, nil otherwise
func messageOf(err error) *Message {
	var ue *uiError
	if errors.As(err, &ue) {
		m := ue.Message
		return &m
	}
	for _, s := range sharedMessages {
		if errors.Is(err, s.err) {
			m := msg(s.key, nil)
			return &m
		}
	}

	return nil
}

// windowError is the Wails ErrorFormatter. Wails hands the formatted value to
// the window as an Error message, so a message error is its Message as JSON
// (the window parses it) and any other error its text.
func windowError(err error) string {
	if m := messageOf(err); m != nil {
		return messageJSON(*m)
	}

	return err.Error()
}

// messageJSON is m as JSON, its English text when that fails
func messageJSON(m Message) string {
	raw, err := json.Marshal(m)
	if err != nil {
		return m.Text
	}

	return string(raw)
}
