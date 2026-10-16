// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthorizeMadaradex(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.FormValue("action") != "mdx_auth_refresh" {
			t.Errorf("unexpected action %q", r.FormValue("action"))
		}
		if c, err := r.Cookie("mdx_fp"); err != nil || len(c.Value) != 32 {
			t.Errorf("missing or malformed mdx_fp cookie: %v", err)
		}
	}))
	defer srv.Close()

	html := fmt.Sprintf(`const configuredDomain = ".127.0.0.1";
const ajaxUrl = "%s"; 'action=mdx_auth_refresh'`, strings.ReplaceAll(srv.URL, "/", `\/`))
	authorizeMadaradex(html)
	authorizeMadaradex(html)
	if hits != 1 {
		t.Errorf("handshake hits = %d, want 1", hits)
	}

	authorizeMadaradex("<html>no handshake</html>")
	if hits != 1 {
		t.Errorf("a page without the script triggered a request")
	}
}
