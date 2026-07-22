// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestUtoonTest(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://www.utoon.us/en/manga/gachiakuta.html", true},
		{"https://utoon.us/en/manga/gachiakuta.html", true},
		{"https://www.utoon.net/manga/gachiakuta", false}, // hijacked/offline domain, not supported
		{"https://mangak.io/some-manga", false},
	}

	for _, c := range cases {
		u := Utoon{Grabber: &Grabber{URL: c.url}}
		got, err := u.Test()
		if err != nil {
			t.Errorf("Test(%q) returned unexpected error: %v", c.url, err)
			continue
		}
		if got != c.want {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

func TestUtoonAjaxConfigRe(t *testing.T) {
	html := `<script id="mangaverse-load-more-js-extra">
var mangaverse_ajax = {"ajax_url":"https://www.utoon.us/wp-admin/admin-ajax.php","nonce":"223204e60f","current_lang":"en"};
//# sourceURL=mangaverse-load-more-js-extra
</script>`

	m := utoonAjaxConfigRe.FindStringSubmatch(html)
	if len(m) < 3 {
		t.Fatalf("expected ajax config to be found, got %v", m)
	}
	if want := "https://www.utoon.us/wp-admin/admin-ajax.php"; m[1] != want {
		t.Errorf("ajax_url = %q, want %q", m[1], want)
	}
	if want := "223204e60f"; m[2] != want {
		t.Errorf("nonce = %q, want %q", m[2], want)
	}
}
