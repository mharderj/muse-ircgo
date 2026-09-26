package ui

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// visibleWidth returns the terminal cell width of s, ignoring ANSI SGR
// escapes. Wide runes (CJK, emoji) count as two cells.
func visibleWidth(s string) int {
	w := 0
	for _, r := range stripANSI(s) {
		w += runewidth.RuneWidth(r)
	}
	return w
}

// isSGRReset reports whether an SGR escape sequence resets attributes.
func isSGRReset(code string) bool {
	inner := strings.TrimSuffix(strings.TrimPrefix(code, "\x1b["), "m")
	for _, p := range strings.Split(inner, ";") {
		if p == "0" || p == "" {
			return true
		}
	}
	return false
}

// wrapANSI word-wraps s to width visible cells, breaking only at spaces.
// Continuation lines start with indent. ANSI SGR state is carried across
// breaks so colors survive wrapping; a word longer than width is left to
// overflow rather than split, which keeps long URLs clickable.
func wrapANSI(s string, width int, indent string) []string {
	if width < 1 {
		width = 1
	}
	const reset = "\x1b[0m"
	var lines []string
	var cur strings.Builder
	curW := 0
	var active []string // SGR codes currently in effect

	breakLine := func() {
		if len(active) > 0 {
			cur.WriteString(reset)
		}
		lines = append(lines, cur.String())
		cur.Reset()
		cur.WriteString(indent)
		for _, c := range active {
			cur.WriteString(c)
		}
		curW = visibleWidth(indent)
	}

	for i, w := range strings.Split(s, " ") {
		ww := visibleWidth(w)
		if i > 0 && curW > 0 {
			if curW+1+ww > width {
				breakLine()
			} else {
				cur.WriteString(" ")
				curW++
			}
		}
		for _, c := range sgrRe.FindAllString(w, -1) {
			if isSGRReset(c) {
				active = nil
			} else {
				active = append(active, c)
			}
		}
		cur.WriteString(w)
		curW += ww
	}
	lines = append(lines, cur.String())
	return lines
}
