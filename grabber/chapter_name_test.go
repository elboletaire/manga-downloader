// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestChapterName(t *testing.T) {
	tests := []struct {
		name   string
		title  string
		number float64
		want   string
	}{
		{"chapter word and number", "Chapter 186 - Foo", 186, "Chapter 186 - Foo"},
		{"padded number", "Chapter 0186: Foo", 186, "Chapter 0186: Foo"},
		{"short chapter word", "Ch. 5: Foo", 5, "Ch. 5: Foo"},
		{"spanish word", "Capítulo 0012", 12, "Capítulo 0012"},
		{"french word", "Chapitre 7 : Foo", 7, "Chapitre 7 : Foo"},
		{"italian word", "Capitolo 3", 3, "Capitolo 3"},
		{"abbreviated cap", "Cap. 4", 4, "Cap. 4"},
		{"turkish word", "Bölüm 9", 9, "Bölüm 9"},
		{"russian word", "Глава 10", 10, "Глава 10"},
		{"volume before chapter", "Vol. 3 Chapter 12", 12, "Vol. 3 Chapter 12"},
		{"bare number", "186. Foo", 186, "186. Foo"},
		{"number alone", "Chapter 186", 186, "Chapter 186"},
		{"fractional kept", "Chapter 12.5 - Foo", 12.5, "Chapter 12.5 - Foo"},
		{"fractional title on whole number", "Chapter 12.5", 12, "Chapter 12 (Chapter 12.5)"},
		{"whole title on fractional number", "Chapter 12", 12.5, "Chapter 12.5 (Chapter 12)"},
		{"site numbering in title", "111. Dong-a-jul (The Lifeline (1))", 186, "Chapter 186 (111. Dong-a-jul (The Lifeline (1)))"},
		{"title without number", "Dead End", 36, "Chapter 36 (Dead End)"},
		{"title with a different leading number", "2nd Stage", 2, "Chapter 2 (2nd Stage)"},
		{"fractional number", "Prologue", 0.5, "Chapter 0.5 (Prologue)"},
		{"empty title", "", 7, "Chapter 7"},
		{"chapter zero keeps title", "Prologue", 0, "Prologue"},
		{"chapter zero titled with its number", "Chapter 0", 0, "Chapter 0"},
		{"unknown number and empty title", "", 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := &Chapter{Title: tt.title, Number: tt.number}
			if got := ChapterName(ch); got != tt.want {
				t.Errorf("ChapterName(%q, %v) = %q, want %q", tt.title, tt.number, got, tt.want)
			}
		})
	}
}
