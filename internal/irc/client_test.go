package irc

import (
	"context"
	"net"
	"testing"
	"time"

	"ircgo/internal/config"
)

// emit must block until the UI receives the event instead of silently
// dropping it when the channel is full (e.g. a ZNC replay burst).
func TestEmitBlocksUntilReceived(t *testing.T) {
	events := make(chan Event) // unbuffered: no room, no receiver yet
	c := New(config.Server{Name: "test"}, events)
	c.ctx = context.Background()

	done := make(chan struct{})
	go func() {
		c.emit(Event{Kind: KindMessage})
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("emit returned with no receiver: event would have been dropped")
	case <-time.After(50 * time.Millisecond):
		// Still blocked: good.
	}

	select {
	case e := <-events:
		if e.Server != "test" {
			t.Fatalf("event server = %q, want test", e.Server)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("emit never delivered the event")
	}
	<-done
}

// emit must not hang shutdown: if the connection context is done while no
// receiver is waiting, emit gives up.
func TestEmitGivesUpOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan Event) // unbuffered, never received
	c := New(config.Server{Name: "test"}, events)
	c.ctx = ctx

	done := make(chan struct{})
	go func() {
		c.emit(Event{Kind: KindMessage})
		close(done)
	}()
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("emit blocked after context cancellation")
	}
}

// A 433 (nickname in use) must send a fallback NICK and still surface the
// server's message to the UI instead of hanging on "connecting…".
func TestNickCollisionFallback(t *testing.T) {
	events := make(chan Event, 8)
	clientEnd, serverEnd := net.Pipe()
	defer clientEnd.Close()
	defer serverEnd.Close()

	c := New(config.Server{Name: "test", Nick: "bob"}, events)
	c.ctx = context.Background()
	c.conn = wrap(clientEnd)

	read := make(chan string, 1)
	go func() {
		buf := make([]byte, 64)
		_ = serverEnd.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, _ := serverEnd.Read(buf)
		read <- string(buf[:n])
	}()

	c.handle(&Message{
		Command: "433",
		Params:  []string{"*", "bob", "Nickname is already in use"},
	})

	select {
	case got := <-read:
		if got != "NICK bob_\r\n" {
			t.Fatalf("sent %q, want %q", got, "NICK bob_\r\n")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no fallback NICK sent")
	}
	if c.nick != "bob_" {
		t.Fatalf("c.nick = %q, want bob_", c.nick)
	}
	// The 433 itself is still published so the UI can show it.
	select {
	case e := <-events:
		if e.Kind != KindMessage || e.Msg.Command != "433" {
			t.Fatalf("event = %+v, want the 433 message", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("433 was not published to the UI")
	}
}

// Run must keep retrying a dead server with backoff instead of returning
// after the first failed dial.
func TestReconnectRetries(t *testing.T) {
	oldInit, oldMax, oldDial := reconnectInitial, reconnectMax, dialTimeout
	reconnectInitial, reconnectMax, dialTimeout = 50*time.Millisecond, 100*time.Millisecond, 50*time.Millisecond
	defer func() { reconnectInitial, reconnectMax, dialTimeout = oldInit, oldMax, oldDial }()

	events := make(chan Event, 32)
	c := New(config.Server{Name: "test", Host: "127.0.0.1", Port: 1}, events) // nothing listens: refused fast

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx); close(done) }()

	// Expect at least: dial error, reconnecting notice, dial error…
	deadline := time.After(5 * time.Second)
	var kinds []EventKind
	for len(kinds) < 3 {
		select {
		case e := <-events:
			kinds = append(kinds, e.Kind)
		case <-deadline:
			t.Fatalf("only %d events before deadline: %v", len(kinds), kinds)
		}
	}
	if kinds[0] != KindError {
		t.Fatalf("first event = %v, want KindError (dial failure)", kinds[0])
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}
