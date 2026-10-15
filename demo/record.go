//go:build demo

package demo

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	stdhttp "net/http"
)

// recordEnv names the directory to record into instead of playing back. The
// requests then go to the live sites, and the bodies and manifest.json are
// written there, ready to copy into demo/fixtures.
const recordEnv = "MANGA_DOWNLOADER_DEMO_RECORD"

// recorder passes requests to the live sites and records the answers
type recorder struct {
	dir  string
	live stdhttp.RoundTripper

	mu      sync.Mutex
	entries map[string]entry
}

// newRecorder records into dir, keeping the entries an earlier run put there
func newRecorder(dir string) (*recorder, error) {
	r := &recorder{dir: dir, live: &stdhttp.Transport{DisableCompression: true}, entries: map[string]entry{}}
	raw, err := os.ReadFile(filepath.Join(dir, manifestName))
	if errors.Is(err, fs.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return nil, err
	}
	var list []entry
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("demo: reading %s: %w", manifestName, err)
	}
	for _, e := range list {
		r.entries[key(e.Method, e.URL)] = e
	}
	return r, nil
}

// RoundTrip records the response to req. Images are recorded as placeholders
// and never saved, so no site's pages end up in the fixtures.
func (r *recorder) RoundTrip(req *stdhttp.Request) (*stdhttp.Response, error) {
	live, err := r.live.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer live.Body.Close()
	body, err := io.ReadAll(live.Body)
	if err != nil {
		return nil, err
	}

	e := entry{Method: req.Method, URL: req.URL.String(), Status: live.StatusCode, Location: live.Header.Get("Location")}
	if strings.HasPrefix(live.Header.Get("Content-Type"), "image/") {
		e.Image = true
		if err := r.save(e, nil); err != nil {
			return nil, err
		}
		return placeholder(req)
	}

	// a redirect or an empty answer has no body worth a file
	if len(body) > 0 {
		sum := sha256.Sum256([]byte(key(e.Method, e.URL)))
		e.File = hex.EncodeToString(sum[:8])
	}
	if err := r.save(e, body); err != nil {
		return nil, err
	}
	return response(req, live.StatusCode, e.Location, body), nil
}

// save writes the body, if any, and rewrites the manifest with e in it
func (r *recorder) save(e entry, body []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if e.File != "" {
		if err := os.MkdirAll(filepath.Join(r.dir, "bodies"), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(r.dir, "bodies", e.File), body, 0o644); err != nil {
			return err
		}
	}

	r.entries[key(e.Method, e.URL)] = e
	list := make([]entry, 0, len(r.entries))
	for _, v := range r.entries {
		list = append(list, v)
	}
	// sorted, so re-recording the same requests produces the same manifest
	sort.Slice(list, func(i, j int) bool { return list[i].URL < list[j].URL })

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(list); err != nil {
		return fmt.Errorf("demo: encoding %s: %w", manifestName, err)
	}
	return os.WriteFile(filepath.Join(r.dir, manifestName), buf.Bytes(), 0o644)
}
