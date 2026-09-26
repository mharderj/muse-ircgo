package ui

import (
	"os"
	"strings"
	"testing"
	"time"

	"ircgo/internal/config"
	"ircgo/internal/history"
	"ircgo/internal/store"
)

// newHistoryApp builds an App whose channel logs live in dir.
func newHistoryApp(t *testing.T, dir string, lines int) *App {
	t.Helper()
	old := history.LogDir
	history.LogDir = dir
	t.Cleanup(func() { history.LogDir = old })
	cfg := &config.Config{}
	cfg.HistoryPlayback = config.PlaybackLines{Set: true, Lines: lines}
	return New(cfg, store.New(), nil, nil)
}

func TestAddLineLogs(t *testing.T) {
	dir := t.TempDir()
	a := newHistoryApp(t, dir, 50)

	at := time.Date(2026, 9, 26, 14, 18, 4, 0, time.Local)
	a.addLine("srv", "#a", store.Line{At: at, Nick: "bob", Text: "first", Kind: store.KindChat})
	a.addLine("srv", "#a", store.Line{At: at, Nick: "bob", Text: "second", Kind: store.KindChat})
	// Image art is not logged.
	a.addLine("srv", "#a", store.Line{At: at, Text: "art", Kind: store.KindImage})

	data, err := os.ReadFile(history.Path("srv", "#a"))
	if err != nil {
		t.Fatal(err)
	}
	want := "[2026-09-26 14:18:04] <bob> first\n[2026-09-26 14:18:04] <bob> second\n"
	if string(data) != want {
		t.Fatalf("log = %q, want %q", data, want)
	}
}

func TestAddLinePlaysBackHistory(t *testing.T) {
	dir := t.TempDir()
	a := newHistoryApp(t, dir, 50)
	at := time.Date(2026, 9, 26, 14, 18, 4, 0, time.Local)
	for _, text := range []string{"one", "two", "three"} {
		a.addLine("srv", "#a", store.Line{At: at, Nick: "bob", Text: text, Kind: store.KindChat})
	}

	// A fresh app replays the last 2 log lines into the new buffer,
	// oldest first, ahead of the live line.
	b := newHistoryApp(t, dir, 2)
	b.addLine("srv", "#a", store.Line{At: at, Nick: "bob", Text: "live", Kind: store.KindChat})
	buf := b.st.Get("srv", "#a")
	lines := buf.Lines()
	if len(lines) != 3 {
		t.Fatalf("buffer has %d lines, want 3", len(lines))
	}
	for i, want := range []string{"two", "three", "live"} {
		if lines[i].Text != want {
			t.Fatalf("line %d = %q, want %q", i, lines[i].Text, want)
		}
	}

	// Playback does not re-log history lines: the log still holds each
	// message exactly once.
	data, err := os.ReadFile(history.Path("srv", "#a"))
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"one", "two", "three"} {
		if n := strings.Count(string(data), "<bob> "+text+"\n"); n != 1 {
			t.Fatalf("%q appears %d times in log, want 1", text, n)
		}
	}
}

func TestAddLinePlaybackDisabled(t *testing.T) {
	dir := t.TempDir()
	a := newHistoryApp(t, dir, 50)
	at := time.Date(2026, 9, 26, 14, 18, 4, 0, time.Local)
	a.addLine("srv", "#a", store.Line{At: at, Nick: "bob", Text: "old", Kind: store.KindChat})

	b := newHistoryApp(t, dir, 0)
	b.addLine("srv", "#a", store.Line{At: at, Nick: "bob", Text: "live", Kind: store.KindChat})
	if lines := b.st.Get("srv", "#a").Lines(); len(lines) != 1 || lines[0].Text != "live" {
		t.Fatalf("buffer = %+v, want only the live line", lines)
	}
}
