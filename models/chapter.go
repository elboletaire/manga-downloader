package models

type Chapter struct {
	Title      string
	Number     float64
	PagesCount int64
	Pages      Pages
	Language   string
}
type Chapters []Chapter

func (c *Chapter) GetNumber() float64 {
	return c.Number
}
