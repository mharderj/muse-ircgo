package irc

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func init() {
	// Tests run without a TTY, where lipgloss would otherwise detect the
	// Ascii profile and strip all color output.
	lipgloss.SetColorProfile(termenv.TrueColor)
}

func TestFormatText(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		plain  string // expected text with all codes/escapes removed
		styled bool   // expect ANSI escapes in the output
	}{
		{"plain", "plain text", "plain text", false},
		{"empty", "", "", false},
		{
			"bridge nick",
			"\x0312<ladies-call-me-sledge-hammer🤪> ass",
			"<ladies-call-me-sledge-hammer🤪> ass", true,
		},
		{"color reset", "\x0312<nick>\x03 hello", "<nick> hello", true},
		{"color bg", "\x0304,08colored\x03 plain", "colored plain", true},
		{"bold", "\x02bold\x02 plain", "bold plain", true},
		{"italic underline", "\x1Ditalic\x1D \x1Funder\x1F", "italic under", true},
		{"reset", "\x02\x0312both\x0F plain", "both plain", true},
		{"bare color reset", "a\x03b", "ab", false},
		// ", " is literal text; " hello" still takes fg color 12.
		{"comma not bg", "\x0312, hello", ", hello", true},
		{"out of range color", "\x0399xx", "xx", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatText(tc.in)
			// No raw mIRC control bytes may survive (\x1b is fine: it's
			// part of the ANSI escapes we emit).
			for i := 0; i < len(got); i++ {
				if got[i] < 0x20 && got[i] != '\n' && got[i] != 0x1b {
					t.Fatalf("raw control byte 0x%02x in %q", got[i], got)
				}
			}
			if stripANSI(got) != tc.plain {
				t.Fatalf("plain text = %q, want %q (full %q)", stripANSI(got), tc.plain, got)
			}
			if hasANSI(got) != tc.styled {
				t.Fatalf("hasANSI = %v, want %v (output %q)", hasANSI(got), tc.styled, got)
			}
		})
	}
}

// stripANSI removes "\x1b[...m" sequences for test comparison.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func hasANSI(s string) bool { return strings.Contains(s, "\x1b[") }
