package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"ircgo/internal/store"
)

// archiveTestApp builds a sidebar with a server window, one channel and two
// PMs: bufs = [srv, #c, amy, bob].
func archiveTestApp(t *testing.T) *App {
	t.Helper()
	a := kickTestApp()
	a.width, a.height = 100, 30
	a.resize()
	a.addLine("srv", "srv", store.Line{Text: "hi"})
	a.addLine("srv", "#c", store.Line{Text: "hi"})
	a.addLine("srv", "amy", store.Line{Text: "hi"})
	a.addLine("srv", "bob", store.Line{Text: "hi"})
	a.refreshBuffers()
	a.renderSidebar()
	return a
}

func sidebarText(a *App) string {
	a.renderSidebar()
	return sgrRe.ReplaceAllString(a.sidebar.View(), "")
}

func bufIndex(a *App, name string) int {
	for i, b := range a.bufs {
		if b.Name == name {
			return i
		}
	}
	return -1
}

func TestMessagesDividerLabeled(t *testing.T) {
	a := archiveTestApp(t)
	if got := sidebarText(a); !strings.Contains(got, "-- Messages --") {
		t.Fatalf("missing labeled Messages divider:\n%s", got)
	}
}

func TestArchiveHiddenWhenEmpty(t *testing.T) {
	a := archiveTestApp(t)
	if got := sidebarText(a); strings.Contains(got, "Archive") {
		t.Fatalf("archive shown with nothing parked:\n%s", got)
	}
}

func TestCloseParksInArchive(t *testing.T) {
	a := archiveTestApp(t)
	a.closeBuffer(bufIndex(a, "amy"))
	if a.displayRank(a.bufs[bufIndex(a, "amy")]) != 3 {
		t.Fatal("amy not parked at rank 3")
	}
	got := sidebarText(a)
	if !strings.Contains(got, "-- Archive --") {
		t.Fatalf("missing Archive divider:\n%s", got)
	}
	// Parked buffer sorts after the main list.
	if ai, bi := bufIndex(a, "amy"), bufIndex(a, "bob"); ai < bi {
		t.Fatalf("amy (idx %d) not after bob (idx %d)", ai, bi)
	}
}

func TestCloseArchivedRemovesFromView(t *testing.T) {
	a := archiveTestApp(t)
	a.closeBuffer(bufIndex(a, "amy"))
	a.closeBuffer(bufIndex(a, "amy"))
	if bufIndex(a, "amy") != -1 {
		t.Fatal("amy still visible after x in archive")
	}
	if len(a.bufs) != 3 {
		t.Fatalf("len(bufs) = %d, want 3", len(a.bufs))
	}
	if got := sidebarText(a); strings.Contains(got, "Archive") {
		t.Fatalf("archive divider lingered:\n%s", got)
	}
}

func TestRecoverRestoresGrouping(t *testing.T) {
	a := archiveTestApp(t)
	a.closeBuffer(bufIndex(a, "#c")) // parked channel...
	a.closeBuffer(bufIndex(a, "amy"))
	a.recoverBuffer(bufIndex(a, "#c"))
	if got := bufIndex(a, "#c"); got != 1 {
		t.Fatalf("#c recovered to idx %d, want 1 (with channels)", got)
	}
	a.recoverBuffer(bufIndex(a, "amy"))
	if got := bufIndex(a, "amy"); got != 2 {
		t.Fatalf("amy recovered to idx %d, want 2 (with PMs)", got)
	}
	if got := a.bufs[a.focus].Name; got != "amy" {
		t.Fatalf("focus = %q, want amy", got)
	}
	if got := sidebarText(a); strings.Contains(got, "Archive") {
		t.Fatalf("archive divider lingered:\n%s", got)
	}
}

func TestCloseFocusedBufferMovesFocus(t *testing.T) {
	a := archiveTestApp(t)
	a.focusBuffer(bufIndex(a, "amy"))
	a.closeBuffer(bufIndex(a, "amy"))
	if got := a.bufs[a.focus].Name; got != "bob" {
		t.Fatalf("focus = %q, want bob (neighbor)", got)
	}
}

func TestNewMessageUnparks(t *testing.T) {
	a := archiveTestApp(t)
	a.closeBuffer(bufIndex(a, "amy"))
	a.addLine("srv", "amy", store.Line{Text: "hey again"})
	if got := a.displayRank(a.bufs[bufIndex(a, "amy")]); got != 2 {
		t.Fatalf("amy rank = %d after new message, want 2", got)
	}
	a.closeBuffer(bufIndex(a, "amy"))
	a.closeBuffer(bufIndex(a, "amy")) // removed from view
	a.addLine("srv", "amy", store.Line{Text: "back"})
	if bufIndex(a, "amy") == -1 {
		t.Fatal("amy did not return on new message")
	}
}

