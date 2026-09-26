package irc

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// mircColors maps the 16 mIRC color codes to terminal colors.
var mircColors = []string{
	"15", // 0  white
	"0",  // 1  black
	"4",  // 2  blue (navy)
	"2",  // 3  green
	"9",  // 4  red
	"1",  // 5  brown (maroon)
	"5",  // 6  purple
	"3",  // 7  orange
	"11", // 8  yellow
	"10", // 9  light green
	"6",  // 10 teal
	"14", // 11 light cyan
	"12", // 12 light blue
	"13", // 13 pink
	"8",  // 14 grey
	"7",  // 15 light grey
}

// FormatText renders mIRC formatting codes as styled text: bold (\x02),
// color (\x03 with optional FG[,BG] numbers), reset (\x0F), reverse (\x16),
// italic (\x1D), and underline (\x1F). Bridges (e.g. Discord) colorize
// relayed nicks like \x0312<nick>, which now shows up in color instead of
// leaking the color number into the visible text.
func FormatText(s string) string {
	return formatStyled(s, lipgloss.NewStyle())
}

// FormatStyled is FormatText with a base style applied to runs without
// explicit formatting (and merged underneath explicit formatting).
func FormatStyled(s string, base lipgloss.Style) string {
	return formatStyled(s, base)
}

func formatStyled(s string, base lipgloss.Style) string {
	var out, cur strings.Builder
	var bold, italic, underline, reverse bool
	fg, bg := -1, -1

	// flush renders the pending plain run with the current style.
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		st := base.Copy()
		if bold {
			st = st.Bold(true)
		}
		if italic {
			st = st.Italic(true)
		}
		if underline {
			st = st.Underline(true)
		}
		if reverse {
			st = st.Reverse(true)
		}
		if fg >= 0 {
			st = st.Foreground(lipgloss.Color(mircColors[fg]))
		}
		if bg >= 0 {
			st = st.Background(lipgloss.Color(mircColors[bg]))
		}
		out.WriteString(st.Render(cur.String()))
		cur.Reset()
	}

	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\x02':
			flush()
			bold = !bold
		case '\x03':
			n := colorLen(s, i+1)
			flush()
			if n == 0 {
				fg, bg = -1, -1 // bare \x03 resets the color
			} else {
				fg, bg = parseColorSpec(s[i+1 : i+1+n])
			}
			i += n
		case '\x0F':
			flush()
			bold, italic, underline, reverse = false, false, false, false
			fg, bg = -1, -1
		case '\x16':
			flush()
			reverse = !reverse
		case '\x1D':
			flush()
			italic = !italic
		case '\x1F':
			flush()
			underline = !underline
		default:
			cur.WriteByte(s[i])
		}
	}
	flush()
	return out.String()
}

// parseColorSpec parses "FG" or "FG,BG" after a \x03 introducer. Out-of-range
// numbers are ignored.
func parseColorSpec(spec string) (fg, bg int) {
	fg, bg = -1, -1
	parts := strings.SplitN(spec, ",", 2)
	fg = atoi2(parts[0])
	if len(parts) == 2 {
		bg = atoi2(parts[1])
	}
	if fg < 0 || fg > 15 {
		fg = -1
	}
	if bg < 0 || bg > 15 {
		bg = -1
	}
	return fg, bg
}

func atoi2(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return -1
		}
		n = n*10 + int(s[i]-'0')
	}
	return n
}

// colorLen returns how many bytes starting at s[i] form the color spec
// after a \x03 introducer: 1-2 foreground digits, then an optional
// ",BG" background when the comma is followed by digits.
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
