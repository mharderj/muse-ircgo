package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"ircgo/internal/config"
	"ircgo/internal/store"
)

// linkTestApp builds an app with one channel buffer holding a single chat
// line, laid out and rendered.
func linkTestApp(t *testing.T, text string) *App {
	t.Helper()
	a := New(&config.Config{}, store.New(), nil, nil)
	a.addLine("srv", "#a", store.Line{
		At:   time.Date(2026, 9, 26, 14, 18, 0, 0, time.UTC),
		Nick: "bob",
		Text: text,
		Kind: store.KindChat,
	})
	a.bufs = a.st.Buffers()
	a.focus = 1
	a.width, a.height = 100, 30
	a.ready = true
	a.resize() // calls renderChat
	return a
}

// chatTopY is the first terminal row of the chat viewport: the topic panel
// occupies rows 0-1 for channel buffers.
const chatTopY = 2

// cellX returns the terminal x of the first cell of sub in content row 0.
func cellX(a *App, sub string) int {
	row := a.chatContentRows[0]
	i := strings.Index(row, sub)
	if i < 0 {
		panic("substring not in row: " + sub)
	}
	return 17 + lipgloss.Width(row[:i]) // 16 sidebar + 1 border
}

func TestLinkAtSingleURL(t *testing.T) {
	a := linkTestApp(t, "see https://x.test/a.png ok")
	url, ok := a.linkAt(cellX(a, "https://"), chatTopY)
	if !ok || url != "https://x.test/a.png" {
		t.Fatalf("linkAt = %q, %v; want the image URL, true", url, ok)
	}
	// Lenient: clicking elsewhere on the same row still opens the one link.
	if url, ok := a.linkAt(17, chatTopY); !ok || url != "https://x.test/a.png" {
		t.Fatalf("lenient linkAt = %q, %v", url, ok)
	}
}

func TestLinkAtNoURL(t *testing.T) {
	a := linkTestApp(t, "just chatting here")
	if url, ok := a.linkAt(20, chatTopY); ok {
		t.Fatalf("linkAt = %q, true; want no link", url)
	}
}

func TestLinkAtOutsideChatPane(t *testing.T) {
	a := linkTestApp(t, "see https://x.test/a.png ok")
	if _, ok := a.linkAt(5, chatTopY); ok { // sidebar
		t.Fatal("linkAt hit in the sidebar")
	}
	if _, ok := a.linkAt(17, 29); ok { // status/input area
		t.Fatal("linkAt hit below the chat pane")
	}
}

func TestLinkAtWrappedURL(t *testing.T) {
	long := "pic https://x.test/" + strings.Repeat("a", 80) + ".png done"
	a := linkTestApp(t, long)
	want := "https://x.test/" + strings.Repeat("a", 80) + ".png"

	// Find the display row where the wrapped URL starts.
	rows := a.visibleRows()
	dStart, rowText := -1, ""
	for i, r := range rows {
		if plain := strings.TrimRight(stripANSI(r.text), " "); strings.Contains(plain, "https://") {
			dStart, rowText = i, plain
			break
		}
	}
	if dStart < 0 {
		t.Fatal("URL not found in visible rows")
	}
	x := 17 + lipgloss.Width(rowText[:strings.Index(rowText, "https://")])

	// Clicking the URL's start opens the full (glued) URL.
	if url, ok := a.linkAt(x, chatTopY+dStart); !ok || url != want {
		t.Fatalf("linkAt wrapped start = %q, %v", url, ok)
	}
	// Clicking the continuation fragment on the next display row too.
	if url, ok := a.linkAt(17, chatTopY+dStart+1); !ok || url != want {
		t.Fatalf("linkAt wrapped continuation = %q, %v", url, ok)
	}
}

func TestLinkAtTwoURLs(t *testing.T) {
	a := linkTestApp(t, "a https://one.test/ b https://two.test/x")
	u1, ok1 := a.linkAt(cellX(a, "https://one"), chatTopY)
	u2, ok2 := a.linkAt(cellX(a, "https://two"), chatTopY)
	if !ok1 || u1 != "https://one.test/" {
		t.Fatalf("first link = %q, %v", u1, ok1)
	}
	if !ok2 || u2 != "https://two.test/x" {
		t.Fatalf("second link = %q, %v", u2, ok2)
	}
}
