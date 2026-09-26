package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"

	"ircgo/internal/store"
)

func TestVisibleWidth(t *testing.T) {
	if got := visibleWidth("\x1b[31mhi\x1b[0m"); got != 2 {
		t.Fatalf("visibleWidth(styled) = %d, want 2", got)
	}
	if got := visibleWidth("🤣"); got != 2 {
		t.Fatalf("visibleWidth(emoji) = %d, want 2", got)
	}
}

func TestWrapANSIPlain(t *testing.T) {
	got := wrapANSI("aa bb cc dd", 5, "  ")
	want := []string{"aa bb", "  cc", "  dd"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWrapANSICarriesStyleAcrossBreak(t *testing.T) {
	got := wrapANSI("\x1b[1mbold words here\x1b[0m", 10, "  ")
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2: %q", len(got), got)
	}
	// First row closes the bold before breaking...
	if !strings.HasSuffix(got[0], "\x1b[0m") {
		t.Fatalf("row 0 doesn't reset style: %q", got[0])
	}
	// ...and the continuation re-opens it.
	if !strings.HasPrefix(got[1], "  \x1b[1m") {
		t.Fatalf("row 1 doesn't reopen style: %q", got[1])
	}
	// Visible text survives intact on both rows.
	if visibleWidth(got[0]) != 10 || visibleWidth(got[1]) != 6 {
		t.Fatalf("widths = %d, %d; rows: %q", visibleWidth(got[0]), visibleWidth(got[1]), got)
	}
}

func TestWrapANSIDoesNotSplitLongWords(t *testing.T) {
	url := "https://example.com/very-long-url-that-exceeds-width"
	got := wrapANSI("see "+url, 10, "  ")
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2: %q", len(got), got)
	}
	if !strings.HasSuffix(got[1], url) {
		t.Fatalf("URL was split across rows: %q", got)
	}
}

func TestWrapANSIWideRunes(t *testing.T) {
	got := wrapANSI("🤣 🤣 🤣", 5, "")
	want := []string{"🤣 🤣", "🤣"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRenderChatWrapsLongLines(t *testing.T) {
	a := kickTestApp()
	a.cfg.UI.TimestampFormat = "15:04" // production default
	a.st.Get("srv", "srv")
	a.st.Get("srv", "#c")
	a.refreshBuffers()
	a.focus = 1 // #c
	a.addLine("srv", "#c", store.Line{Nick: "bob", Text: strings.Repeat("word ", 30), Kind: store.KindChat})
	a.chat = viewport.New(50, 20)
	a.renderChat()

	if len(a.chatContentRows) < 3 {
		t.Fatalf("expected wrapped rows, got %d", len(a.chatContentRows))
	}
	for i, r := range a.chatContentRows {
		if strings.TrimSpace(r) == "" {
			continue
		}
		if w := visibleWidth(r); w > 50 {
			t.Fatalf("row %d width %d > 50: %q", i, w, r)
		}
	}
	// Continuation rows hang under the message: ts(5) + space + nick(14) +
	// space = 21 cells of indent.
	if !strings.HasPrefix(a.chatContentRows[1], strings.Repeat(" ", 21)) {
		t.Fatalf("continuation not indented: %q", a.chatContentRows[1])
	}
}

func TestRenderChatLeavesArtUnwrapped(t *testing.T) {
	a := kickTestApp()
	a.st.Get("srv", "srv")
	a.st.Get("srv", "#c")
	a.refreshBuffers()
	a.focus = 1
	art := strings.Repeat("▄", 40) + "\n" + strings.Repeat("▀", 40)
	a.addLine("srv", "#c", store.Line{Nick: "bob", Text: "see https://x.test/pic.png", Kind: store.KindChat})
	lines := a.st.Get("srv", "#c").Lines()
	a.st.Get("srv", "#c").SetImageArt(lines[len(lines)-1].Seq, 0, art)
	a.chat = viewport.New(50, 20)
	a.renderChat()
	found := 0
	for _, r := range a.chatContentRows {
		if strings.Contains(r, "▄") || strings.Contains(r, "▀") {
			found++
			if strings.Count(r, "▄")+strings.Count(r, "▀") != 40 {
				t.Fatalf("art row was wrapped or altered: %q", r)
			}
		}
	}
	if found != 2 {
		t.Fatalf("found %d art rows, want 2", found)
	}
}
