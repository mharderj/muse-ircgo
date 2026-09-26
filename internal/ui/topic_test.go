package ui

import (
	"strings"
	"testing"

	"ircgo/internal/config"
	"ircgo/internal/irc"
	"ircgo/internal/store"
)

func testTopicApp() *App {
	a := New(&config.Config{}, store.New(), nil, nil)
	a.addLine("srv", "srv", store.Line{Text: "hi"})
	a.addLine("srv", "#a", store.Line{Text: "hi"})
	a.bufs = a.st.Buffers()
	return a
}

func TestTopicFrom332(t *testing.T) {
	a := testTopicApp()
	a.handleMessage("srv", &irc.Message{Command: "332", Params: []string{"me", "#a", "welcome to #a"}})
	if got := a.st.Get("srv", "#a").Topic; got != "welcome to #a" {
		t.Fatalf("topic = %q, want %q", got, "welcome to #a")
	}
}

func TestTopicCommandUpdates(t *testing.T) {
	a := testTopicApp()
	a.handleMessage("srv", &irc.Message{
		Command: "TOPIC", Prefix: "bob!u@h", Params: []string{"#a", "new topic"},
	})
	if got := a.st.Get("srv", "#a").Topic; got != "new topic" {
		t.Fatalf("topic = %q, want %q", got, "new topic")
	}
	lines := a.st.Get("srv", "#a").Lines()
	last := lines[len(lines)-1]
	if last.Kind != store.KindSystem || last.Text != "changed the topic to: new topic" {
		t.Fatalf("last line = %+v, want topic-change system line", last)
	}
	if last.Nick != "bob" {
		t.Fatalf("last line nick = %q, want bob", last.Nick)
	}
}

func TestTopicVisibleOnlyForChannels(t *testing.T) {
	a := testTopicApp()
	a.focus = 0 // server window
	if a.topicVisible() {
		t.Fatal("topicVisible for server window, want false")
	}
	a.focus = 1 // #a
	if !a.topicVisible() {
		t.Fatal("topicVisible for channel, want true")
	}
}

func TestTopicBarShowsTopic(t *testing.T) {
	a := testTopicApp()
	a.st.Get("srv", "#a").Topic = "hello world"
	a.focus = 1
	a.width, a.height = 100, 30
	a.resize()
	if bar := a.topicBar(); !strings.Contains(bar, "Topic: hello world") {
		t.Fatalf("topic bar = %q, want it to contain the topic", bar)
	}
}

func TestTopicBarEmpty(t *testing.T) {
	a := testTopicApp()
	a.focus = 1
	a.width, a.height = 100, 30
	a.resize()
	if bar := a.topicBar(); !strings.Contains(bar, "no topic") {
		t.Fatalf("topic bar = %q, want placeholder", bar)
	}
}
