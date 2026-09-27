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

// writeQuitConfig makes a temp config.toml the app can surgically update.
func writeQuitConfig(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	content := "# test config\n[ui]\n\n[[server]]\nname = \"srv\"\nhost = \"example.com\"\nnick = \"me\"\n"
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func focusNamed(t *testing.T, a *App, name string) {
	t.Helper()
	for i, b := range a.bufs {
		if b.Name == name {
			a.focusBuffer(i)
			return
		}
	}
	t.Fatalf("no buffer %q", name)
}

func TestFocusSwitchPersistsImmediately(t *testing.T) {
	p := writeQuitConfig(t)
	a := kickTestApp()
	a.ConfigPath = p
	a.addLine("srv", "srv", store.Line{Text: "hi"})
	a.addLine("srv", "#c", store.Line{Text: "hi"})
	a.refreshBuffers()
	focusNamed(t, a, "#c")
	// The switch hits the disk at once: even without a clean quit, the
	// next launch must restore #c.
	cfg, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.LastBuffer) != 2 || cfg.LastBuffer[0] != "srv" || cfg.LastBuffer[1] != "#c" {
		t.Fatalf("LastBuffer = %q, want [srv #c]", cfg.LastBuffer)
	}
}

// TestUncleanShutdownRestoresFocusedBuffer replays the reported bug: the
// app was closed on #linux without a clean quit (no QuitMsg, so the
// quit-time write never ran) and came back on the wrong buffer. Because
// focus persists on every switch, the on-disk state is already right.
func TestUncleanShutdownRestoresFocusedBuffer(t *testing.T) {
	p := writeQuitConfig(t)
	a := kickTestApp()
	a.ConfigPath = p
	a.addLine("srv", "srv", store.Line{Text: "hi"})
	a.addLine("srv", "#c", store.Line{Text: "hi"})
	a.addLine("srv", "dave", store.Line{Text: "hi"})
	a.refreshBuffers()
	focusNamed(t, a, "#c")
	// No QuitMsg: simulate the process dying here (closed terminal).
	b := kickTestApp()
	b.ConfigPath = p
	bcfg, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	b.cfg = bcfg
	b.addLine("srv", "dave", store.Line{Text: "hi"})
	b.refreshBuffers()
	b.restoreLastFocus() // #c not present yet: remembered, not forced
	if b.pendingFocus.name != "#c" {
		t.Fatalf("pendingFocus = %+v, want #c", b.pendingFocus)
	}
	b.addLine("srv", "#c", store.Line{Text: "hi"})
	b.refreshBuffers() // #c appears: pending restore fires
	if b.pendingFocus.server != "" {
		t.Fatal("pendingFocus not consumed")
	}
	if b.bufs[b.focus].Name != "#c" {
		t.Fatalf("focused = %q, want #c", b.bufs[b.focus].Name)
	}
}

func TestQuitPersistsFocusedBuffer(t *testing.T) {
	p := writeQuitConfig(t)
	a := kickTestApp()
	a.ConfigPath = p
	a.addLine("srv", "srv", store.Line{Text: "hi"})
	a.addLine("srv", "#c", store.Line{Text: "hi"})
	a.refreshBuffers()
	focusNamed(t, a, "#c")
	_, _ = a.Update(tea.QuitMsg{})
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "# test config") {
		t.Fatalf("comment lost:\n%s", s)
	}
	cfg, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.LastBuffer) != 2 || cfg.LastBuffer[0] != "srv" || cfg.LastBuffer[1] != "#c" {
		t.Fatalf("LastBuffer = %q", cfg.LastBuffer)
	}
}

func TestRestoreLastFocusSelectsSavedBuffer(t *testing.T) {
	a := kickTestApp()
	a.cfg.LastBuffer = []string{"srv", "#c"}
	a.addLine("srv", "srv", store.Line{Text: "hi"})
	a.addLine("srv", "#c", store.Line{Text: "hi"})
	a.refreshBuffers()
	a.restoreLastFocus()
	if got := a.bufs[a.focus].Name; got != "#c" {
		t.Fatalf("focus = %q, want #c", got)
	}
	if a.pendingFocus.server != "" {
		t.Fatal("pendingFocus should be clear")
	}
}

// TestRestoreLastFocusWaitsForChannel covers the startup race: channels
// don't exist until JOIN creates them, so the restore waits and focuses
// the buffer when it appears.
func TestRestoreLastFocusWaitsForChannel(t *testing.T) {
	a := kickTestApp()
	a.cfg.LastBuffer = []string{"srv", "#c"}
	a.addLine("srv", "srv", store.Line{Text: "hi"})
	a.refreshBuffers()
	a.restoreLastFocus()
	if a.pendingFocus.name != "#c" {
		t.Fatalf("pendingFocus = %+v, want #c", a.pendingFocus)
	}
	a.addLine("srv", "#c", store.Line{Text: "hi"})
	a.refreshBuffers()
	if got := a.bufs[a.focus].Name; got != "#c" {
		t.Fatalf("focus = %q, want #c", got)
	}
	if a.pendingFocus.server != "" {
		t.Fatal("pendingFocus not cleared")
	}
}

// TestManualSwitchCancelsPendingRestore: if the user switches buffers
// before the remembered channel appears, the pending jump is dropped and
// never yanks focus later.
func TestManualSwitchCancelsPendingRestore(t *testing.T) {
	a := kickTestApp()
	a.cfg.LastBuffer = []string{"srv", "#c"}
	a.addLine("srv", "srv", store.Line{Text: "hi"})
	a.addLine("srv", "#a", store.Line{Text: "hi"})
	a.refreshBuffers()
	a.restoreLastFocus()
	focusNamed(t, a, "#a")
	if a.pendingFocus.server != "" {
		t.Fatal("manual switch did not cancel pending restore")
	}
	a.addLine("srv", "#c", store.Line{Text: "hi"})
	a.refreshBuffers()
	if got := a.bufs[a.focus].Name; got != "#a" {
		t.Fatalf("focus = %q, want #a (no yank)", got)
	}
}

func TestRestoreLastFocusIgnoresGarbage(t *testing.T) {
	for _, lb := range [][]string{nil, {}, {"only"}, {"", ""}, {"srv", "#gone"}} {
		a := kickTestApp()
		a.cfg.LastBuffer = lb
		a.addLine("srv", "srv", store.Line{Text: "hi"})
		a.refreshBuffers()
		a.restoreLastFocus() // must not panic or move focus
		if got := a.bufs[a.focus].Name; got != "srv" {
			t.Fatalf("LastBuffer=%q: focus = %q, want srv", lb, got)
		}
	}
}
