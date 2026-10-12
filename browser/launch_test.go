// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package browser

import (
	"testing"

	"github.com/chromedp/cdproto/emulation"
)

func TestUnheadBrands(t *testing.T) {
	brands := []*emulation.UserAgentBrandVersion{
		{Brand: "Not)A;Brand", Version: "8"},
		{Brand: "Chromium", Version: "138"},
		{Brand: "HeadlessChrome", Version: "138"},
		nil,
	}
	unheadBrands(brands)

	want := []string{"Not)A;Brand", "Chromium", "Google Chrome"}
	for i, w := range want {
		if brands[i].Brand != w || brands[i].Version == "" {
			t.Errorf("brand %d = %+v, want %q with its version", i, brands[i], w)
		}
	}
	unheadBrands(nil) // no brands: nothing to do
}
