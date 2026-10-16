// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"strconv"
)

// chapterWord is the word labels name a chapter with, kept in this one place
// so it can be localized
const chapterWord = "Chapter"

// NumberLabel names a chapter by its number alone: "Chapter 12", "Chapter 10.5"
func NumberLabel(number float64) string {
	return chapterWord + " " + strconv.FormatFloat(number, 'f', -1, 64)
}

// LabelStyle is how ChapterLabel composes a chapter's label from its number and
// the site's title. The zero value shows the title as is, for grabbers that
// still write the chapter word into it.
type LabelStyle uint8

const (
	// LabelTitle is the title as is
	LabelTitle LabelStyle = iota
	// LabelNumbered is the title, or NumberLabel when the site gives none
	LabelNumbered
	// LabelPrefixed is NumberLabel, then " - " and the title when there is one
	LabelPrefixed
	// LabelPadded is the chapter word, the number's whole part padded to four
	// digits, then the title: "Chapter 0012 Title"
	LabelPadded
)

// Labeler is a chapter that composes its own label
type Labeler interface {
	GetLabel() string
}

// ChapterLabel is what a chapter is shown and named with ({{.Title}} in file
// names): its number and the site's title, composed as its LabelStyle says
func ChapterLabel(f Filterable) string {
	if l, ok := f.(Labeler); ok {
		return l.GetLabel()
	}
	return f.GetTitle()
}

// GetLabel returns the chapter's label, see ChapterLabel. Composed labels go
// through the same cleanup as GetTitle, as when grabbers wrote them as titles.
func (c Chapter) GetLabel() string {
	switch c.LabelStyle {
	case LabelNumbered:
		if c.GetTitle() == "" {
			return NumberLabel(c.Number)
		}
	case LabelPrefixed:
		if c.GetTitle() == "" {
			return NumberLabel(c.Number)
		}
		return cleanTitle(NumberLabel(c.Number) + " - " + c.Title)
	case LabelPadded:
		return cleanTitle(fmt.Sprintf("%s %04d %s", chapterWord, int64(c.Number), c.Title))
	}
	return c.GetTitle()
}

// numberTitle is the title of a chapter the site names by its number alone:
// empty, for ChapterLabel to name, unless the site writes the number its own
// way ("05"), which the label keeps.
func numberTitle(raw string, number float64) string {
	if raw == strconv.FormatFloat(number, 'f', -1, 64) {
		return ""
	}
	return chapterWord + " " + raw
}

// GetLabelStyle returns how the chapter's label is composed
func (c Chapter) GetLabelStyle() LabelStyle {
	return c.LabelStyle
}

// labelStyleOf returns the LabelStyle of a listed chapter, for the chapter
// FetchChapter builds from it to be labelled the same
func labelStyleOf(f Filterable) LabelStyle {
	if c, ok := f.(interface{ GetLabelStyle() LabelStyle }); ok {
		return c.GetLabelStyle()
	}
	return LabelTitle
}
