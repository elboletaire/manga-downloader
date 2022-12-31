package models

type Page struct {
	Number int64
	URL    string
}

type Pages []Page

func (p *Page) GetNumber() float64 {
	return float64(p.Number)
}

func (i *Page) GetTitle() string {
	return i.URL
}
