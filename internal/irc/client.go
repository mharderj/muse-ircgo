package irc

import (
	"context"
	"fmt"
	"sync"

	"ircgo/internal/config"
)

// EventKind classifies an Event.
type EventKind int

const (
	// KindMessage is a parsed IRC protocol message.
	KindMessage EventKind = iota
	// KindConnected fires after registration completes.
	KindConnected
	// KindDisconnected fires when the connection ends.
	KindDisconnected
	// KindError carries a human-readable failure in Text.
	KindError
)

// Event is handed from the connection goroutine to the UI.
type Event struct {
	Server string
	Kind   EventKind
	Msg    *Message // set for KindMessage
	Text   string   // set for KindError
}

// requestedCaps are negotiated when the server offers them. server-time and
// znc.in/server-time-iso matter most: they stamp ZNC backlog replays with
// their original time instead of "just now".
var requestedCaps = []string{
	"multi-prefix",
	"away-notify",
	"account-notify",
	"extended-join",
	"server-time",
	"znc.in/server-time-iso",
	"batch",
}

// Client manages one server connection and publishes events.
type Client struct {
	cfg    config.Server
	events chan<- Event

	mu         sync.Mutex
	conn       *Conn
	capOffered map[string]bool
}

// New returns a client that publishes events to the channel.
func New(cfg config.Server, events chan<- Event) *Client {
	return &Client{cfg: cfg, events: events, capOffered: map[string]bool{}}
}

func (c *Client) emit(e Event) {
	e.Server = c.cfg.Name
	select {
	case c.events <- e:
	default:
		// Never block the read loop on a slow UI.
	}
}

// Send writes a raw protocol line. Safe for concurrent use.
func (c *Client) Send(line string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("irc: not connected")
	}
	return c.conn.Send(line)
}

// Run connects, registers, and pumps messages until ctx is cancelled or the
// connection drops.
func (c *Client) Run(ctx context.Context) {
	conn, err := Dial(c.cfg.Name, c.cfg.Host, c.port(), c.cfg.TLS, c.cfg.InsecureSkipVerify)
	if err != nil {
		c.emit(Event{Kind: KindError, Text: fmt.Sprintf("dial %s: %v", c.cfg.Addr(), err)})
		return
	}
	debugf(c.cfg.Name, "registering as %s", c.cfg.Nick)
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	defer func() {
		conn.Close()
		c.mu.Lock()
		c.conn = nil
		c.mu.Unlock()
		c.emit(Event{Kind: KindDisconnected})
	}()

	if err := c.register(); err != nil {
		c.emit(Event{Kind: KindError, Text: fmt.Sprintf("register: %v", err)})
		return
	}
	c.emit(Event{Kind: KindConnected})

	for {
		select {
		case <-ctx.Done():
			_ = c.Send("QUIT :ircgo leaving")
			return
		default:
		}
		line, err := conn.ReadLine()
		if err != nil {
			c.emit(Event{Kind: KindError, Text: fmt.Sprintf("read: %v", err)})
			return
		}
		debugf(c.cfg.Name, "<- %s", line)
		msg, err := Parse(line)
		if err != nil {
			continue
		}
		c.handle(msg)
	}
}

func (c *Client) port() int {
	if c.cfg.Port != 0 {
		return c.cfg.Port
	}
	if c.cfg.TLS {
		return 6697
	}
	return 6667
}

// register performs the PASS/CAP/NICK/USER handshake.
func (c *Client) register() error {
	if c.cfg.HasPass() {
		debugf(c.cfg.Name, "-> PASS (redacted)")
		if err := c.Send("PASS " + c.cfg.Pass()); err != nil {
			return err
		}
	}
	for _, line := range []string{
		"CAP LS 302",
		"NICK " + c.cfg.Nick,
		fmt.Sprintf("USER %s 0 * :ircgo", c.cfg.Nick),
	} {
		debugf(c.cfg.Name, "-> %s", line)
		if err := c.Send(line); err != nil {
			return err
		}
	}
	return nil
}

// handle routes one parsed message: protocol chores stay here, everything
// else is published for the UI.
func (c *Client) handle(m *Message) {
	switch m.Command {
	case "PING":
		_ = c.Send("PONG :" + m.Trailing())
		return
	case "CAP":
		c.handleCap(m)
		return
	case "AUTHENTICATE":
		c.handleAuthenticate(m)
		return
	case "001": // welcome: registration complete
		debugf(c.cfg.Name, "registered, joining %d channel(s)", len(c.cfg.Channels))
		for _, ch := range c.cfg.Channels {
			_ = c.Send("JOIN " + ch)
		}
	case "903": // SASL success
		_ = c.Send("CAP END")
	case "904", "905": // SASL failure
		c.emit(Event{Kind: KindError, Text: "SASL authentication failed"})
		_ = c.Send("CAP END")
	}
	c.emit(Event{Kind: KindMessage, Msg: m})
}
