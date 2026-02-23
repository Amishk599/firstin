package adapter

import (
	"html"
	"regexp"
	"strings"
)

var htmlTagRegex = regexp.MustCompile(`<[^>]*>`)

// extractText converts an HTML or HTML-encoded string to plain text.
// It first unescapes HTML entities (handles Greenhouse's double-encoding;
// no-op on already-real HTML), strips all tags, then collapses whitespace.
func extractText(content string) string {
	unescaped := html.UnescapeString(content)
	plain := htmlTagRegex.ReplaceAllString(unescaped, "")
	return strings.Join(strings.Fields(plain), " ")
}
