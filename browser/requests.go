// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/elboletaire/manga-downloader/notice"
)

// Request is a network request a page sent, as GetPageRequests reports it
type Request struct {
	URL    string
	Type   string // the resource type ("Image", "Fetch", …)
	Loaded bool   // it finished loading
	Failed bool   // it failed to load
}

// PageWait says how GetPageRequests waits on a page the user may have to act in first
type PageWait struct {
	Match  *regexp.Regexp // the request URLs to collect
	Puzzle string         // JS, true while the page waits for the user
	Ready  string         // JS, true once the page is usable
	Count  string         // JS number of Image requests to wait for once Ready
}

// requestsStall is how long a page that's ready keeps waiting for its requests
// once none of them finishes any more
const requestsStall = 15 * time.Second

// pageStatePoll is how often the page's PageWait state is read
const pageStatePoll = 500 * time.Millisecond

// GetPageRequests renders pageURL like GetHTML, waiting through a puzzle the user
// solves (see PageWait), and returns the HTML and the matching requests in the
// order sent; a reload of the document starts them over.
func GetPageRequests(pageURL string, w PageWait, timeout time.Duration) (string, []Request, error) {
	abort := lock()
	defer mu.Unlock()

	var html string
	var reqs []Request
	err := runPage(abort, pageURL, timeout, true, func(ctx context.Context, h hooks) error {
		var err error
		html, reqs, err = collectPage(ctx, h, pageURL, w)
		return err
	})
	if err != nil {
		return "", nil, err
	}
	return html, reqs, nil
}

// collectPage is the page operation behind GetPageRequests
func collectPage(ctx context.Context, h hooks, pageURL string, w PageWait) (string, []Request, error) {
	var main cdp.FrameID
	if c := chromedp.FromContext(ctx); c != nil && c.Target != nil {
		main = cdp.FrameID(c.Target.TargetID)
	}
	col := &requestCollector{match: w.Match, main: main, index: map[network.RequestID]int{}}
	listen(chromedp.Events(ctx, network.RequestWillBeSent), col.sent)
	listen(chromedp.Events(ctx, network.LoadingFinished), col.finished)
	listen(chromedp.Events(ctx, network.LoadingFailed), col.failed)
	if NetLog != nil {
		listen(chromedp.Events(ctx, network.ResponseReceived), func(resp network.EventResponseReceived) {
			NetLog(resp.Response.URL, int(resp.Response.Status), resp.Response.MimeType)
		})
	}

	if err := chromedp.Do(ctx, send(network.Enable, network.EnableParams{}), chromedp.Navigate(pageURL)); err != nil {
		if ctx.Err() != nil {
			return "", nil, &challengeError{url: pageURL, selector: "the page's puzzle"}
		}
		return "", nil, err
	}

	js := pageStateJS(w)
	var (
		puzzled, ready bool
		lastImages     int
		grew           time.Time
	)
	tick := time.NewTicker(pageStatePoll)
	defer tick.Stop()
	for {
		if raw, err := chromedp.Run(ctx, chromedp.Evaluate[string](js)); err == nil {
			st, perr := parsePageState(raw)
			if perr == nil {
				if st.Puzzle && !puzzled {
					puzzled = true
					h.attention()
					h.rearm(visibleTimeout)
					notice.Info("%s shows a puzzle in the browser window: put the strips back in order there, and the chapter downloads by itself.", hostOf(pageURL))
				}
				if st.Ready && !ready {
					ready = true
					h.rendered()
					h.rearm(afterRenderTimeout)
					grew = time.Now()
					lastImages = col.images()
				}
				if ready {
					n := col.images()
					if n > lastImages {
						grew = time.Now()
					}
					lastImages = n
					if (st.Count > 0 && n >= st.Count) || time.Since(grew) >= requestsStall {
						break
					}
				}
			}
		}

		select {
		case <-ctx.Done():
			if !ready {
				return "", nil, &challengeError{url: pageURL, selector: "the page's puzzle"}
			}
			return "", nil, fmt.Errorf("%s loaded but its pages didn't finish in time", hostOf(pageURL))
		case <-tick.C:
		}
	}

	var html string
	if err := chromedp.Do(ctx, chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
		var err error
		html, err = documentHTML(ctx, t)
		return err
	}), chromedp.Func(harvestSession)); err != nil {
		return "", nil, err
	}
	return html, col.snapshot(), nil
}

// pageStateJS is the script that reads the PageWait state of the page as JSON
func pageStateJS(w PageWait) string {
	return fmt.Sprintf(
		`(function(){try{return JSON.stringify({puzzle:!!(%s),ready:!!(%s),count:Number(%s)||0})}catch(e){return '{}'}})()`,
		w.Puzzle, w.Ready, w.Count,
	)
}

// pageState is what pageStateJS reports
type pageState struct {
	Puzzle bool `json:"puzzle"`
	Ready  bool `json:"ready"`
	Count  int  `json:"count"`
}

// parsePageState reads the JSON pageStateJS returns
func parsePageState(raw string) (pageState, error) {
	var st pageState
	err := json.Unmarshal([]byte(raw), &st)
	return st, err
}

// requestCollector keeps the requests of a page that match its PageWait, in
// the order they were sent
type requestCollector struct {
	mu    sync.Mutex
	match *regexp.Regexp
	// main is the frame of the page's own document
	main  cdp.FrameID
	reqs  []Request
	index map[network.RequestID]int
}

// sent records a request, or starts the collection over when the page's
// document is loaded again
func (c *requestCollector) sent(e network.EventRequestWillBeSent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e.Type == network.ResourceTypeDocument && e.FrameID == c.main {
		c.reqs = nil
		c.index = map[network.RequestID]int{}
		return
	}
	if e.Request == nil || e.Request.Method != "GET" || !c.match.MatchString(e.Request.URL) {
		return
	}
	c.index[e.RequestID] = len(c.reqs)
	c.reqs = append(c.reqs, Request{URL: e.Request.URL, Type: string(e.Type)})
}

// finished marks a request as loaded
func (c *requestCollector) finished(e network.EventLoadingFinished) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if i, ok := c.index[e.RequestID]; ok {
		c.reqs[i].Loaded = true
	}
}

// failed marks a request as failed to load
func (c *requestCollector) failed(e network.EventLoadingFailed) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if i, ok := c.index[e.RequestID]; ok {
		c.reqs[i].Failed = true
	}
}

// images counts the distinct Image URLs collected, sent or not yet loaded
func (c *requestCollector) images() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	seen := map[string]bool{}
	for _, r := range c.reqs {
		if r.Type == "Image" {
			seen[r.URL] = true
		}
	}
	return len(seen)
}

// snapshot returns a copy of the collected requests
func (c *requestCollector) snapshot() []Request {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Request(nil), c.reqs...)
}
