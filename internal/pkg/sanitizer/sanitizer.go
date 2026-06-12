package sanitizer

import (
	"github.com/microcosm-cc/bluemonday"
)

var htmlPolicy *bluemonday.Policy

func init() {
	p := bluemonday.NewPolicy()

	p.AllowElements(
		"p", "b", "i", "strong", "em", "a", "code", "pre",
		"ul", "ol", "li",
		"h1", "h2", "h3", "h4", "h5", "h6",
		"br", "hr",
		"table", "thead", "tbody", "tfoot", "td", "th", "tr",
		"blockquote", "dl", "dt", "dd",
		"span", "div",
	)

	p.AllowAttrs("href").OnElements("a")
	p.AllowAttrs("src", "alt").OnElements("img")
	p.AllowAttrs("colspan", "rowspan").OnElements("td", "th")

	p.RequireNoFollowOnLinks(true)

	htmlPolicy = p
}

func SanitizeHTML(html string) string {
	return htmlPolicy.Sanitize(html)
}
