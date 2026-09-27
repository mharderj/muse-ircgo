package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// The bridge's :shortcode: text must render as the emoji in chat, while
// shortcode-looking URL paths stay untouched.
func TestChatShortcodeReplaced(t *testing.T) {
	a := linkTestApp(t, "sounds like a no :confused:")
	row := a.chatContentRows[0]
	if !strings.Contains(row, "😕") {
		t.Fatalf("shortcode not replaced in chat row: %q", row)
	}
	if strings.Contains(row, ":confused:") {
		t.Fatalf("shortcode text lingered in chat row: %q", row)
	}
}

func TestChatShortcodeInURLUntouched(t *testing.T) {
	a := linkTestApp(t, "see https://x.test/:smile:/a.png ok")
	row := a.chatContentRows[0]
	if strings.Contains(row, "😄") {
		t.Fatalf("shortcode inside URL was replaced: %q", row)
	}
	if !strings.Contains(row, "https://x.test/:smile:/a.png") {
		t.Fatalf("URL mangled: %q", row)
	}
}

// Hovering a translated emoji sets the preview popup; the rendered view
// then carries the popup box with the :shortcode:.
func TestEmojiHoverPopup(t *testing.T) {
	a := linkTestApp(t, "sounds like a no :confused:")
	x := cellX(a, "😕")
	_, _ = a.Update(tea.MouseMsg{X: x, Y: chatTopY, Action: tea.MouseActionMotion})
	if a.emojiTip == nil {
		t.Fatal("no emoji popup on hover")
	}
	if a.emojiTip.emoji != "😕" || a.emojiTip.shortcode != "confused" {
		t.Fatalf("popup = %+v; want 😕 / confused", a.emojiTip)
	}
	v := stripANSI(a.View())
	if !strings.Contains(v, ":confused:") {
		t.Fatal("popup box missing from rendered view")
	}
}

// Moving the mouse off the emoji dismisses the popup.
func TestEmojiHoverDismiss(t *testing.T) {
	a := linkTestApp(t, "sounds like a no :confused:")
	_, _ = a.Update(tea.MouseMsg{X: cellX(a, "😕"), Y: chatTopY, Action: tea.MouseActionMotion})
	if a.emojiTip == nil {
		t.Fatal("no emoji popup on hover")
	}
	_, _ = a.Update(tea.MouseMsg{X: cellX(a, "sounds"), Y: chatTopY, Action: tea.MouseActionMotion})
	if a.emojiTip != nil {
		t.Fatal("popup lingered after moving off the emoji")
	}
}

// A keypress dismisses the popup.
func TestEmojiPopupKeyDismiss(t *testing.T) {
	a := linkTestApp(t, "sounds like a no :confused:")
	_, _ = a.Update(tea.MouseMsg{X: cellX(a, "😕"), Y: chatTopY, Action: tea.MouseActionMotion})
	if a.emojiTip == nil {
		t.Fatal("no emoji popup on hover")
	}
	_, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if a.emojiTip != nil {
		t.Fatal("popup lingered after keypress")
	}
}

// Hovering plain text (no emoji) sets no popup.
func TestEmojiHoverNoEmoji(t *testing.T) {
	a := linkTestApp(t, "just chatting here")
	_, _ = a.Update(tea.MouseMsg{X: cellX(a, "chatting"), Y: chatTopY, Action: tea.MouseActionMotion})
	if a.emojiTip != nil {
		t.Fatalf("unexpected popup: %+v", a.emojiTip)
	}
}

// spliceCells replaces by cell without splitting ANSI escapes or wide
// runes at the cut edges.
func TestSpliceCells(t *testing.T) {
	if got := spliceCells("hello world", 6, "XX"); got != "hello XXrld" {
		t.Fatalf("plain splice = %q", got)
	}
	got := spliceCells("\x1b[31mhello\x1b[0m world", 0, "YY")
	if stripANSI(got) != "YYllo world" {
		t.Fatalf("ANSI splice = %q", stripANSI(got))
	}
	// A cut that would split the wide 😕 swallows it whole instead.
	got = spliceCells("a 😕 b", 2, "ZZ")
	if stripANSI(got) != "a ZZ b" {
		t.Fatalf("wide-rune splice = %q", stripANSI(got))
	}
}
