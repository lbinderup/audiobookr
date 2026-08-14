package metadata

import (
	"html"
	"regexp"
	"strings"
)

var (
	htmlBreak = regexp.MustCompile(`(?i)<br\s*/?>|</p\s*>|</div\s*>|</li\s*>`)
	htmlTag   = regexp.MustCompile(`<[^>]*>`)
	htmlSpace = regexp.MustCompile(`[ \t]{2,}`)
	htmlBlank = regexp.MustCompile(`\n{3,}`)
)

// HTMLToText flattens a provider's HTML blurb into plain text suitable for
// an mp4 metadata atom: block-level tags become paragraph breaks, the
// remaining tags are dropped, and entities are decoded. Shared by every
// catalog client (Audnexus's summary and Audible's publisher_summary arrive
// as the same flavor of HTML).
func HTMLToText(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = htmlBreak.ReplaceAllString(s, "\n\n")
	s = htmlTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, " ", " ") // non-breaking space
	s = htmlSpace.ReplaceAllString(s, " ")
	s = htmlBlank.ReplaceAllString(s, "\n\n")
	// Tag removal leaves stray spaces at line edges (e.g. from "</p> <p>").
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
