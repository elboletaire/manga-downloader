package grabber

import "strings"

type Chapter struct {
	Title      string
	Number     float64
	PagesCount int64
	Pages      Pages
	Language   string
}
type Chapters []Chapter

type Page struct {
	Number int64
	URL    string
}

type Pages []Page

func (c Chapter) GetNumber() float64 {
	return c.Number
}

func (c Chapter) GetTitle() string {
	return strings.TrimSpace(c.Title)
}
