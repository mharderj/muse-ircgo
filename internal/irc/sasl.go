package irc

import (
	"encoding/base64"
)

// handleAuthenticate answers the server's AUTHENTICATE challenge with a
// SASL PLAIN blob. For ZNC the auth identity is "user/network".
func (c *Client) handleAuthenticate(m *Message) {
	if m.Trailing() != "+" {
		return
	}
	user := c.cfg.Username
	if user == "" {
		user = c.cfg.Nick
	}
	if c.cfg.Username != "" && c.cfg.Network != "" {
		user = c.cfg.Username + "/" + c.cfg.Network
	}
	raw := "\x00" + user + "\x00" + c.cfg.Password
	b64 := base64.StdEncoding.EncodeToString([]byte(raw))
	for _, chunk := range saslChunks(b64) {
		_ = c.Send("AUTHENTICATE " + chunk)
	}
	debugf(c.cfg.Name, "-> AUTHENTICATE (redacted blob)")
}

// saslChunks splits a base64 blob into 400-byte AUTHENTICATE lines,
// terminating with "+" when the blob is an exact multiple of 400.
func saslChunks(b64 string) []string {
	var out []string
	for len(b64) > 400 {
		out = append(out, b64[:400])
		b64 = b64[400:]
	}
	out = append(out, b64)
	if len(out[len(out)-1]) == 400 {
		out = append(out, "+")
	}
	return out
}
