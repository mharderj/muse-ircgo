package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"ircgo/internal/config"
	"ircgo/internal/store"
)

// dragTestApp builds a sidebar with a server window, two channels and two
// PMs: bufs = [srv, #a, #b, amy, bob]. Rows: 0 srv, 1 #a, 2 #b,
// 3 rule, 4 Messages, 5 amy, 6 bob.
func dragTestApp(t *testing.T) *App {
	t.Helper()
	a := kickTestApp()
	a.width, a.height = 100, 30
	a.resize()
	a.addLine("srv", "srv", store.Line{Text: "hi"})
	a.addLine("srv", "#a", store.Line{Text: "hi"})
	a.addLine("srv", "#b", store.Line{Text: "hi"})
	a.addLine("srv", "amy", store.Line{Text: "hi"})
	a.addLine("srv", "bob", store.Line{Text: "hi"})
	a.refreshBuffers()
	a.renderSidebar()
	return a
}

func bufOrder(a *App) []string {
	var names []string
	for _, b := range a.bufs {
		names = append(names, b.Name)
	}
	return names
}

func dragPress(a *App, x, y int) {
	a.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
}

func dragMotion(a *App, x, y int) {
	a.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft})
}

func dragRelease(a *App, x, y int) {
	a.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft})
}

// dragTo performs press/motion/release from srcRow to dstRow, activating
// the drag with an intermediate stop past the press row.
func dragTo(a *App, srcRow, dstRow int) {
	dragPress(a, 5, srcRow)
	dragMotion(a, 5, 10) // leaves the press row: drag activates
	if !a.dragActive {
		return
	}
	dragMotion(a, 5, dstRow)
	dragRelease(a, 5, dstRow)
}

func TestDragReorderChannelsDown(t *testing.T) {
	a := dragTestApp(t)
	dragTo(a, 1, 2) // #a down onto #b
	if got, want := strings.Join(bufOrder(a), ","), "srv,#b,#a,amy,bob"; got != want {
		t.Fatalf("order = %s, want %s", got, want)
	}
	if a.archived[memberKey("srv", "#a")] {
		t.Fatal("#a got parked by a reorder drag")
	}
}

func TestDragReorderChannelsUp(t *testing.T) {
	a := dragTestApp(t)
	dragTo(a, 2, 1) // #b up onto #a
	if got, want := strings.Join(bufOrder(a), ","), "srv,#b,#a,amy,bob"; got != want {
		t.Fatalf("order = %s, want %s", got, want)
	}
}

func TestDragReorderPMs(t *testing.T) {
	a := dragTestApp(t)
	dragTo(a, 6, 5) // bob up onto amy
	if got, want := strings.Join(bufOrder(a), ","), "srv,#a,#b,bob,amy"; got != want {
		t.Fatalf("order = %s, want %s", got, want)
	}
}

func TestDragRejectsCrossGroup(t *testing.T) {
	a := dragTestApp(t)
	before := strings.Join(bufOrder(a), ",")
	dragTo(a, 1, 5) // #a onto amy (PM group): no-op
	if got := strings.Join(bufOrder(a), ","); got != before {
		t.Fatalf("cross-group drag reordered: %s -> %s", before, got)
	}
	if len(a.archived) != 0 {
		t.Fatalf("cross-group drag parked something: %v", a.archived)
	}
}

func TestDragToArchivePartsChannel(t *testing.T) {
	a := dragTestApp(t)
	dragPress(a, 5, 1)   // #a
	dragMotion(a, 5, 10) // activate; archive section appears at the bottom
	if !a.dragActive {
		t.Fatal("drag did not activate")
	}
	divRow := sidebarRowWith(t, a, "Archive")
	dragMotion(a, 5, divRow)
	if a.dropKind != "archive" {
		t.Fatalf("dropKind = %q over the archive divider, want archive", a.dropKind)
	}
	dragRelease(a, 5, divRow)
	key := memberKey("srv", "#a")
	if !a.archived[key] {
		t.Fatal("#a was not parked by drag-to-archive")
	}
	if !a.partedOnPark[key] {
		t.Fatal("drag-to-archive did not record the auto-PART")
	}
	if got := a.displayRank(a.bufs[bufIndex(a, "#a")]); got != 3 {
		t.Fatalf("#a rank = %d, want 3", got)
	}
}

func TestDragPMToArchiveNoPart(t *testing.T) {
	a := dragTestApp(t)
	dragPress(a, 5, 5) // amy
	dragMotion(a, 5, 10)
	if !a.dragActive {
		t.Fatal("drag did not activate")
	}
	divRow := sidebarRowWith(t, a, "Archive")
	dragMotion(a, 5, divRow)
	dragRelease(a, 5, divRow)
	key := memberKey("srv", "amy")
	if !a.archived[key] {
		t.Fatal("amy was not parked by drag-to-archive")
	}
	if a.partedOnPark[key] {
		t.Fatal("PM drag-to-archive recorded a PART")
	}
}

func TestDragRecoverFromArchive(t *testing.T) {
	a := dragTestApp(t)
	a.closeBuffer(bufIndex(a, "amy"))
	amyRow := sidebarRowWith(t, a, "amy")
	dragPress(a, 5, amyRow)
	dragMotion(a, 5, 10)
	if !a.dragActive {
		t.Fatal("drag did not activate")
	}
	// bob sits at row 5 now that amy is parked.
	dragMotion(a, 5, 5)
	if a.dropKind != "recover" {
		t.Fatalf("dropKind = %q over a PM row, want recover", a.dropKind)
	}
	dragRelease(a, 5, 5)
	key := memberKey("srv", "amy")
	if a.archived[key] {
		t.Fatal("amy still archived after drag-recover")
	}
	if got := a.displayRank(a.bufs[bufIndex(a, "amy")]); got != 2 {
		t.Fatalf("amy rank = %d, want 2 (PM group)", got)
	}
	// amy returns to its original slot, not wherever it was dropped.
	if got, want := strings.Join(bufOrder(a), ","), "srv,#a,#b,amy,bob"; got != want {
		t.Fatalf("order = %s, want %s", got, want)
	}
}

