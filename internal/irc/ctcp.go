package irc

import "strings"

// ParseCTCP extracts the command and arguments from a CTCP message body,
// e.g. "\x01VERSION\x01" or "\x01PING 123456\x01". ok is false when the
// body is not CTCP-delimited.
func ParseCTCP(body string) (cmd, args string, ok bool) {
	if len(body) < 2 || body[0] != '\x01' || body[len(body)-1] != '\x01' {
		return "", "", false
	}
	inner := body[1 : len(body)-1]
	if i := strings.IndexByte(inner, ' '); i >= 0 {
		return strings.ToUpper(inner[:i]), inner[i+1:], true
	}
	return strings.ToUpper(inner), "", true
}
