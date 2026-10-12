// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package browser

import (
	"context"
	"errors"
	"testing"
)

func TestParseSetup(t *testing.T) {
	wait, result, err := parseSetup(`{"wait":true,"result":{"selected":"English"}}`)
	if err != nil || !wait || string(result) != `{"selected":"English"}` {
		t.Errorf("parseSetup = %v, %s, %v; want wait and the result", wait, result, err)
	}

	wait, _, err = parseSetup(`{"wait":false,"result":null}`)
	if err != nil || wait {
		t.Errorf("parseSetup without a request = %v, %v; want no wait", wait, err)
	}

	if _, _, err := parseSetup(""); err == nil {
		t.Error("parseSetup of an empty result should fail")
	}
}

func TestRenderError(t *testing.T) {
	timedOut, cancel := context.WithCancel(context.Background())
	cancel()
	cause := context.Canceled

	var ce *challengeError
	if err := renderError(timedOut, cause, "https://example.com/a", "#list", false); !errors.As(err, &ce) {
		t.Errorf("timeout before the selector matched = %v, want a challengeError", err)
	}
	err := renderError(timedOut, cause, "https://example.com/a", "#list", true)
	if errors.As(err, &ce) || !errors.Is(err, cause) {
		t.Errorf("timeout after the selector matched = %v, want the plain error, not a challenge", err)
	}
	other := errors.New("boom")
	if err := renderError(context.Background(), other, "https://example.com/a", "#list", false); err != other {
		t.Errorf("a failure without a timeout = %v, want it as is", err)
	}
}
