package irc

import "testing"

func TestStripFormatting(t *testing.T) {
	cases := []struct{ in, want string }{
		// The Discord bridge's colorized relayed nick.
		{"\x0312<ladies-call-me-sledge-hammer🤪> ass", "<ladies-call-me-sledge-hammer🤪> ass"},
		{"\x0312<nick>\x03 hello", "<nick> hello"},
		{"\x0304,08colored\x03 plain", "colored plain"},
		{"\x034red\x03", "red"},
		{"\x02bold\x02 and \x1Funderline\x1F", "bold and underline"},
		{"\x1Ditalic\x1D \x16reverse\x16 \x0Freset", "italic reverse reset"},
		{"plain text", "plain text"},
		{"", ""},
		// \x03 with no digits is just a reset.
		{"a\x03b", "ab"},
		// A comma not followed by digits is literal text.
		{"\x0312, hello", ", hello"},
		// Incomplete trailing sequences don't eat text.
		{"end\x03", "end"},
	}
	for _, tc := range cases {
		if got := StripFormatting(tc.in); got != tc.want {
			t.Errorf("StripFormatting(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
