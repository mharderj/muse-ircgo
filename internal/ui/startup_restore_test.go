package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"ircgo/internal/irc"
	"ircgo/internal/store"
)

func bufNames(a *App) []string {
	var out []string
	for _, b := range a.bufs {
		out = append(out, b.Name)
	}
	return out
}

// TestRestartFocusesChannel simulates a full restart where the
// previous session ended on a channel: DM buffers are restored from logs
// on first layout, the server window appears on connect, and the channel
// buffer appears on JOIN. The pending last-buffer restore must win over
// the DM buffers that exist first.
func TestRestartFocusesChannel(t *testing.T) {
	dir := t.TempDir()
	// Seed DM logs so startup recreates them (like Dennis/belial).
	seed := newRestoreApp(t, dir)
	seed.addLine("srv", "dave", store.Line{Nick: "dave", Text: "hey", Kind: store.KindChat})

	b := newRestoreApp(t, dir)
	b.cfg.LastBuffer = []string{"srv", "#c"}

	if _, _ = b.Update(tea.WindowSizeMsg{Width: 120, Height: 40}); true {
	}
	if b.pendingFocus.name != "#c" {
		t.Fatalf("pendingFocus = %+v, want #c after first layout", b.pendingFocus)
	}

	// Connect: the server window appears; the channel isn't there yet.
	b.handleEvent(irc.Event{Kind: irc.KindConnected, Server: "srv"})
	b.refreshBuffers()
	if b.pendingFocus.name != "#c" {
		t.Fatalf("pendingFocus lost on connect: %+v", b.pendingFocus)
	}

	// JOIN #c (as ZNC sends on attach): the pending restore must fire.
	b.handleMessage("srv", &irc.Message{Prefix: "me!u@h", Command: "JOIN", Params: []string{"#c"}})
	b.refreshBuffers()
	if b.pendingFocus.server != "" {
		t.Fatalf("pendingFocus not consumed after JOIN: %+v", b.pendingFocus)
	}
	if got := b.bufs[b.focus].Name; got != "#c" {
		t.Fatalf("focused = %q (bufs %v), want #c", got, bufNames(b))
	}
}