func TestHoverShowsAffordances(t *testing.T) {
	a := archiveTestApp(t)
	a.hovered = bufIndex(a, "bob")
	lines := strings.Split(sidebarText(a), "\n")
	var bob string
	for _, l := range lines {
		if strings.Contains(l, "bob") {
			bob = l
		}
	}
	if !strings.HasSuffix(strings.TrimRight(bob, " "), "x") {
		t.Fatalf("hovered row missing x:\n%q", bob)
	}
	// Archived rows get + to recover alongside x to remove.
	a.closeBuffer(bufIndex(a, "amy"))
	a.hovered = bufIndex(a, "amy")
	lines = strings.Split(sidebarText(a), "\n")
	var amy string
	for _, l := range lines {
		if strings.Contains(l, "amy") {
			amy = l
		}
	}
	if !strings.Contains(amy, "+") || !strings.HasSuffix(strings.TrimRight(amy, " "), "x") {
		t.Fatalf("archived hovered row missing +/x:\n%q", amy)
	}
	// No hover, no affordances.
	a.hovered = -1
	if got := sidebarText(a); strings.Contains(got, "x\n") {
		t.Fatalf("x shown without hover:\n%s", got)
	}
}

func TestMouseMotionTracksHover(t *testing.T) {
	a := archiveTestApp(t)
	// Row 4 is amy (0 header, 1 srv, 2 #c, 3 Messages divider).
	_, _ = a.Update(tea.MouseMsg{X: 5, Y: 4, Action: tea.MouseActionMotion})
	if a.hovered != bufIndex(a, "amy") {
		t.Fatalf("hovered = %d, want amy", a.hovered)
	}
	// Moving off the sidebar clears the hover.
	_, _ = a.Update(tea.MouseMsg{X: 90, Y: 4, Action: tea.MouseActionMotion})
	if a.hovered != -1 {
		t.Fatalf("hovered = %d after leaving sidebar", a.hovered)
	}
}

func TestClickXParksViaMouse(t *testing.T) {
	a := archiveTestApp(t)
	sw := a.sidebarWidth() // x affordance lives at sw-2
	_, _ = a.Update(tea.MouseMsg{X: 5, Y: 4, Action: tea.MouseActionMotion})
	_, _ = a.Update(tea.MouseMsg{X: sw - 2, Y: 4, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft})
	if a.displayRank(a.bufs[bufIndex(a, "amy")]) != 3 {
		t.Fatal("clicking x did not park amy")
	}
}

func TestClickRecoverViaMouse(t *testing.T) {
	a := archiveTestApp(t)
	a.closeBuffer(bufIndex(a, "amy"))
	// Rows: 0 header, 1 srv, 2 #c, 3 Messages, 4 bob, 5 Archive divider, 6 amy.
	row := 6
	sw := a.sidebarWidth()
	_, _ = a.Update(tea.MouseMsg{X: 5, Y: row, Action: tea.MouseActionMotion})
	_, _ = a.Update(tea.MouseMsg{X: sw - 4, Y: row, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft})
	if got := a.displayRank(a.bufs[bufIndex(a, "amy")]); got != 2 {
		t.Fatalf("clicking + left amy at rank %d, want 2", got)
	}
}

func TestSidebarBufferAtArchiveRows(t *testing.T) {
	a := archiveTestApp(t)
	a.closeBuffer(bufIndex(a, "amy"))
	// 0 header, 1 srv, 2 #c, 3 Messages, 4 bob, 5 Archive divider, 6 amy.
	if _, ok := a.sidebarBufferAt(5, 5); ok {
		t.Fatal("Archive divider row mapped to a buffer")
	}
	if i, ok := a.sidebarBufferAt(5, 6); !ok || a.bufs[i].Name != "amy" {
		t.Fatalf("archive row -> (%d, %v), want amy", i, ok)
	}
}

func TestServerWindowNotCloseable(t *testing.T) {
	a := archiveTestApp(t)
	a.hovered = 0 // server window
	if got := a.sidebarAffordance(0, a.sidebarWidth()-2); got != "" {
		t.Fatalf("server window affordance = %q", got)
	}
	// Hovering the server window renders no x.
	lines := strings.Split(sidebarText(a), "\n")
	for _, l := range lines {
		if strings.Contains(l, "srv") && strings.HasSuffix(strings.TrimRight(l, " "), "x") {
			t.Fatalf("x on server window row:\n%q", l)
		}
	}
}

// Hovering a buffer row must re-render the sidebar so the close affordance
// appears; moving off must clear it again. Regression test: the motion
// handler used to update hover state without re-rendering, so the "x" never
// showed on screen.
func TestHoverReRendersSidebarWithCloseX(t *testing.T) {
	a := archiveTestApp(t)
	before := a.sidebar.View()
	if strings.Contains(before, "x") {
		t.Fatalf("close affordance visible before any hover:\n%s", before)
	}
	// Buffers: rows are 0=server header, 1=server window, 2=channel,
	// 3="-- Messages --" divider, 4=amy (PM), 5=bob (PM). Hover amy's row.
	u, _ := a.Update(tea.MouseMsg{Action: tea.MouseActionMotion, X: 2, Y: 4})
	a = u.(*App)
	if a.hovered != 2 {
		t.Fatalf("hovered = %d, want 2", a.hovered)
	}
	after := a.sidebar.View()
	if !strings.Contains(after, "x") {
		t.Fatalf("close affordance not rendered after hover:\n%s", after)
	}
	// Moving off the sidebar clears the affordance.
	u, _ = a.Update(tea.MouseMsg{Action: tea.MouseActionMotion, X: 100, Y: 3})
	a = u.(*App)
	if a.hovered != -1 {
		t.Fatalf("hovered = %d, want -1 after moving off", a.hovered)
	}
	if strings.Contains(a.sidebar.View(), "x") {
		t.Fatalf("close affordance still visible after moving off:\n%s", a.sidebar.View())
	}
}
