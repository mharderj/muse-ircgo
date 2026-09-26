package irc

import (
	"context"
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
