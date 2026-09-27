package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"ircgo/internal/config"
	"ircgo/internal/store"
)

// Two servers, two buffers each. Sidebar rows:
//
//	0: srv1 (idx 0)   1: #a (idx 1)
//	2: separator      3: srv2 (idx 2)   4: #b (idx 3)
func testClickApp() *App {
	a := New(&config.Config{}, store.New(), nil, nil)
	a.addLine("srv1", "srv1", store.Line{Text: "hi"})
	a.addLine("srv1", "#a", store.Line{Text: "hi"})
	a.addLine("srv2", "srv2", store.Line{Text: "hi"})
	a.addLine("srv2", "#b", store.Line{Text: "hi"})
	a.bufs = a.st.Buffers()
	return a
}

func click(a *App, x, y int) *App {
	m, _ := a.Update(tea.MouseMsg{
		X: x, Y: y,
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
	})
	return m.(*App)
}

func TestSidebarClickSwitchesBuffer(t *testing.T) {
	a := testClickApp()
	a = click(a, 2, 4) // #b on srv2
	if a.focus != 3 {
		t.Fatalf("focus = %d, want 3", a.focus)
	}
	a = click(a, 2, 1) // #a on srv1
	if a.focus != 1 {
		t.Fatalf("focus = %d, want 1", a.focus)
	}
	// The server window is its section's header row: clicking it shows
	// the server buffer.
	a = click(a, 2, 3) // srv2 header row
	if a.focus != 2 {
		t.Fatalf("focus = %d, want 2 (srv2 server window)", a.focus)
	}
}

func TestSidebarClickIgnoresNonBuffers(t *testing.T) {
	a := testClickApp()
	for _, xy := range [][2]int{
		{2, 2},  // separator between servers
		{2, 20}, // blank space below the list
		{40, 4}, // chat area, outside the sidebar
	} {
		a = click(a, xy[0], xy[1])
		if a.focus != 0 {
			t.Fatalf("click at %v moved focus to %d", xy, a.focus)
		}
	}
}
