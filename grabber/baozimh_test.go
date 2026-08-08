// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestBaozimhChapterNumber(t *testing.T) {
	tests := []struct {
		in     string
		want   float64
		wantOK bool
	}{
		// simplified script (mainland-China visitors)
		{"第109话", 109, true},
		// traditional script (everyone else)
		{"第109話", 109, true},
		// decimal and padded numbers
		{"第 1.5 话", 1.5, true},
		// multi-part chapters carry a (k/M) suffix on the reader page title
		{"第100話(1/2)", 100, true},
		// season-finale titles wrap the number in brackets
		{"第112話（第一季 最終話）", 112, true},
		{"第112話（第1季 最終話）", 112, true},
		// a free-badge prefix before the number is fine
		{"【免費】第109話", 109, true},
		// some series append a subtitle after the number ("第N話 標題")
		{"第1話 高武世界，夢中荒原", 1, true},
		{"第332話 抵達", 332, true},
		// other series list a bare number before the title, zero-padded or not
		{"001 傳染", 1, true},
		{"10 要人", 10, true},
		{"064 常總（下）", 64, true},
		{"062 校園（中）", 62, true},
		// the number can even run straight into the title
		{"40支援 （下）", 40, true},
		// entries without a number are skipped
		{"序章", 0, false},
		{"後記", 0, false},
		{"后记", 0, false},
		{"番外", 0, false},
		{"【免費】第一季 後記-3（JongBeom Lee, SOULPUNG）& 第二季預告", 0, false},
		// a digit-led date ("launch announcement") is not a chapter number
		{"7月18號正式上線！", 0, false},
		// update notices and cross-series promos carry no number either
		{"更新調整", 0, false},
		{"五一更新調整", 0, false},
		{"更新通知", 0, false},
		{"故事盡頭的時鐘發條", 0, false},
		{"傳說級新作來襲！", 0, false},
		// an unnumbered part (the site dropped the "36" prefix) is skipped too
		{"懸賞 （下）", 0, false},
		// a season-finale titled with a Chinese numeral can't be ranged
		{"第三季完結篇 破碎的拼圖", 0, false},
		// no chapter number anywhere
		{"", 0, false},
	}
	for _, tt := range tests {
		got, ok := baozimhChapterNumber(tt.in)
		if ok != tt.wantOK || got != tt.want {
			t.Errorf("baozimhChapterNumber(%q) = (%v, %v), want (%v, %v)", tt.in, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestBaozimhNextPartURL(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{
			// the part links found on real reader pages wrap their text in a
			// <span> and carry an absolute href
			name: "part page links the next part (traditional 下一頁)",
			html: `<html><body>
				<a href="https://www.twmanga.com/comic/foo/0_99_2.html"><span>下一頁</span></a>
			</body></html>`,
			want: "https://www.twmanga.com/comic/foo/0_99_2.html",
		},
		{
			name: "simplified 下一页 with a root-relative href",
			html: `<html><body>
				<a href="/user/page_direct/0_99_2.html">下一页</a>
			</body></html>`,
			want: "/user/page_direct/0_99_2.html",
		},
		{
			// the last part links 上一頁 (prev page) and 下一話 (next *chapter*),
			// neither of which is a next-page link
			name: "last part has no next-page link",
			html: `<html><body>
				<a href="prev.html"><span>上一頁</span></a>
				<a href="next-chapter.html"><span>下一話</span></a>
			</body></html>`,
			want: "",
		},
		{
			name: "single-part chapter has no navigation",
			html: `<html><body>
				<p>no nav</p>
			</body></html>`,
			want: "",
		},
		{
			// 上一頁 also contains the 頁 character but must not match, and the
			// first 下一頁 anchor in the document wins
			name: "prev and next nav both present",
			html: `<html><body>
				<a href="part-1.html">上一頁</a>
				<a href="part-3.html">下一頁</a>
			</body></html>`,
			want: "part-3.html",
		},
	}
	for _, tt := range tests {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(tt.html))
		if err != nil {
			t.Fatalf("%s: parse: %v", tt.name, err)
		}
		if got := baozimhNextPartURL(doc); got != tt.want {
			t.Errorf("%s: baozimhNextPartURL() = %q, want %q", tt.name, got, tt.want)
		}
	}
}
