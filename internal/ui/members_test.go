package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"ircgo/internal/config"
	"ircgo/internal/store"
)

func testApp() *App {
	return New(&config.Config{}, store.New(), nil, nil)
}

func TestMemberSort(t *testing.T) {
	a := testApp()
	a.memberAdd("srv", "#c", "zed", "")
	a.memberAdd("srv", "#c", "amy", "+")
	a.memberAdd("srv", "#c", "bob", "@")
	a.memberAdd("srv", "#c", "cal", "")

	var got []string
	for _, n := range a.members[memberKey("srv", "#c")].sorted() {
		got = append(got, n.prefix+n.nick)
	}
	want := []string{"@bob", "+amy", "cal", "zed"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestHandleNames(t *testing.T) {
	a := testApp()
	// ":srv 353 me = #c :@op +voice plain"
	a.handleNames("srv", "@op +voice plain", []string{"me", "=", "#c", "@op +voice plain"})
	ms := a.members[memberKey("srv", "#c")]
	if ms == nil {
		t.Fatal("no member set created")
	}
	if ms.nicks["op"] != "@" || ms.nicks["voice"] != "+" || ms.nicks["plain"] != "" {
		t.Fatalf("bad prefixes: %v", ms.nicks)
	}
	// Unknown channel param shape is ignored.
	a.handleNames("srv", "x", []string{"me", "="})
}

func TestMemberQuitAndRename(t *testing.T) {
	a := testApp()
	a.memberAdd("srv", "#a", "zed", "")
	a.memberAdd("srv", "#b", "zed", "+")
	a.memberQuit("srv", "zed")
	for k, ms := range a.members {
		if len(ms.nicks) != 0 {
			t.Fatalf("quit left %v in %s", ms.nicks, k)
		}
	}

	a.memberAdd("srv", "#a", "old", "@")
	a.memberRename("srv", "old", "new")
	ms := a.members[memberKey("srv", "#a")]
	if ms.nicks["new"] != "@" {
		t.Fatalf("rename lost prefix: %v", ms.nicks)
	}
	if _, ok := ms.nicks["old"]; ok {
		t.Fatalf("old nick still present: %v", ms.nicks)
	}
}

func TestNickPaneVisibility(t *testing.T) {
	a := testApp()
	a.st.Get("znc", "#thezone")
	a.bufs = a.st.Buffers()
	a.memberAdd("znc", "#thezone", "thadood", "@")
	a.memberAdd("znc", "#thezone", "Botto", "")
	m, _ := a.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	a = m.(*App)
	v := a.View()
	if !strings.Contains(v, "@thadood") || !strings.Contains(v, "Botto") {
		t.Fatalf("nick pane missing from view:\n%s", v)
	}
	// Narrow terminal: pane must disappear, chat still renders.
	m, _ = a.Update(tea.WindowSizeMsg{Width: 70, Height: 40})
	a = m.(*App)
	if strings.Contains(a.View(), "@thadood") {
		t.Fatal("nick pane should hide on narrow terminals")
	}
	// Server buffer: no pane.
	a.focus = 0
	a.bufs = []*store.Buffer{a.st.Get("znc", "znc")}
	a.resize()
	if strings.Contains(a.View(), "@thadood") {
		t.Fatal("nick pane should not show for server buffer")
	}
}
