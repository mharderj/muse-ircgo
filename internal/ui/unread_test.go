package ui

import (
	"testing"

	"ircgo/internal/irc"
)

func unreadTestApp() *App {
	a := kickTestApp()
	a.st.Get("srv", "#a")
	a.st.Get("srv", "#b")
	a.bufs = a.st.Buffers() // [#a, #b], focus starts at #a
	return a
}

func TestUnreadBumpsOnBackgroundMessage(t *testing.T) {
	a := unreadTestApp()
	a.handleMessage("srv", &irc.Message{
		Prefix: "bob!u@h", Command: "PRIVMSG", Params: []string{"#b", "hey"},
	})
	if got := a.st.Get("srv", "#b").Unread; got != 1 {
		t.Fatalf("#b unread = %d, want 1", got)
	}
	if got := a.st.Get("srv", "#a").Unread; got != 0 {
		t.Fatalf("#a unread = %d, want 0", got)
	}
}

func TestUnreadNotBumpedWhenFocused(t *testing.T) {
	a := unreadTestApp()
	a.focus = 1 // looking at #b
	a.handleMessage("srv", &irc.Message{
		Prefix: "bob!u@h", Command: "PRIVMSG", Params: []string{"#b", "hey"},
	})
	if got := a.st.Get("srv", "#b").Unread; got != 0 {
		t.Fatalf("#b unread = %d, want 0", got)
	}
}

func TestUnreadNotBumpedForOwnMessages(t *testing.T) {
	a := unreadTestApp()
	// Our own nick ("me" in test config) never bumps, e.g. ZNC replaying
	// messages we sent from another client.
	a.handleMessage("srv", &irc.Message{
		Prefix: "me!u@h", Command: "PRIVMSG", Params: []string{"#b", "hey"},
	})
	if got := a.st.Get("srv", "#b").Unread; got != 0 {
		t.Fatalf("#b unread = %d, want 0", got)
	}
}

func TestUnreadBumpsOnBackgroundDM(t *testing.T) {
	a := unreadTestApp()
	a.st.Get("srv", "belial")
	a.bufs = a.st.Buffers()

	a.handleMessage("srv", &irc.Message{
		Prefix: "Belial!u@h", Command: "PRIVMSG", Params: []string{"me", "yo"},
	})
	if got := a.st.Get("srv", "belial").Unread; got != 1 {
		t.Fatalf("belial unread = %d, want 1", got)
	}
}

func TestFocusClearsUnread(t *testing.T) {
	a := unreadTestApp()
	a.st.Get("srv", "#b").Unread = 3
	a.focusBuffer(1)
	if a.focus != 1 {
		t.Fatalf("focus = %d, want 1", a.focus)
	}
	if got := a.st.Get("srv", "#b").Unread; got != 0 {
		t.Fatalf("#b unread = %d, want 0", got)
	}
	// Out of range is a no-op, not a panic.
	a.focusBuffer(99)
	if a.focus != 1 {
		t.Fatalf("focus = %d, want 1", a.focus)
	}
}
