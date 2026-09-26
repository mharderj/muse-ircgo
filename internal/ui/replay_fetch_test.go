package ui

import (
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"ircgo/internal/irc"
	"ircgo/internal/store"
)

// TestReplayedImageURLsFetchEndToEnd replays a log with several image URLs
// through a fresh App, executes the fetch commands playbackHistory returns,
// and requires art for every URL.
func TestReplayedImageURLsFetchEndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := image.NewRGBA(image.Rect(0, 0, 4, 4))
		for y := 0; y < 4; y++ {
			for x := 0; x < 4; x++ {
				m.Set(x, y, color.RGBA{200, 100, 50, 255})
			}
		}
		w.Header().Set("Content-Type", "image/png")
		_ = png.Encode(w, m)
	}))
	defer srv.Close()

	dir := t.TempDir()
	a := newRestoreApp(t, dir)
	urls := []string{srv.URL + "/a.png", srv.URL + "/b.png", srv.URL + "/c.png"}
	for _, u := range urls {
		a.addLine("srv", "#c", store.Line{Nick: "bob", Text: "see " + u, Kind: store.KindChat})
	}

	b := newRestoreApp(t, dir)
	cmd := b.addLine("srv", "#c", store.Line{Nick: "me", Text: "live", Kind: store.KindChat})
	if cmd == nil {
		t.Fatal("addLine returned nil cmd")
	}
	if len(b.imgInflight) != len(urls) {
		t.Fatalf("imgInflight has %d urls, want %d", len(b.imgInflight), len(urls))
	}

	// Execute the batch: every fetch must come back with art.
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("cmd returned %T, want tea.BatchMsg", msg)
	}
	got := map[string]bool{}
	for _, sub := range batch {
		res := sub()
		im, ok := res.(imageFetchedMsg)
		if !ok {
			continue
		}
		// Feed it through the real handler like Update would.
		b.handleImageFetched(im)
		got[im.url] = im.art != ""
	}
	for _, u := range urls {
		art, ok := got[u]
		if !ok {
			t.Errorf("no fetch result for %s", u)
			continue
		}
		if !art {
			t.Errorf("fetch for %s returned no art", u)
		}
	}
}

// TestJoinPropagatesReplayFetchCmd is the regression test for the real bug:
// handleMessage's JOIN branch used to discard addLine's return value, so a
// channel buffer created by JOIN never ran its replay image fetches.
func TestJoinPropagatesReplayFetchCmd(t *testing.T) {
	dir := t.TempDir()
	a := newRestoreApp(t, dir)
	a.addLine("srv", "#c", store.Line{Nick: "bob", Text: "see https://example.com/pic.png", Kind: store.KindChat})

	b := newRestoreApp(t, dir)
	join := &irc.Message{
		Prefix:  "me!u@h",
		Command: "JOIN",
		Params:  []string{"#c"},
	}
	if cmd := b.handleMessage("srv", join); cmd == nil {
		t.Fatal("JOIN for a new channel dropped the replay fetch cmd")
	}
	if len(b.imgInflight) != 1 {
		t.Fatalf("imgInflight has %d urls, want 1", len(b.imgInflight))
	}
}
