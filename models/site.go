package models

type Site interface {
	Test() bool
	FetchChapters(string) Filterables
	FetchChapter(Filterable) Chapter
	GetBaseUrl() string
	Title() string
}
