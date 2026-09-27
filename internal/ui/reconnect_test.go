package ui

import (
	"strings"
	"testing"
	"time"

	"ircgo/internal/config"
	"ircgo/internal/irc"
	"ircgo/internal/store"
)

// reconnectApp builds an app with a live (but never started) client for
// "srv" and one channel buffer #a focused.
func reconnectApp() (*App, *irc.Client, *store.Buffer) {
	cfg := &config.Config{}
	cfg.UI.TimestampFormat = "15:04" // what config.Load defaults to
	cl := irc.New(config.Server{Name: "srv", Nick: "me"}, make(chan irc.Event, 8))
	a := New(cfg, store.New(), nil, map[string]*irc.Client{"srv": cl})
	a.addLine("srv", "#a", store.Line{At: time.Now(), Nick: "bob", Text: "hi", Kind: store.KindChat})
	a.bufs = a.st.Buffers()
	var buf *store.Buffer
	for i, b := range a.bufs {
		if b.Name == "#a" {
			buf = b
			a.focus = i
		}
	}
	if buf == nil {
		panic("no #a buffer")
	}
	return a, cl, buf
}

func TestReconnectCommandAcks(t *testing.T) {
	a, cl, buf := reconnectApp()
	// The client was never started: ReconnectNow must not block or panic.
	a.sendCommand(cl, buf, "/reconnect")
	if content := stripANSI(a.chatContent()); !strings.Contains(content, "reconnecting now…") {
		t.Fatalf("missing reconnect ack:\\n%s", content)
	}
}

func TestSendWhileDisconnectedWarns(t *testing.T) {
	a, _, _ := reconnectApp()
	// Client never connected: the message must not be echoed as if sent.
	a.sendInput("testmessage123")
	content := stripANSI(a.chatContent())
	if !strings.Contains(content, "not connected — message not sent") {
		t.Fatalf("missing disconnect warning:\\n%s", content)
	}
	if strings.Contains(content, "testmessage123") {
		t.Fatalf("message echoed as sent while disconnected:\\n%s", content)
	}
}

func TestConnDownTracking(t *testing.T) {
	a, _, _ := reconnectApp()
	if a.anyConnDown() {
		t.Fatal("anyConnDown = true before any disconnect")
	}
	a.handleEvent(irc.Event{Server: "srv", Kind: irc.KindDisconnected})
	if !a.anyConnDown() {
		t.Fatal("anyConnDown = false after disconnect")
	}
	// A second server staying up doesn't mask the down one.
	a.handleEvent(irc.Event{Server: "other", Kind: irc.KindConnected})
	if !a.anyConnDown() {
		t.Fatal("anyConnDown = false while srv still down")
	}
	a.handleEvent(irc.Event{Server: "srv", Kind: irc.KindConnected})
	if a.anyConnDown() {
		t.Fatal("anyConnDown = true after reconnect")
	}
}