func TestDragRecoverClearsPartedOnPark(t *testing.T) {
	a := dragTestApp(t)
	a.closeBuffer(bufIndex(a, "#a"))
	key := memberKey("srv", "#a")
	a.partedOnPark[key] = true // as if drag-parked with an auto-PART
	archRow := sidebarRowWith(t, a, "#a")
	dragPress(a, 5, archRow)
	dragMotion(a, 5, 10)
	dragMotion(a, 5, 1) // onto #b's row: recover into the channel group
	dragRelease(a, 5, 1)
	if a.archived[key] {
		t.Fatal("#a still archived after drag-recover")
	}
	if a.partedOnPark[key] {
		t.Fatal("partedOnPark not cleared on recover (JOIN path skipped)")
	}
}

func TestDragPlainClickStillFocuses(t *testing.T) {
	a := dragTestApp(t)
	amyIdx := bufIndex(a, "amy")
	dragPress(a, 5, 5)
	dragRelease(a, 5, 5) // no motion: plain click
	if a.dragActive {
		t.Fatal("plain click activated a drag")
	}
	if a.focus != amyIdx {
		t.Fatalf("focus = %d, want amy at %d", a.focus, amyIdx)
	}
	if len(a.archived) != 0 {
		t.Fatal("plain click parked a buffer")
	}
}

func TestDragAffordancePressNeverDrags(t *testing.T) {
	a := dragTestApp(t)
	sw := a.sidebarWidth()
	// Hover amy so the x affordance exists, then press it and move.
	a.Update(tea.MouseMsg{X: 5, Y: 5, Action: tea.MouseActionMotion, Button: tea.MouseButtonNone})
	dragPress(a, sw-2, 5)
	dragMotion(a, sw-2, 12)
	if a.dragActive {
		t.Fatal("affordance press started a drag")
	}
	dragRelease(a, sw-2, 5)
	if !a.archived[memberKey("srv", "amy")] {
		t.Fatal("x affordance click did not park amy")
	}
}

func TestDragTracksBufferByKeyAcrossShift(t *testing.T) {
	a := dragTestApp(t)
	dragPress(a, 5, 5)  // amy
	dragMotion(a, 5, 2) // activate
	if !a.dragActive {
		t.Fatal("drag did not activate")
	}
	// An IRC event shifts the sidebar mid-drag: #z joins, pushing amy's
	// index down one.
	a.addLine("srv", "#z", store.Line{Text: "hi"})
	a.refreshBuffers()
	a.renderSidebar()
	divRow := sidebarRowWith(t, a, "Archive")
	dragMotion(a, 5, divRow)
	// The motion re-resolved the dragged buffer by key after the shift.
	if got := a.bufs[a.dragIdx].Name; got != "amy" {
		t.Fatalf("dragIdx tracks %q after the shift, want amy", got)
	}
	dragRelease(a, 5, divRow)
	if !a.archived[memberKey("srv", "amy")] {
		t.Fatal("amy was not parked; the drop hit the shifted row instead")
	}
	if a.archived[memberKey("srv", "#z")] {
		t.Fatal("#z was parked instead of amy: stale index used at drop")
	}
}

func TestDragOrderPersistsAcrossRestart(t *testing.T) {
	a := dragTestApp(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	// The order writer never creates a config file unprompted.
	if err := os.WriteFile(path, []byte("[[server]]\nname = \"srv\"\nhost = \"example.com\"\nnick = \"tester\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a.ConfigPath = path
	dragTo(a, 1, 2) // #a down onto #b
	// writeBufferOrder ran as part of the drop; read it back raw.
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	var names []string
	for _, p := range cfg.BufferOrder {
		names = append(names, p[1])
	}
	if got, want := strings.Join(names, ","), "srv,#b,#a,amy,bob"; got != want {
		t.Fatalf("persisted order = %s, want %s", got, want)
	}
	// A fresh app restores the persisted order: like real startup, the
	// order loads before any buffers are tracked.
	b := kickTestApp()
	b.width, b.height = 100, 30
	b.resize()
	b.ConfigPath = path
	b.cfg, err = config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	b.loadBufferOrder()
	b.addLine("srv", "srv", store.Line{Text: "hi"})
	b.addLine("srv", "#a", store.Line{Text: "hi"})
	b.addLine("srv", "#b", store.Line{Text: "hi"})
	b.addLine("srv", "amy", store.Line{Text: "hi"})
	b.addLine("srv", "bob", store.Line{Text: "hi"})
	b.refreshBuffers()
	b.renderSidebar()
	if got, want := strings.Join(bufOrder(b), ","), "srv,#b,#a,amy,bob"; got != want {
		t.Fatalf("restored order = %s, want %s", got, want)
	}
}

func TestDragCancelsOnKeypress(t *testing.T) {
	a := dragTestApp(t)
	before := strings.Join(bufOrder(a), ",")
	dragPress(a, 5, 2)
	dragMotion(a, 5, 10)
	if !a.dragActive {
		t.Fatal("drag did not activate")
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if a.dragActive {
		t.Fatal("keypress did not cancel the drag")
	}
	if got := strings.Join(bufOrder(a), ","); got != before {
		t.Fatalf("cancelled drag changed order: %s -> %s", before, got)
	}
}
