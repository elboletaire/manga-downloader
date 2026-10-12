// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package browser

import (
	"strings"
	"testing"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/target"
)

func TestIsPopupOf(t *testing.T) {
	tabID := target.ID("tab-1")

	cases := []struct {
		name string
		ti   *target.Info
		want bool
	}{
		{
			name: "popup opened by our tab",
			ti:   &target.Info{TargetID: "popup-1", Type: "page", OpenerID: tabID},
			want: true,
		},
		{
			name: "our own tab (no opener)",
			ti:   &target.Info{TargetID: tabID, Type: "page", OpenerID: ""},
			want: false,
		},
		{
			name: "page opened by someone else",
			ti:   &target.Info{TargetID: "other-1", Type: "page", OpenerID: "other-opener"},
			want: false,
		},
		{
			name: "non-page target opened by our tab (e.g. worker)",
			ti:   &target.Info{TargetID: "worker-1", Type: "worker", OpenerID: tabID},
			want: false,
		},
		{
			name: "nil TargetInfo",
			ti:   nil,
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPopupOf(tc.ti, tabID); got != tc.want {
				t.Errorf("isPopupOf(%+v, %q) = %v, want %v", tc.ti, tabID, got, tc.want)
			}
		})
	}
}

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

func TestPopupClosedNoticeText(t *testing.T) {
	// TargetInfo.URL is empty at EventTargetCreated time, so the notice must not
	// attempt to embed a host (which renders as "by ."). Verify the constant is safe.
	for _, bad := range []string{"by .", "%s", "://", "by %"} {
		if strings.Contains(popupClosedNotice, bad) {
			t.Errorf("popupClosedNotice %q contains %q; must not reference the URL", popupClosedNotice, bad)
		}
	}
	if !strings.Contains(popupClosedNotice, "pop-up") {
		t.Errorf("popupClosedNotice %q doesn't mention pop-up", popupClosedNotice)
	}
}
