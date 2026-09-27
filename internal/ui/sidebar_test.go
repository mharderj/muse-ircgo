package ui

import (
	"strings"
	"testing"
)

func sortTestApp() *App {
	a := kickTestApp()
	// Scrambled creation order: query, channel, server window.
	a.st.Get("srv", "belial")
	a.st.Get("srv", "#a")
	a.st.Get("srv", "srv")
	return a
}

func TestRefreshBuffersSortsChannelsBeforeQueries(t *testing.T) {
	a := sortTestApp()
	a.refreshBuffers()
	var names []string
	for _, b := range a.bufs {
		names = append(names, b.Name)
	}
	want := []string{"srv", "#a", "belial"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("buffer order = %v, want %v", names, want)
	}
}

func TestRefreshBuffersKeepsFocusOnSameBuffer(t *testing.T) {
	a := sortTestApp()
	a.refreshBuffers()
	// Focus the query buffer (now last).
	a.focus = 2
	if a.bufs[a.focus].Name != "belial" {
		t.Fatalf("focus = %q, want belial", a.bufs[a.focus].Name)
	}
	// A new channel slots in above it; focus must follow belial, not the
	// index.
	a.st.Get("srv", "#b")
	a.refreshBuffers()
	if a.bufs[a.focus].Name != "belial" {
		t.Fatalf("focus = %q, want belial", a.bufs[a.focus].Name)
	}
}

func TestSidebarRendersDividerBetweenChannelsAndQueries(t *testing.T) {
	a := sortTestApp()
	a.refreshBuffers()
	a.sidebar.Width = 30
	a.sidebar.Height = 20
	a.renderSidebar()
	view := a.sidebar.View()
	if !strings.Contains(view, "-- Messages --") {
		t.Fatalf("sidebar has no Messages divider:\n%s", view)
	}
	// The divider sits between the channel row and the query row.
	lines := strings.Split(view, "\n")
	chanRow, divRow, queryRow := -1, -1, -1
	for i, l := range lines {
		switch {
		case strings.Contains(l, "#a"):
			chanRow = i
		case strings.Contains(l, "-- Messages --"):
			divRow = i
		case strings.Contains(l, "belial"):
			queryRow = i
		}
	}
	if !(chanRow >= 0 && chanRow < divRow && divRow < queryRow) {
		t.Fatalf("rows: channel=%d divider=%d query=%d, want channel < divider < query", chanRow, divRow, queryRow)
	}
}

func TestSidebarDividerClickIsNoop(t *testing.T) {
	a := sortTestApp()
	a.refreshBuffers()
	// Rows: 0 = server window (doubles as the header row), 1 = #a,
	// 2 = divider, 3 = belial.
	if _, ok := a.sidebarBufferAt(2, 2); ok {
		t.Fatal("click on divider row selected a buffer")
	}
	i, ok := a.sidebarBufferAt(2, 3)
	if !ok {
		t.Fatal("click on belial row selected nothing")
	}
	if a.bufs[i].Name != "belial" {
		t.Fatalf("click selected %q, want belial", a.bufs[i].Name)
	}
	// The server window's header row selects the server buffer.
	i, ok = a.sidebarBufferAt(2, 0)
	if !ok || a.bufs[i].Name != "srv" {
		t.Fatalf("click on server row -> (%d, %v), want srv", i, ok)
	}
}
