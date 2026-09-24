package irc

import (
	"strings"
)

// wantedCaps returns the capabilities this client asks for.
func (c *Client) wantedCaps() []string {
	caps := append([]string{}, requestedCaps...)
	if c.cfg.SASL {
		caps = append(caps, "sasl")
	}
	return caps
}

// handleCap processes a CAP subcommand during registration.
func (c *Client) handleCap(m *Message) {
	if len(m.Params) < 2 {
		return
	}
	sub := strings.ToUpper(m.Params[1])

	switch sub {
	case "LS":
		// Multi-line LS marks every line but the last with "*".
		continued := len(m.Params) > 3 && m.Params[2] == "*"
		list := strings.Join(m.Params[2:], " ")
		if continued {
			list = strings.Join(m.Params[3:], " ")
		}
		for _, cap := range strings.Fields(list) {
			name := strings.TrimPrefix(cap, "-")
			if i := strings.IndexByte(name, '='); i >= 0 {
				name = name[:i]
			}
			c.capOffered[strings.ToLower(name)] = true
		}
		if continued {
			return
		}
		var want []string
		for _, w := range c.wantedCaps() {
			if c.capOffered[w] {
				want = append(want, w)
			}
		}
		if len(want) == 0 {
			_ = c.Send("CAP END")
			return
		}
		_ = c.Send("CAP REQ :" + strings.Join(want, " "))
	case "ACK":
		args := ""
		if len(m.Params) > 2 {
			args = strings.Join(m.Params[2:], " ")
		}
		sasl := false
		for _, a := range strings.Fields(args) {
			if strings.EqualFold(a, "sasl") {
				sasl = true
			}
		}
		if sasl && c.cfg.SASL {
			_ = c.Send("AUTHENTICATE PLAIN")
			return
		}
		_ = c.Send("CAP END")
	case "NAK":
		_ = c.Send("CAP END")
	}
}
