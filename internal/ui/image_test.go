package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"

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

func TestImageFetchedAttachesToMessage(t *testing.T) {
	a := testImageApp()
	a.handleMessage("srv", &irc.Message{
		Command: "PRIVMSG", Prefix: "bob!u@h",
		Params: []string{"#a", "look https://x.test/pic.png"},
	})
	before := len(a.st.Get("srv", "#a").Lines())
	m, _ := a.Update(imageFetchedMsg{url: "https://x.test/pic.png", art: "ART"})
	a = m.(*App)
	if a.imgInflight["https://x.test/pic.png"] {
		t.Fatal("URL still in-flight after completion")
	}
	lines := a.st.Get("srv", "#a").Lines()
	if len(lines) != before {
		t.Fatalf("lines = %d, want %d (art must not append a new line)", len(lines), before)
	}
	msg := lines[len(lines)-1]
	if len(msg.Art) != 1 || msg.Art[0] != "ART" {
		t.Fatalf("message art = %+v, want [ART]", msg.Art)
	}
}

func TestImageFetchFailureAppendsNothing(t *testing.T) {
	a := testImageApp()
	a.handleMessage("srv", &irc.Message{
		Command: "PRIVMSG", Prefix: "bob!u@h",
		Params: []string{"#a", "look https://x.test/pic.png"},
	})
	before := a.st.Get("srv", "#a").Lines()
	m, _ := a.Update(imageFetchedMsg{url: "https://x.test/pic.png"})
	a = m.(*App)
	after := a.st.Get("srv", "#a").Lines()
	if len(after) != len(before) {
		t.Fatalf("lines = %d, want %d (no preview on failure)", len(after), len(before))
	}
	if len(after[len(after)-1].Art) != 0 {
		t.Fatalf("art attached on failure: %+v", after[len(after)-1].Art)
	}
}

func TestImageFetchedAfterPartAppendsNothing(t *testing.T) {
	a := testImageApp()
	a.handleMessage("srv", &irc.Message{
		Command: "PRIVMSG", Prefix: "bob!u@h",
		Params: []string{"#a", "look https://x.test/pic.png"},
	})
	a.st.Remove("srv", "#a")
	m, _ := a.Update(imageFetchedMsg{url: "https://x.test/pic.png", art: "ART"})
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

// TestImageArtRendersUnderItsMessage is the regression test for previews
// piling up at the end of the buffer: each finished preview must render
// directly beneath the message that carried its URL.
func TestImageArtRendersUnderItsMessage(t *testing.T) {
	a := kickTestApp()
	a.handleMessage("srv", &irc.Message{
		Command: "PRIVMSG", Prefix: "bob!u@h",
		Params: []string{"#c", "first https://x.test/a.png"},
	})
	a.handleMessage("srv", &irc.Message{
		Command: "PRIVMSG", Prefix: "bob!u@h",
		Params: []string{"#c", "second https://x.test/b.png"},
	})
	m, _ := a.Update(imageFetchedMsg{url: "https://x.test/a.png", art: "ARTA"})
	m, _ = m.(*App).Update(imageFetchedMsg{url: "https://x.test/b.png", art: "ARTB"})
	a = m.(*App)
	a.refreshBuffers()
	for i, b := range a.bufs {
		if b.Name == "#c" {
			a.focus = i
		}
	}
	a.chat = viewport.New(60, 30)
	a.renderChat()
	rows := a.chatContentRows
	msgA, artA, msgB, artB := -1, -1, -1, -1
	for i, r := range rows {
		switch {
		case strings.Contains(r, "first https://x.test/a.png"):
			msgA = i
		case strings.Contains(r, "ARTA"):
			artA = i
		case strings.Contains(r, "second https://x.test/b.png"):
			msgB = i
		case strings.Contains(r, "ARTB"):
			artB = i
		}
	}
	if msgA < 0 || artA < 0 || msgB < 0 || artB < 0 {
		t.Fatalf("missing rows: msgA=%d artA=%d msgB=%d artB=%d", msgA, artA, msgB, artB)
	}
	if artA != msgA+1 || artB != msgB+1 {
		t.Fatalf("art not under its message: msgA=%d artA=%d msgB=%d artB=%d", msgA, artA, msgB, artB)
	}
}

// TestSharedURLFillsEveryMessage: two messages carrying the same image URL
// trigger one fetch, but both get the preview.
func TestSharedURLFillsEveryMessage(t *testing.T) {
	a := testImageApp()
	for _, text := range []string{"one https://x.test/s.png", "two https://x.test/s.png"} {
		if cmd := a.handleMessage("srv", &irc.Message{
			Command: "PRIVMSG", Prefix: "bob!u@h",
			Params: []string{"#a", text},
		}); text == "two https://x.test/s.png" && cmd != nil {
			t.Fatal("shared URL queued a second fetch")
		}
	}
	m, _ := a.Update(imageFetchedMsg{url: "https://x.test/s.png", art: "ART"})
	a = m.(*App)
	lines := a.st.Get("srv", "#a").Lines()
	for _, l := range lines {
		if l.Kind != store.KindChat || !strings.Contains(l.Text, "https://x.test/s.png") {
			continue
		}
		if len(l.Art) != 1 || l.Art[0] != "ART" {
			t.Fatalf("message %q art = %+v, want [ART]", l.Text, l.Art)
		}
	}
}
