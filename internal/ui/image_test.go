package ui

import (
	"testing"

	"ircgo/internal/config"
	"ircgo/internal/irc"
	"ircgo/internal/store"
)

func testImageApp() *App {
	a := New(&config.Config{}, store.New(), nil, nil)
	a.addLine("srv", "srv", store.Line{Text: "hi"})
	a.addLine("srv", "#a", store.Line{Text: "hi"})
	a.bufs = a.st.Buffers()
	return a
}

func TestImageURLQueuesFetch(t *testing.T) {
	a := testImageApp()
	cmd := a.handleMessage("srv", &irc.Message{
		Command: "PRIVMSG", Prefix: "bob!u@h",
		Params: []string{"#a", "look https://x.test/pic.png"},
	})
	if cmd == nil {
		t.Fatal("want a fetch command for an image URL")
	}
	if !a.imgInflight["https://x.test/pic.png"] {
		t.Fatal("URL not marked in-flight")
	}
	// Same URL again: deduped, no new command.
	if cmd := a.handleMessage("srv", &irc.Message{
		Command: "PRIVMSG", Prefix: "bob!u@h",
		Params: []string{"#a", "again https://x.test/pic.png"},
	}); cmd != nil {
		t.Fatal("duplicate URL queued a second fetch")
	}
	// Non-image URLs queue nothing.
	if cmd := a.handleMessage("srv", &irc.Message{
		Command: "PRIVMSG", Prefix: "bob!u@h",
		Params: []string{"#a", "see https://example.com/page"},
	}); cmd != nil {
		t.Fatal("non-image URL queued a fetch")
	}
}

func TestImageFetchedAppendsPreview(t *testing.T) {
	a := testImageApp()
	m, _ := a.Update(imageFetchedMsg{server: "srv", buf: "#a", url: "u", art: "ART"})
	a = m.(*App)
	if a.imgInflight["u"] {
		t.Fatal("URL still in-flight after completion")
	}
	lines := a.st.Get("srv", "#a").Lines()
	last := lines[len(lines)-1]
	if last.Kind != store.KindImage || last.Text != "ART" {
		t.Fatalf("last line = %+v, want KindImage with art", last)
	}
}

func TestImageFetchFailureAppendsNothing(t *testing.T) {
	a := testImageApp()
	before := len(a.st.Get("srv", "#a").Lines())
	m, _ := a.Update(imageFetchedMsg{server: "srv", buf: "#a", url: "u"})
	a = m.(*App)
	if got := len(a.st.Get("srv", "#a").Lines()); got != before {
		t.Fatalf("lines = %d, want %d (no preview on failure)", got, before)
	}
}

func TestImageFetchedAfterPartAppendsNothing(t *testing.T) {
	a := testImageApp()
	a.st.Remove("srv", "#a")
	m, _ := a.Update(imageFetchedMsg{server: "srv", buf: "#a", url: "u", art: "ART"})
	a = m.(*App)
	if a.st.Has("srv", "#a") {
		t.Fatal("parted buffer was resurrected by a late fetch")
	}
}

func TestBareHashJoinAccepted(t *testing.T) {
	// A literal "#" is a real channel (not junk replay data).
	a := testImageApp()
	a.handleMessage("srv", &irc.Message{
		Command: "JOIN", Prefix: "bob!u@h", Params: []string{"#"},
	})
	if !a.st.Has("srv", "#") {
		t.Fatal("JOIN # did not open a buffer")
	}
}
