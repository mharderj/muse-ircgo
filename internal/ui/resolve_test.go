package ui

import (
	"testing"

	"ircgo/internal/irc"
	"ircgo/internal/store"
)

// Regression test: messaging "belial" then getting a reply from "Belial"
// must land in the one existing buffer, not open a second.
func TestDMBufferUnifiesCase(t *testing.T) {
	a := kickTestApp()
	a.st.Get("srv", "belial") // outgoing /msg created the lowercase buffer

	if got := a.st.Resolve("srv", "Belial"); got != "belial" {
		t.Fatalf("Resolve = %q, want belial", got)
	}
	if got := a.st.Resolve("srv", "BELIAL"); got != "belial" {
		t.Fatalf("Resolve = %q, want belial", got)
	}
	// Unknown names pass through unchanged.
	if got := a.st.Resolve("srv", "someone"); got != "someone" {
		t.Fatalf("Resolve = %q, want someone", got)
	}
	// A nick that merely resembles the server name must not capture the
	// server window.
	a.st.Get("srv", "srv")
	if got := a.st.Resolve("srv", "SRV"); got != "SRV" {
		t.Fatalf("Resolve = %q, want SRV (server window protected)", got)
	}
	// Exact server name still resolves to the server window.
	if got := a.st.Resolve("srv", "srv"); got != "srv" {
		t.Fatalf("Resolve = %q, want srv", got)
	}
}

func TestIncomingDMReusesExistingBuffer(t *testing.T) {
	a := kickTestApp()
	a.st.Get("srv", "belial")

	a.handleMessage("srv", &irc.Message{
		Prefix: "Belial!u@h", Command: "PRIVMSG", Params: []string{"me", "hey"},
	})

	if a.st.Has("srv", "Belial") {
		t.Fatal("second buffer Belial was created")
	}
	lines := a.st.Get("srv", "belial").Lines()
	if len(lines) != 1 || lines[0].Text != "hey" || lines[0].Nick != "Belial" {
		t.Fatalf("lines = %+v", lines)
	}
}

func TestAddLineResolvesChannelCase(t *testing.T) {
	a := kickTestApp()
	a.addLine("srv", "#TheZone", store.Line{Text: "first"})
	a.addLine("srv", "#thezone", store.Line{Text: "second"})

	if a.st.Has("srv", "#thezone") {
		t.Fatal("second channel buffer #thezone was created")
	}
	lines := a.st.Get("srv", "#TheZone").Lines()
	if len(lines) != 2 || lines[1].Text != "second" {
		t.Fatalf("lines = %+v", lines)
	}
}
