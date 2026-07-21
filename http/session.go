// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package http

import (
	"strings"
	"sync"
)

// session holds cookies and user agent harvested from the browser package,
// so plain HTTP requests can reuse the browser session (e.g. cloudflare
// clearance cookies tied to the browser's user agent)
var session = struct {
	sync.RWMutex
	userAgent string
	// cookies per registrable domain: domain -> name -> value
	cookies map[string]map[string]string
}{
	cookies: map[string]map[string]string{},
}

// SetUserAgent overrides the user agent sent with every request. Cloudflare
// clearance cookies are only valid with the exact user agent that solved the
// challenge, so both must be set together.
func SetUserAgent(ua string) {
	session.Lock()
	defer session.Unlock()
	session.userAgent = ua
}

// SetCookie stores a cookie to be sent with requests to the given domain and
// its subdomains
func SetCookie(domain, name, value string) {
	session.Lock()
	defer session.Unlock()
	if session.cookies[domain] == nil {
		session.cookies[domain] = map[string]string{}
	}
	session.cookies[domain][name] = value
}

// sessionUserAgent returns the harvested user agent, or fallback if none
func sessionUserAgent(fallback string) string {
	session.RLock()
	defer session.RUnlock()
	if session.userAgent != "" {
		return session.userAgent
	}
	return fallback
}

// sessionCookies returns the "name=value; ..." cookie header for a host, or
// an empty string if there are no cookies for it
func sessionCookies(host string) string {
	session.RLock()
	defer session.RUnlock()

	pairs := []string{}
	for domain, cookies := range session.cookies {
		if host != domain && !strings.HasSuffix(host, "."+domain) {
			continue
		}
		for name, value := range cookies {
			pairs = append(pairs, name+"="+value)
		}
	}

	return strings.Join(pairs, "; ")
}
