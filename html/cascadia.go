package html

import (
	"strings"

	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
)

func Reader(ht string) *html.Node {
	doc, err := html.Parse(strings.NewReader(ht))
	if err != nil {
		panic(err)
	}

	return doc
}

func Query(n *html.Node, query string) *html.Node {
	sel, err := cascadia.Parse(query)
	if err != nil {
		return &html.Node{}
	}
	return cascadia.Query(n, sel)
}

func QueryAll(n *html.Node, query string) []*html.Node {
	sel, err := cascadia.Parse(query)
	if err != nil {
		return []*html.Node{}
	}
	return cascadia.QueryAll(n, sel)
}

func AttrOr(n *html.Node, attrName, or string) string {
	for _, a := range n.Attr {
		if a.Key == attrName {
			return a.Val
		}
	}
	return or
}
