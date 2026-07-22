// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestProjectsukiBookID(t *testing.T) {
	cases := []struct {
		url     string
		want    int
		wantErr bool
	}{
		{"https://projectsuki.com/book/159270", 159270, false},
		{"https://projectsuki.com/book/159270/", 159270, false},
		{"https://projectsuki.com/", 0, true},
	}

	for _, c := range cases {
		p := Projectsuki{Grabber: &Grabber{URL: c.url}}
		got, err := p.bookID()
		if (err != nil) != c.wantErr {
			t.Errorf("bookID(%q) error = %v, wantErr %v", c.url, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("bookID(%q) = %d, want %d", c.url, got, c.want)
		}
	}
}

func TestProjectsukiChapterNumRe(t *testing.T) {
	cases := []struct {
		text string
		want string
		ok   bool
	}{
		{"Chapter 884 - Interrogation", "884", true},
		{"Chapter 233", "233", true},
		{"Chapter 205.9", "205.9", true},
		{"No chapter here", "", false},
	}

	for _, c := range cases {
		m := projectsukiChapterNumRe.FindStringSubmatch(c.text)
		if c.ok && (m == nil || m[1] != c.want) {
			t.Errorf("projectsukiChapterNumRe(%q) = %v, want %q", c.text, m, c.want)
		}
		if !c.ok && m != nil {
			t.Errorf("projectsukiChapterNumRe(%q) = %v, want no match", c.text, m)
		}
	}
}

func TestProjectsukiImageRe(t *testing.T) {
	cases := []struct {
		src      string
		wantHash string
		wantPage string
		wantOk   bool
	}{
		{"https://projectsuki.com/images/gallery/159270/5b1dd2d90a6d40fe94eed37ac2186e7e/001?", "5b1dd2d90a6d40fe94eed37ac2186e7e", "1", true},
		{"https://projectsuki.com/images/gallery/159270/5b1dd2d90a6d40fe94eed37ac2186e7e/0016?", "5b1dd2d90a6d40fe94eed37ac2186e7e", "16", true},
		{"https://projectsuki.com/images/gallery/159270/thumb.jpg", "", "", false},
	}

	for _, c := range cases {
		m := projectsukiImageRe.FindStringSubmatch(c.src)
		if !c.wantOk {
			if m != nil {
				t.Errorf("projectsukiImageRe(%q) = %v, want no match", c.src, m)
			}
			continue
		}
		if len(m) != 4 {
			t.Errorf("projectsukiImageRe(%q) = %v, want 4 submatches", c.src, m)
			continue
		}
		if m[2] != c.wantHash {
			t.Errorf("projectsukiImageRe(%q) hash = %q, want %q", c.src, m[2], c.wantHash)
		}
		if m[3] != c.wantPage {
			t.Errorf("projectsukiImageRe(%q) page = %q, want %q", c.src, m[3], c.wantPage)
		}
	}
}
