package irc

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"ircgo/internal/config"
)

// reconnectInitial/reconnectMax bound the backoff between connection
// attempts. Vars so tests can shrink them.
var (
	reconnectInitial = 2 * time.Second
	reconnectMax     = 5 * time.Minute
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
	ctx        context.Context // connection lifetime, set by Run

	// nick is the nick we're currently trying to register as; a 433
	// (nickname in use) appends "_" and retries. Reset to the configured
	// nick at the start of every connection attempt.
	nick string
	// registered is set when 001 arrives; a healthy session resets the
	// reconnect backoff. Atomic because Connected is read from the UI
	// goroutine while the Run goroutine writes it.
	registered atomic.Bool
	// reconnectCh wakes Run from its backoff sleep for a manual
	// /reconnect. Buffered (size 1) so ReconnectNow never blocks; one
	// pending retry is all that's needed.
	reconnectCh chan struct{}
}

// New returns a client that publishes events to the channel.
func New(cfg config.Server, events chan<- Event) *Client {
	return &Client{cfg: cfg, events: events, capOffered: map[string]bool{}, nick: cfg.Nick, reconnectCh: make(chan struct{}, 1)}
}

func (c *Client) emit(e Event) {
	e.Server = c.cfg.Name
	ctx := c.ctx
	if ctx == nil {
		// Not running under Run (shouldn't happen): never block.
		select {
		case c.events <- e:
		default:
		}
		return
	}
	// Block until the UI receives the event rather than silently dropping
	// backlog (e.g. a ZNC replay burst bigger than the channel buffer).
	// Give up if the connection is torn down so a wedged UI can't hang
	// shutdown.
	select {
	case c.events <- e:
	case <-ctx.Done():
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

// ReconnectNow drops the current connection (if any) and prompts Run to
// retry immediately instead of waiting out the backoff. Safe for
// concurrent use (e.g. the UI's /reconnect command).
func (c *Client) ReconnectNow() {
	c.mu.Lock()
	if c.conn != nil {
		c.conn.Close()
	}
	c.mu.Unlock()
	select {
	case c.reconnectCh <- struct{}{}:
	default:
	}
}

// Connected reports whether the client currently holds a registered
// session. The UI uses it to avoid silently dropping outgoing messages
// typed while the connection is down.
func (c *Client) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn != nil && c.registered.Load()
}

// withJitter spreads a backoff delay between 75% and 125% of d so a fleet
// of clients doesn't retry in lockstep after a netsplit.
func withJitter(d time.Duration) time.Duration {
	q := int64(d) / 4
	if q <= 0 {
		return d
	}
	return time.Duration(3*q + rand.Int63n(2*q+1))
}

// Run connects, registers, and pumps messages, reconnecting with
// exponential backoff until ctx is cancelled. A dropped connection used to
// leave the UI looking alive while dead; now the UI sees "disconnected",
// a retry notice, and "connected" again when the session recovers.
func (c *Client) Run(ctx context.Context) {
	c.ctx = ctx
	backoff := reconnectInitial
	for {
		c.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		if c.registered.Load() {
			// The last session was healthy: start the backoff over so a
			// flaky network doesn't wedge us at the maximum delay.
			backoff = reconnectInitial
		}
		// A manual /reconnect skips the wait and retries immediately.
		select {
		case <-c.reconnectCh:
			backoff = reconnectInitial
			continue
		default:
		}
		sleep := withJitter(backoff)
		c.emit(Event{Kind: KindError, Text: fmt.Sprintf("reconnecting in %v", sleep.Round(time.Second))})
		select {
		case <-ctx.Done():
			return
		case <-c.reconnectCh:
			backoff = reconnectInitial
		case <-time.After(sleep):
			backoff *= 2
			if backoff > reconnectMax {
				backoff = reconnectMax
			}
		}
	}
}

// runOnce makes a single connection attempt: dial, register, and pump
// messages until the connection drops or ctx is cancelled.
func (c *Client) runOnce(ctx context.Context) {
	c.nick = c.cfg.Nick // fresh attempt at the preferred nick
	c.registered.Store(false)
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
		"NICK " + c.nick,
		fmt.Sprintf("USER %s 0 * :ircgo", c.nick),
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
		c.registered.Store(true)
		debugf(c.cfg.Name, "registered, joining %d channel(s)", len(c.cfg.Channels))
		for _, ch := range c.cfg.Channels {
			_ = c.Send("JOIN " + ch)
		}
	case "433": // nickname in use: fall back and keep registering
		c.nick += "_"
		_ = c.Send("NICK " + c.nick)
		// Deliberately falls through to emit: the UI shows the server's
		// "Nickname is already in use" line and learns the real nick
		// from 001's target parameter.
	case "903": // SASL success
		_ = c.Send("CAP END")
	case "904", "905": // SASL failure
		c.emit(Event{Kind: KindError, Text: "SASL authentication failed"})
		_ = c.Send("CAP END")
	}
	c.emit(Event{Kind: KindMessage, Msg: m})
}
