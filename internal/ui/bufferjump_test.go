package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"ircgo/internal/config"
	"ircgo/internal/store"
)

// bufferJumpApp builds an app with three buffers, laid out and rendered.
func bufferJumpApp(t *testing.T) *App {
	t.Helper()
	a := New(&config.Config{}, store.New(), nil, nil)
	for _, name := range []string{"#a", "#b", "#c"} {
		a.addLine("srv", name, store.Line{
			At:   time.Date(2026, 9, 26, 14, 18, 0, 0, time.UTC),
			Nick: "bob",
			Text: "hi " + name,
			Kind: store.KindChat,
		})
	}
	a.bufs = a.st.Buffers()
	a.focus = 0
	a.width, a.height = 100, 30
	a.ready = true
	a.resize()
	return a
}

func altDigit(d rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{d}, Alt: true}
}

func TestAltDigitJumpsToBuffer(t *testing.T) {
	a := bufferJumpApp(t)
	m, _ := a.Update(altDigit('3'))
	a = m.(*App)
	if a.focus != 2 {
		t.Fatalf("focus = %d, want 2 after alt+3", a.focus)
	}
	m, _ = a.Update(altDigit('1'))
	a = m.(*App)
	if a.focus != 0 {
		t.Fatalf("focus = %d, want 0 after alt+1", a.focus)
	}
}

func TestAltDigitOutOfRangeIgnored(t *testing.T) {
	a := bufferJumpApp(t)
	m, _ := a.Update(altDigit('9'))
	a = m.(*App)
	if a.focus != 0 {
		t.Fatalf("focus = %d, want 0 (unchanged) after alt+9", a.focus)
	}
}

func TestAltDigitFallsThroughToInput(t *testing.T) {
	a := bufferJumpApp(t)
	// alt+0 is not a buffer jump; the input line should keep working.
	m, _ := a.Update(altDigit('0'))
	a = m.(*App)
	if a.focus != 0 {
		t.Fatalf("focus = %d, want 0 after alt+0", a.focus)
	}
}
