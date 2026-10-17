// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"regexp"
	"strconv"
)

// chapterWords are the words labels name a chapter with, by Label.Lang ("" is
// English), kept in this one place so they can be localized
var chapterWords = map[string]string{"": "Chapter", "es": "Capítulo", "fr": "Chapitre", "tr": "Bölüm"}

// partWord names the part of a chapter split over several posts
const partWord = "Part"

// NumberLabel names a chapter by its number alone: "Chapter 12", "Chapter 10.5"
func NumberLabel(number float64) string {
	return NumberTextLabel(strconv.FormatFloat(number, 'f', -1, 64))
}

// NumberTextLabel names a chapter by its number written as text: "Chapter 0012"
func NumberTextLabel(number string) string {
	return chapterWords[""] + " " + number
}

// LabelStyle is how ChapterLabel composes a chapter's label from its number and
// the site's title. The zero value shows the title as is.
type LabelStyle uint8

const (
	// LabelTitle is the title as is
	LabelTitle LabelStyle = iota
	// LabelNumbered is the title or, when the site gives none, the chapter word
	// and number. A part goes with either: "Chapter 364 Part 2", "Part 2 Title"
	LabelNumbered
	// LabelPrefixed is the chapter word and number, then " - " and the title
	LabelPrefixed
	// LabelNumberFirst is the chapter word and number, then the title:
	// "Chapter 12 Title"
	LabelNumberFirst
	// LabelUnnumbered is the title, or the chapter word alone (a one-shot
	// numbered by its position in the list)
	LabelUnnumbered
)

// Label is what ChapterLabel composes a chapter's label from, besides its
// Number and Title
type Label struct {
	Style LabelStyle
	// Number is the site's number text when it writes it its own way ("05")
	Number string
	// Part is the part of a chapter split over several posts
	Part string
	// Lang picks the chapter word from chapterWords
	Lang string
}

// Labeler is a chapter that composes its own label
type Labeler interface {
	GetLabel() string
}

// ChapterLabel is what a chapter is shown and named with ({{.Title}} in file
// names): its number and the site's title, composed as its Label says
func ChapterLabel(f Filterable) string {
	if l, ok := f.(Labeler); ok {
		return l.GetLabel()
	}
	return f.GetTitle()
}

// GetLabel returns the chapter's label, see ChapterLabel. Composed labels get
// the same cleanup as GetTitle.
func (c Chapter) GetLabel() string {
	word := chapterWords[c.Label.Lang]
	number := c.Label.Number
	if number == "" {
		number = strconv.FormatFloat(c.Number, 'f', -1, 64)
	}
	title := c.GetTitle()
	switch c.Label.Style {
	case LabelNumbered:
		if title == "" {
			title = word + " " + number
			if c.Label.Part != "" {
				title += " " + partLabel(c.Label.Part)
			}
		} else if c.Label.Part != "" {
			title = partLabel(c.Label.Part) + " " + title
		}
	case LabelPrefixed:
		if title == "" {
			return word + " " + number
		}
		return cleanTitle(word + " " + number + " - " + c.Title)
	case LabelNumberFirst:
		return cleanTitle(word + " " + number + " " + c.Title)
	case LabelUnnumbered:
		if title == "" {
			return word
		}
	}
	return title
}

// partWordRe is a word part with its number, "前編1"
var partWordRe = regexp.MustCompile(`^(\D+)(\d+)$`)

// partLabel names a chapter's part: "Part 2" for a number, "前編 Part 1" for a
// word with one, a word ("前編") as is
func partLabel(part string) string {
	if _, err := strconv.ParseFloat(part, 64); err == nil {
		return partWord + " " + part
	}
	if m := partWordRe.FindStringSubmatch(part); m != nil {
		return m[1] + " " + partWord + " " + m[2]
	}
	return part
}

// PartOf splits a listed chapter's label into its part ("Part 2", "前編") and its
// title without it, "" when the site gives none, as its merged file is named.
// part is "" for a chapter without one.
func PartOf(f Filterable) (part, title string) {
	l := labelOf(f)
	if l.Style != LabelNumbered || l.Part == "" {
		return "", ""
	}
	return partLabel(l.Part), f.GetTitle()
}

// PartsTitle is the title a chapter's parts share without their parts, which
// names their merged file and its header: the title PartOf gives when every part
// has a part and they all agree, "" otherwise
func PartsTitle(parts Filterables) string {
	title := ""
	for i, p := range parts {
		part, t := PartOf(p)
		if part == "" || (i > 0 && t != title) {
			return ""
		}
		title = t
	}
	return title
}

// PartLess orders the parts of a chapter as their merged file lists them: by
// their part ("Part 2" before "Part 10"), else by their label ("356a" before "356b")
func PartLess(a, b Filterable) bool {
	return naturalLess(partSortKey(a), partSortKey(b))
}

// partSortKey is the part a listed chapter's PartOf gives, else its label
func partSortKey(f Filterable) string {
	if part, _ := PartOf(f); part != "" {
		return part
	}
	return ChapterLabel(f)
}

// labelOf returns the Label of a listed chapter, for the chapter FetchChapter
// builds from it to be labelled the same
func labelOf(f Filterable) Label {
	if c, ok := f.(interface{ label() Label }); ok {
		return c.label()
	}
	return Label{}
}

func (c Chapter) label() Label {
	return c.Label
}
