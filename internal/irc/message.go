// Package irc implements a small IRC client: line parsing, connection,
// capability negotiation and SASL. Protocol events are published on a
// channel for the UI layer to consume.
package irc

import (
	"fmt"
	"strings"
	"time"
)

// Message is a parsed IRC protocol message, including IRCv3 message tags.
type Message struct {
	Tags    map[string]string
	Prefix  string
	Command string
	Params  []string
}

// Nick returns the nickname portion of the prefix, if any.
func (m *Message) Nick() string {
	if m.Prefix == "" {
		return ""
	}
	if i := strings.IndexAny(m.Prefix, "!@"); i >= 0 {
		return m.Prefix[:i]
	}
	return m.Prefix
}

// Trailing returns the last parameter (the free-form text), if any.
func (m *Message) Trailing() string {
	if len(m.Params) == 0 {
		return ""
	}
	return m.Params[len(m.Params)-1]
}

// Time returns the instant from the server-time tag, or the zero time when
// the server didn't tag the message. Bouncers like ZNC tag replayed backlog
// with the original timestamp via server-time / znc.in/server-time-iso.
func (m *Message) Time() time.Time {
	raw, ok := m.Tags["time"]
	if !ok {
		return time.Time{}
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t
		}
	}
	return time.Time{}
}

// Parse parses a single raw IRC line into a Message.
func Parse(line string) (*Message, error) {
	m := &Message{Tags: map[string]string{}}
	rest := line

	// IRCv3 tags: "@tag1=value1;tag2=value2 :prefix COMMAND ..."
	if strings.HasPrefix(rest, "@") {
		i := strings.IndexByte(rest, ' ')
		if i < 0 {
			return nil, fmt.Errorf("irc: malformed tags in %q", line)
		}
		for _, kv := range strings.Split(rest[1:i], ";") {
			k, v, _ := strings.Cut(kv, "=")
			m.Tags[k] = unescapeTag(v)
		}
		rest = strings.TrimLeft(rest[i+1:], " ")
	}

	// Prefix: ":nick!user@host COMMAND ..."
	if strings.HasPrefix(rest, ":") {
		i := strings.IndexByte(rest, ' ')
		if i < 0 {
			return nil, fmt.Errorf("irc: malformed prefix in %q", line)
		}
		m.Prefix = rest[1:i]
		rest = strings.TrimLeft(rest[i+1:], " ")
	}

	i := strings.IndexByte(rest, ' ')
	if i < 0 {
		m.Command = strings.ToUpper(rest)
	} else {
		m.Command = strings.ToUpper(rest[:i])
		rest = strings.TrimLeft(rest[i+1:], " ")
		for len(rest) > 0 {
			if rest[0] == ':' {
				m.Params = append(m.Params, rest[1:])
				break
			}
			j := strings.IndexByte(rest, ' ')
			if j < 0 {
				m.Params = append(m.Params, rest)
				break
			}
			m.Params = append(m.Params, rest[:j])
			rest = strings.TrimLeft(rest[j+1:], " ")
		}
	}
	if m.Command == "" {
		return nil, fmt.Errorf("irc: empty command in %q", line)
	}
	return m, nil
}

var tagUnescaper = strings.NewReplacer(
	`\:`, ";",
	`\s`, " ",
	`\\`, "\\",
	`\r`, "\r",
	`\n`, "\n",
)

func unescapeTag(v string) string { return tagUnescaper.Replace(v) }
