package irc

import "strings"

// StripFormatting removes mIRC formatting codes from text: bold (\x02),
// color (\x03 with optional FG[,BG] numbers), reset (\x0F), reverse (\x16),
// italic (\x1D), and underline (\x1F). Bridges (e.g. Discord) often colorize
// relayed nicks; without stripping, the color numbers leak into the visible
// text (e.g. "12<nick>").
func StripFormatting(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\x02', '\x0F', '\x16', '\x1D', '\x1F':
			// Formatting toggle with no parameters: drop it.
		case '\x03':
			// Color: optional 1-2 digit foreground, then an optional
			// ",BG" background when the comma is followed by digits.
			i += colorLen(s, i+1)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// colorLen returns how many bytes starting at s[i] form the color spec
// after a \x03 introducer.
func colorLen(s string, i int) int {
	n := 0
	for k := 0; k < 2 && i+n < len(s) && isDigit(s[i+n]); k++ {
		n++
	}
	if n > 0 && i+n < len(s) && s[i+n] == ',' &&
		i+n+1 < len(s) && isDigit(s[i+n+1]) {
		n++ // the comma
		for k := 0; k < 2 && i+n < len(s) && isDigit(s[i+n]); k++ {
			n++
		}
	}
	return n
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
