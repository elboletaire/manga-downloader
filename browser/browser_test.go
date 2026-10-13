// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package browser

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"
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

func TestStatusFromResponse(t *testing.T) {
	resp := func(typ network.ResourceType, code int64, hdr network.Headers) *network.EventResponseReceived {
		return &network.EventResponseReceived{RequestID: "r1", LoaderID: "r1", FrameID: "main", Type: typ,
			Response: &network.Response{URL: "https://x.test/", Status: code, Headers: hdr}}
	}
	e := statusFromResponse(resp(network.ResourceTypeDocument, 502, network.Headers{"retry-after": "60"}), "main")
	if e == nil || e.Code != 502 || e.RetryAfter != 60*time.Second {
		t.Errorf("502 document = %+v, want code 502 and a 60s Retry-After", e)
	}
	if e := statusFromResponse(resp(network.ResourceTypeDocument, 429, nil), "main"); e == nil || e.RetryAfter != 0 {
		t.Errorf("429 without Retry-After = %+v", e)
	}
	if e := statusFromResponse(resp(network.ResourceTypeDocument, 503, network.Headers{"Retry-After": "5"}), "main"); e == nil {
		t.Error("503 with Retry-After should be a StatusError")
	}
	subframe := resp(network.ResourceTypeDocument, 502, nil)
	subframe.FrameID = "iframe"
	for name, r := range map[string]*network.EventResponseReceived{
		"xhr 502":                resp(network.ResourceTypeXHR, 502, nil),
		"403":                    resp(network.ResourceTypeDocument, 403, nil),
		"200":                    resp(network.ResourceTypeDocument, 200, nil),
		"bare 503":               resp(network.ResourceTypeDocument, 503, nil),
		"cloudflare challenge":   resp(network.ResourceTypeDocument, 503, network.Headers{"Cf-Mitigated": "challenge", "retry-after": "5"}),
		"sub-frame document 502": subframe,
	} {
		if e := statusFromResponse(r, "main"); e != nil {
			t.Errorf("%s should not be a StatusError, got %+v", name, e)
		}
	}
}
