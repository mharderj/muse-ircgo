// Package link finds URLs in chat text and styles them as clickable links.
package link

import (
	"regexp"
	"strings"
)

var urlRe = regexp.MustCompile(`https?://[^\s<>"']+`)

// Span is one URL match: byte offsets of the raw match and the URL with
// trailing punctuation trimmed.
type Span struct {
	Start, End int
	URL        string
}

// FindSpans returns the URL matches in s.
func FindSpans(s string) []Span {
	var out []Span
	for _, idx := range urlRe.FindAllStringIndex(s, -1) {
		raw := s[idx[0]:idx[1]]
		out = append(out, Span{
			Start: idx[0],
			End:   idx[1],
			URL:   strings.TrimRight(raw, ".,;:!?)"),
		})
	}
	return out
}

// FindAll returns the cleaned URLs in s.
func FindAll(s string) []string {
	spans := FindSpans(s)
	out := make([]string, 0, len(spans))
	for _, sp := range spans {
		out = append(out, sp.URL)
	}
	return out
}

// Style wraps each URL in s with style(), leaving trailing punctuation
// outside the styled region.
func Style(s string, style func(string) string) string {
	spans := FindSpans(s)
	if len(spans) == 0 {
		return s
	}
	var b strings.Builder
	pos := 0
	for _, sp := range spans {
		b.WriteString(s[pos:sp.Start])
		b.WriteString(style(sp.URL))
		b.WriteString(s[sp.Start+len(sp.URL) : sp.End])
		pos = sp.End
	}
	b.WriteString(s[pos:])
	return b.String()
}
