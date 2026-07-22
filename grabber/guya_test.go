// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestGuyaSlug(t *testing.T) {
	cases := []struct {
		url     string
		want    string
		wantErr bool
	}{
		{"https://guya.moe/read/manga/Kaguya-Wants-To-Be-Confessed-To/", "Kaguya-Wants-To-Be-Confessed-To", false},
		{"https://guya.moe/read/manga/Kaguya-Wants-To-Be-Confessed-To/281/7/", "Kaguya-Wants-To-Be-Confessed-To", false},
		{"https://guya.cubari.moe/read/manga/Oshi-No-Ko/", "Oshi-No-Ko", false},
		{"https://danke.moe/read/manga/100-girlfriends/", "100-girlfriends", false},
		{"https://guya.moe/", "", true},
	}

	for _, c := range cases {
		g := Guya{Grabber: &Grabber{URL: c.url}}
		got, err := g.slug()
		if (err != nil) != c.wantErr {
			t.Errorf("slug(%q) error = %v, wantErr %v", c.url, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("slug(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

func TestGuyaTest(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://guya.moe/read/manga/Kaguya-Wants-To-Be-Confessed-To/", true},
		{"https://guya.cubari.moe/read/manga/Kaguya-Wants-To-Be-Confessed-To/", true},
		{"https://danke.moe/read/manga/100-girlfriends/", true},
		{"https://cubari.moe/read/gist/...", false},
		{"https://mangak.io/some-series", false},
	}

	for _, c := range cases {
		g := Guya{Grabber: &Grabber{URL: c.url}}
		got, _ := g.Test()
		if got != c.want {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

func TestGuyaPreferredGroup(t *testing.T) {
	cases := []struct {
		name          string
		groups        map[string][]string
		preferredSort []string
		want          string
	}{
		{
			name:          "single group",
			groups:        map[string][]string{"1": {"01.png"}},
			preferredSort: []string{"7", "3", "2", "1", "4"},
			want:          "1",
		},
		{
			name:          "picks first match in preferred_sort order",
			groups:        map[string][]string{"3": {"01.png"}, "4": {"01.png"}},
			preferredSort: []string{"7", "3", "2", "1", "4"},
			want:          "3",
		},
		{
			name:          "falls back to any group when none preferred",
			groups:        map[string][]string{"9": {"01.png"}},
			preferredSort: []string{"7", "3", "2", "1", "4"},
			want:          "9",
		},
		{
			name:          "no groups",
			groups:        map[string][]string{},
			preferredSort: []string{"7", "3"},
			want:          "",
		},
	}

	for _, c := range cases {
		if got := preferredGroup(c.groups, c.preferredSort); got != c.want {
			t.Errorf("%s: preferredGroup() = %q, want %q", c.name, got, c.want)
		}
	}
}
