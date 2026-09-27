package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"ircgo/internal/config"
	"ircgo/internal/store"
)

// collapseApp builds an app with one channel buffer holding lines.
func collapseApp(cfg *config.Config, lines ...store.Line) *App {
	if cfg == nil {
		cfg = &config.Config{}
	}
	if cfg.UI.TimestampFormat == "" {
		cfg.UI.TimestampFormat = "15:04" // what config.Load defaults to
	}
	a := New(cfg, store.New(), nil, nil)
	for _, l := range lines {
		a.addLine("srv", "#a", l)
	}
	a.bufs = a.st.Buffers()
	return a
}

func joinLine(at time.Time, nick string) store.Line {
	return store.Line{At: at, Nick: nick, Text: "joined #a", Kind: store.KindJoin}
}

func TestJoinFloodCollapses(t *testing.T) {
	at := time.Date(2026, 9, 26, 17, 2, 0, 0, time.Local)
	var lines []store.Line
	for i := 0; i < 5; i++ {
		lines = append(lines, joinLine(at.Add(time.Duration(i)*time.Minute), "thadood"))
	}
	content := stripANSI(collapseApp(nil, lines...).chatContent())
	if n := strings.Count(content, "joined #a"); n != 1 {
		t.Fatalf("expected one collapsed row, got %d:\n%s", n, content)
	}
	if !strings.Contains(content, "thadood joined #a (×5)") {
		t.Fatalf("missing summary row:\n%s", content)
	}
	if !strings.Contains(content, "17:02–17:06") {
		t.Fatalf("missing time range:\n%s", content)
	}
}

func TestLoneJoinRendersNormally(t *testing.T) {
	at := time.Date(2026, 9, 26, 17, 2, 0, 0, time.Local)
	a := collapseApp(nil,
		store.Line{At: at, Nick: "bob", Text: "hello", Kind: store.KindChat},
		joinLine(at, "thadood"),
		store.Line{At: at, Nick: "bob", Text: "bye", Kind: store.KindChat},
	)
	content := stripANSI(a.chatContent())
	if !strings.Contains(content, "thadood joined #a\n") && !strings.Contains(content, "thadood joined #a") {
		t.Fatalf("lone join missing:\n%s", content)
	}
	if strings.Contains(content, "(×") {
		t.Fatalf("lone join should not collapse:\n%s", content)
	}
}

func TestMixedMembershipRunGroupsByEvent(t *testing.T) {
	at := time.Date(2026, 9, 26, 17, 2, 0, 0, time.Local)
	a := collapseApp(nil,
		joinLine(at, "bob"),
		joinLine(at.Add(time.Minute), "bob"),
		store.Line{At: at.Add(2 * time.Minute), Nick: "bob", Text: "left #a", Kind: store.KindPart},
	)
	content := stripANSI(a.chatContent())
	if !strings.Contains(content, "bob joined #a (×2)") {
		t.Fatalf("missing join summary:\n%s", content)
	}
	// The single part renders as an ordinary line, not a summary.
	if !strings.Contains(content, "bob left #a") || strings.Contains(content, "left #a (×") {
		t.Fatalf("single part should render plainly:\n%s", content)
	}
}

func TestCollapseDisabled(t *testing.T) {
	cfg := &config.Config{}
	off := false
	cfg.UI.CollapseJoins = &off
	at := time.Date(2026, 9, 26, 17, 2, 0, 0, time.Local)
	var lines []store.Line
	for i := 0; i < 3; i++ {
		lines = append(lines, joinLine(at.Add(time.Duration(i)*time.Minute), "thadood"))
	}
	content := stripANSI(collapseApp(cfg, lines...).chatContent())
	if n := strings.Count(content, "joined #a"); n != 3 {
		t.Fatalf("expected 3 separate rows, got %d:\n%s", n, content)
	}
}

func TestChatScrollPinning(t *testing.T) {
	a := New(&config.Config{}, store.New(), nil, nil)
	a.chat = viewport.New(60, 5)
	at := time.Date(2026, 9, 26, 17, 2, 0, 0, time.Local)
	for i := 0; i < 20; i++ {
		a.addLine("srv", "#a", store.Line{At: at, Nick: "bob", Text: fmt.Sprintf("msg %d", i), Kind: store.KindChat})
	}
	a.bufs = a.st.Buffers()
	a.renderChat()
	if !a.chatPinned {
		t.Fatal("chat should start pinned")
	}
	if !a.chat.AtBottom() {
		t.Fatal("chat should start at bottom")
	}
	// pgup scrolls up and unpins.
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	a = m.(*App)
	if a.chatPinned {
		t.Fatal("pgup should unpin the chat pane")
	}
	up := a.chat.YOffset
	// New lines while unpinned don't move the view.
	a.addLine("srv", "#a", store.Line{At: at, Nick: "bob", Text: "fresh", Kind: store.KindChat})
	a.renderChat()
	if a.chat.YOffset != up {
		t.Fatalf("unpinned render moved YOffset %d -> %d", up, a.chat.YOffset)
	}
	if a.chatPinned {
		t.Fatal("new lines must not re-pin")
	}
	// end re-pins to the bottom.
	m, _ = a.Update(tea.KeyMsg{Type: tea.KeyEnd})
	a = m.(*App)
	if !a.chatPinned || !a.chat.AtBottom() {
		t.Fatal("end should re-pin to the bottom")
	}
	// pgdn from unpinned reaching the bottom re-pins too.
	m, _ = a.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	a = m.(*App)
	for i := 0; i < 10 && !a.chatPinned; i++ {
		m, _ = a.Update(tea.KeyMsg{Type: tea.KeyPgDown})
		a = m.(*App)
	}
	if !a.chatPinned || !a.chat.AtBottom() {
		t.Fatal("pgdn to the bottom should re-pin")
	}
}

func TestWheelScrollsChatPane(t *testing.T) {
	a := New(&config.Config{}, store.New(), nil, nil)
	a.chat = viewport.New(60, 5)
	at := time.Date(2026, 9, 26, 17, 2, 0, 0, time.Local)
	for i := 0; i < 20; i++ {
		a.addLine("srv", "#a", store.Line{At: at, Nick: "bob", Text: fmt.Sprintf("msg %d", i), Kind: store.KindChat})
	}
	a.bufs = a.st.Buffers()
	a.renderChat()
	top := a.chat.YOffset

	wheel := func(up bool) *App {
		btn := tea.MouseButtonWheelDown
		if up {
			btn = tea.MouseButtonWheelUp
		}
		// Right of the (16-wide, unconfigured) sidebar, below the topic bar.
		m, _ := a.Update(tea.MouseMsg{X: 30, Y: 3, Action: tea.MouseActionPress, Button: btn})
		return m.(*App)
	}

	a = wheel(true)
	if a.chatPinned {
		t.Fatal("wheel up should unpin the chat pane")
	}
	if a.chat.YOffset >= top {
		t.Fatalf("wheel up should scroll up (YOffset %d -> %d)", top, a.chat.YOffset)
	}
	// Wheel over the sidebar is swallowed, not scrolled.
	m, _ := a.Update(tea.MouseMsg{X: 2, Y: 3, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	a = m.(*App)
	// Rolling down to the bottom re-pins.
	for i := 0; i < 10 && !a.chatPinned; i++ {
		a = wheel(false)
	}
	if !a.chatPinned || !a.chat.AtBottom() {
		t.Fatal("wheel down to the bottom should re-pin")
	}
}
