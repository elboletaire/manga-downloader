// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package browser

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
)

func TestPageStateJSHoldsTheExpressions(t *testing.T) {
	js := pageStateJS(PageWait{Puzzle: "P()", Ready: "R()", Count: "C()"})
	for _, want := range []string{"puzzle:!!(P())", "ready:!!(R())", "Number(C())||0"} {
		if !strings.Contains(js, want) {
			t.Errorf("pageStateJS = %s, want it to hold %q", js, want)
		}
	}
}

func TestPageStateParses(t *testing.T) {
	st, err := parsePageState(`{"puzzle":true,"ready":false,"count":21}`)
	if err != nil || st != (pageState{Puzzle: true, Count: 21}) {
		t.Errorf("parsePageState = %+v, %v", st, err)
	}
	// the state script answers {} when its expressions throw
	if st, err := parsePageState("{}"); err != nil || st != (pageState{}) {
		t.Errorf("parsePageState({}) = %+v, %v", st, err)
	}
	if _, err := parsePageState("not json"); err == nil {
		t.Error("parsePageState accepted what isn't JSON")
	}
}

// collectorFor makes a collector for the page's main frame "main"
func collectorFor() *requestCollector {
	return &requestCollector{
		match: regexp.MustCompile(`^https://c\d+\.example\.com/`),
		main:  "main",
		index: map[network.RequestID]int{},
	}
}

func sentReq(id, typ, frame, url string) network.EventRequestWillBeSent {
	return network.EventRequestWillBeSent{
		RequestID: network.RequestID(id),
		Type:      network.ResourceType(typ),
		FrameID:   cdp.FrameID(frame),
		Request:   &network.Request{URL: url, Method: "GET"},
	}
}

func TestCollectorKeepsMatchingRequestsInOrder(t *testing.T) {
	c := collectorFor()
	c.sent(sentReq("1", "Fetch", "main", "https://c1.example.com/decoy"))
	c.sent(sentReq("2", "Image", "main", "https://c1.example.com/a.png"))
	c.sent(sentReq("3", "Image", "main", "https://other.org/b.png"))
	c.sent(sentReq("4", "Image", "main", "https://c1.example.com/c.png"))
	c.finished(network.EventLoadingFinished{RequestID: "4"})
	c.finished(network.EventLoadingFinished{RequestID: "2"})

	// the other host doesn't match the pattern, so it's not kept at all
	want := []Request{
		{URL: "https://c1.example.com/decoy", Type: "Fetch"},
		{URL: "https://c1.example.com/a.png", Type: "Image", Loaded: true},
		{URL: "https://c1.example.com/c.png", Type: "Image", Loaded: true},
	}
	if got := c.snapshot(); !reflect.DeepEqual(got, want) {
		t.Errorf("requests = %+v, want %+v", got, want)
	}
	if n := c.images(); n != 2 {
		t.Errorf("images = %d, want 2", n)
	}
}

func TestCollectorCountsImagesSentNotYetLoaded(t *testing.T) {
	c := collectorFor()
	c.sent(sentReq("1", "Fetch", "main", "https://c1.example.com/decoy"))
	c.sent(sentReq("2", "Image", "main", "https://c1.example.com/a.png"))
	c.sent(sentReq("3", "Image", "main", "https://c1.example.com/b.png"))
	// a page requested again is still one page
	c.sent(sentReq("4", "Image", "main", "https://c1.example.com/a.png"))

	// a sent Image counts, loaded or not: only its URL is needed
	if n := c.images(); n != 2 {
		t.Errorf("images = %d, want the two distinct sent Image URLs", n)
	}
}

func TestCollectorRecordsFailedRequests(t *testing.T) {
	c := collectorFor()
	c.sent(sentReq("1", "Image", "main", "https://c1.example.com/a.png"))
	c.sent(sentReq("2", "Image", "main", "https://c1.example.com/b.png"))
	c.finished(network.EventLoadingFinished{RequestID: "1"})
	c.failed(network.EventLoadingFailed{RequestID: "2"})

	want := []Request{
		{URL: "https://c1.example.com/a.png", Type: "Image", Loaded: true},
		{URL: "https://c1.example.com/b.png", Type: "Image", Failed: true},
	}
	if got := c.snapshot(); !reflect.DeepEqual(got, want) {
		t.Errorf("requests = %+v, want %+v", got, want)
	}
}

func TestCollectorStartsOverOnAMainFrameDocument(t *testing.T) {
	c := collectorFor()
	c.sent(sentReq("1", "Image", "main", "https://c1.example.com/old.png"))
	c.finished(network.EventLoadingFinished{RequestID: "1"})

	// the reader reloads itself after a solved puzzle
	c.sent(sentReq("2", "Document", "main", "https://www.example.com/reader"))
	c.sent(sentReq("3", "Image", "main", "https://c1.example.com/new.png"))
	// a finish for a request from before the reload is ignored
	c.finished(network.EventLoadingFinished{RequestID: "1"})

	want := []Request{{URL: "https://c1.example.com/new.png", Type: "Image"}}
	if got := c.snapshot(); !reflect.DeepEqual(got, want) {
		t.Errorf("requests = %+v, want %+v", got, want)
	}
}

func TestCollectorIgnoresSubFrameDocuments(t *testing.T) {
	c := collectorFor()
	c.sent(sentReq("1", "Image", "main", "https://c1.example.com/a.png"))
	c.sent(sentReq("2", "Document", "frame2", "https://ads.example.org/"))
	c.sent(sentReq("3", "Image", "main", "https://c1.example.com/b.png"))

	if got := c.snapshot(); len(got) != 2 {
		t.Errorf("requests = %+v, want the two images kept", got)
	}
}
