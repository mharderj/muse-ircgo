package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"ircgo/internal/config"
	"ircgo/internal/history"
	"ircgo/internal/store"
)

// newRestoreApp builds an App with logs in dir and one configured server.
func newRestoreApp(t *testing.T, dir string) *App {
	t.Helper()
	old := history.LogDir
	history.LogDir = dir
	t.Cleanup(func() { history.LogDir = old })
	cfg := &config.Config{}
	cfg.Servers = []config.Server{{Name: "srv", Nick: "me"}}
	cfg.HistoryPlayback = config.PlaybackLines{Set: true, Lines: 50}
	return New(cfg, store.New(), nil, nil)
}

func TestPlaybackRegeneratesImageFetches(t *testing.T) {
	dir := t.TempDir()
	a := newRestoreApp(t, dir)
	a.addLine("srv", "belial", store.Line{Nick: "Belial", Text: "look https://example.com/pic.png", Kind: store.KindChat})

	// A fresh session replays the log into the new buffer and must return
	// a fetch command for the image URL: art is never logged, so without
	// the fetch the reopened buffer would show a bare URL.
	b := newRestoreApp(t, dir)
	cmd := b.addLine("srv", "belial", store.Line{Nick: "me", Text: "hi", Kind: store.KindChat})
	if cmd == nil {
		t.Fatal("addLine returned nil cmd, want image fetch for replayed URL")
	}
	if n := len(b.st.Get("srv", "belial").Lines()); n != 2 {
		t.Fatalf("buffer has %d lines, want 2 (replayed + live)", n)
	}
}

func TestRestoreQueryBuffers(t *testing.T) {
	dir := t.TempDir()
	a := newRestoreApp(t, dir)
	a.addLine("srv", "belial", store.Line{Nick: "Belial", Text: "yo", Kind: store.KindChat})
	a.addLine("srv", "belial", store.Line{Nick: "Belial", Text: "see https://example.com/pic.png", Kind: store.KindChat})
	a.addLine("srv", "#chan", store.Line{Nick: "bob", Text: "hey", Kind: store.KindChat})
	a.addLine("srv", "srv", store.Line{Text: "connected", Kind: store.KindSystem})

	b := newRestoreApp(t, dir)
	cmd := b.restoreQueryBuffers()
	if cmd == nil {
		t.Fatal("restoreQueryBuffers returned nil cmd, want image fetch for replayed URL")
	}

	// The DM buffer is back with its history replayed.
	if !b.st.Has("srv", "belial") {
		t.Fatal("belial buffer was not restored")
	}
	if n := len(b.st.Get("srv", "belial").Lines()); n != 2 {
		t.Fatalf("belial has %d lines, want 2 replayed", n)
	}
	// Channels and the server window manage their own buffers.
	if b.st.Has("srv", "#chan") {
		t.Fatal("#chan buffer was restored, want only queries")
	}
	if b.st.Has("srv", "srv") {
		t.Fatal("server window was restored, want only queries")
	}
	// And it shows up in the sidebar list.
	found := false
	for _, buf := range b.bufs {
		if buf.Server == "srv" && buf.Name == "belial" {
			found = true
		}
	}
	if !found {
		t.Fatal("belial not in sidebar buffer list")
	}

	// Idempotent: a second restore (or a DM that arrived first) replays
	// nothing and duplicates no lines.
	b.restoreQueryBuffers()
	if n := len(b.st.Get("srv", "belial").Lines()); n != 2 {
		t.Fatalf("belial has %d lines after second restore, want 2", n)
	}
}

func TestFailedFetchIsNotCached(t *testing.T) {
	a := kickTestApp()
	url := "https://example.com/x.png"
	a.handleImageFetched(imageFetchedMsg{url: url})
	if _, ok := a.imgCache[url]; ok {
		t.Fatal("failed fetch was cached, poisoning the URL for the session")
	}
	// A re-posted URL gets a fresh fetch attempt instead of a cache hit
	// on the earlier failure.
	cmd := a.queueImageFetches("srv", "b", 0, "see "+url)
	if cmd == nil {
		t.Fatal("no fetch queued for re-posted URL")
	}
	if !a.imgInflight[url] {
		t.Fatal("re-posted URL not marked in-flight")
	}
}

func TestFirstWindowSizeRestoresQueries(t *testing.T) {
	dir := t.TempDir()
	a := newRestoreApp(t, dir)
	a.addLine("srv", "belial", store.Line{Nick: "Belial", Text: "yo", Kind: store.KindChat})

	b := newRestoreApp(t, dir)
	if b.st.Has("srv", "belial") {
		t.Fatal("belial exists before first layout")
	}
	if _, _ = b.Update(tea.WindowSizeMsg{Width: 120, Height: 40}); !b.st.Has("srv", "belial") {
		t.Fatal("belial was not restored on first WindowSizeMsg")
	}
}
