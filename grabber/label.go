// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "strconv"

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

// partLabel names a chapter's part: "Part 2" for a number, a word ("前編") as is
func partLabel(part string) string {
	if _, err := strconv.ParseFloat(part, 64); err == nil {
		return partWord + " " + part
	}
	return part
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
