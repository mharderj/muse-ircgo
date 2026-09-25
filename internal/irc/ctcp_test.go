package irc

import "testing"

func TestParseCTCP(t *testing.T) {
	tests := []struct {
		body string
		cmd  string
		args string
		ok   bool
	}{
		{"\x01VERSION\x01", "VERSION", "", true},
		{"\x01PING 123456\x01", "PING", "123456", true},
		{"\x01ACTION waves hello\x01", "ACTION", "waves hello", true},
		{"\x01action shrugs\x01", "ACTION", "shrugs", true}, // case-insensitive
		{"\x01\x01", "", "", true},                          // empty but delimited
		{"hello", "", "", false},
		{"\x01VERSION", "", "", false}, // missing closing delimiter
		{"VERSION\x01", "", "", false}, // missing opening delimiter
		{"", "", "", false},
		{"\x01", "", "", false},
	}
	for _, tt := range tests {
		cmd, args, ok := ParseCTCP(tt.body)
		if cmd != tt.cmd || args != tt.args || ok != tt.ok {
			t.Errorf("ParseCTCP(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tt.body, cmd, args, ok, tt.cmd, tt.args, tt.ok)
		}
	}
}
