// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package http

import "testing"

// TestSessionCookie checks a cookie set for a domain is read back for it and
// its subdomains only
func TestSessionCookie(t *testing.T) {
	SetCookie("session-cookie.test", "visitor", "abc")

	cases := map[string]string{
		"session-cookie.test":     "abc",
		"www.session-cookie.test": "abc",
		"other.test":              "",
	}
	for host, want := range cases {
		if got := SessionCookie(host, "visitor"); got != want {
			t.Errorf("SessionCookie(%s) = %q, want %q", host, got, want)
		}
	}
	if got := SessionCookie("session-cookie.test", "missing"); got != "" {
		t.Errorf("unknown cookie = %q, want empty", got)
	}
}
