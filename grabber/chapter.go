package grabber

import "strings"

// Chapter represents a manga chapter
type Chapter struct {
	// Title is the chapter title
	Title string
	// Number is the chapter number
	Number float64
	// PagesCount is the number of pages in the chapter
	PagesCount int64
	// Pages is the list of pages in the chapter
	Pages []Page
	// Language is the chapter language
	Language string
}

// Page represents a chapter page
type Page struct {
	// Number is the page number
	Number int64
	// URL is the page URL
	URL string
}

// GetNumber returns the chapter number
func (c Chapter) GetNumber() float64 {
	return c.Number
}

// GetTitle returns the chapter title removing whitespace and newlines
func (c Chapter) GetTitle() string {
	title := strings.TrimSpace(c.Title)
	title = strings.ReplaceAll(title, "\n", " ")
	return title
}
