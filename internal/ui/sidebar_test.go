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
	if !strings.Contains(view, "Messages") {
		t.Fatalf("sidebar has no Messages section:\n%s", view)
	}
	// Rows: 0 = server window, 1 = #a, 2 = rule, 3 = Messages header,
	// 4 = belial. The rule and header sit between channel and query rows.
	lines := strings.Split(view, "\n")
	chanRow, ruleRow, headRow, queryRow := -1, -1, -1, -1
	for i, l := range lines {
		switch {
		case strings.Contains(l, "#a"):
			chanRow = i
		case strings.Contains(l, "─"):
			ruleRow = i
		case strings.Contains(l, "Messages"):
			headRow = i
		case strings.Contains(l, "belial"):
			queryRow = i
		}
	}
	if !(chanRow >= 0 && chanRow < ruleRow && ruleRow < headRow && headRow < queryRow) {
		t.Fatalf("rows: channel=%d rule=%d header=%d query=%d, want channel < rule < header < query", chanRow, ruleRow, headRow, queryRow)
	}
}

func TestSidebarSectionRowsClickIsNoop(t *testing.T) {
	a := sortTestApp()
	a.refreshBuffers()
	// Rows: 0 = server window (doubles as the header row), 1 = #a,
	// 2 = rule, 3 = Messages header, 4 = belial.
	if _, ok := a.sidebarBufferAt(2, 2); ok {
		t.Fatal("click on rule row selected a buffer")
	}
	if _, ok := a.sidebarBufferAt(2, 3); ok {
		t.Fatal("click on Messages header row selected a buffer")
	}
	i, ok := a.sidebarBufferAt(2, 4)
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
