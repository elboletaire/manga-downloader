package grabber

import "strings"

// Chapter represents a manga chapter
type Chapter struct {
	Title      string
	Number     float64
	PagesCount int64
	Pages      []Page
	Language   string
}

// Page represents a chapter page
type Page struct {
	Number int64
	URL    string
}

// GetNumber returns the chapter number
func (c Chapter) GetNumber() float64 {
	return c.Number
}

// GetTitle returns the chapter title
func (c Chapter) GetTitle() string {
	return strings.TrimSpace(c.Title)
}
