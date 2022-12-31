package models

type Site interface {
	Test() bool
	FetchChapters() Filterables
	FetchChapter(Filterable) Chapter
	Title() string
}
