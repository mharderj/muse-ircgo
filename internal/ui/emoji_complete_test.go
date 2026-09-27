package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"ircgo/internal/emoji"
)

func TestFindACTrigger(t *testing.T) {
	cases := []struct {
		in     string
		pos    int
		anchor int
		prefix string
		ok     bool
	}{
		{"hello :con", 10, 6, "con", true},
		{":joy", 4, 0, "joy", true},
		{"a :sm b", 5, 2, "sm", true},          // cursor mid-line
		{"a :sm b", 3, 0, "", false},           // cursor before the trigger
		{"https://x.test/a", 16, 0, "", false}, // URL colon is not a trigger
		{"hi :", 4, 0, "", false},              // bare colon needs a character
		{"12:30", 5, 2, "30", true},            // syntactic trigger; no matches
		{"no trigger here", 15, 0, "", false},
	}
	for _, c := range cases {
		anchor, prefix, ok := findACTrigger(c.in, c.pos)
		if ok != c.ok || anchor != c.anchor || prefix != c.prefix {
			t.Errorf("findACTrigger(%q, %d) = (%d, %q, %v); want (%d, %q, %v)",
				c.in, c.pos, anchor, prefix, ok, c.anchor, c.prefix, c.ok)
		}
	}
}

func TestSuggest(t *testing.T) {
	got := emoji.Suggest("con")
	found := false
	for i, s := range got {
		if i > 0 && got[i-1].Name >= s.Name {
			t.Fatalf("suggestions not sorted: %q before %q", got[i-1].Name, s.Name)
		}
		if s.Name == "confused" && s.Emoji == "😕" {
			found = true
		}
	}
	if !found {
		t.Fatalf("confused missing from suggestions for \"con\": %+v", got)
	}
	lower, upper := emoji.Suggest("con"), emoji.Suggest("CON")
	if len(lower) != len(upper) {
		t.Fatalf("case-insensitive mismatch: %d vs %d", len(lower), len(upper))
	}
	if len(emoji.Suggest("zzzqx")) != 0 {
		t.Fatal("expected no suggestions for zzzqx")
	}
}

func TestAutocompleteOpenAndAccept(t *testing.T) {
	a := testApp()
	a.input.SetValue("a :confused")
	a.input.SetCursor(11)
	a.refreshEmojiAC()
	if a.emojiAC == nil {
		t.Fatal("popup did not open for :confused")
	}
	if len(a.emojiAC.matches) != 1 || a.emojiAC.matches[0].Name != "confused" {
		t.Fatalf("matches = %+v", a.emojiAC.matches)
	}
	a.acceptEmojiAC()
	if got := a.input.Value(); got != "a :confused: " {
		t.Fatalf("value = %q; want %q", got, "a :confused: ")
	}
	if pos := a.input.Position(); pos != 13 {
		t.Fatalf("cursor = %d; want 13", pos)
	}
	if a.emojiAC != nil {
		t.Fatal("popup still open after accept")
	}
}

// Accepting mid-line inserts the shortcode without a trailing space and
// must not reopen the popup on the trailing colon.
func TestAutocompleteAcceptMidLine(t *testing.T) {
	a := testApp()
	a.input.SetValue("x :jo yz")
	a.input.SetCursor(5)
	a.refreshEmojiAC()
	if a.emojiAC == nil {
		t.Fatal("popup did not open for :jo")
	}
	a.acceptEmojiAC()
	a.refreshEmojiAC()
	if got := a.input.Value(); got != "x :joy: yz" {
		t.Fatalf("value = %q; want %q", got, "x :joy: yz")
	}
	if pos := a.input.Position(); pos != 7 {
		t.Fatalf("cursor = %d; want 7", pos)
	}
	if a.emojiAC != nil {
		t.Fatal("popup reopened after mid-line accept")
	}
}

func TestAutocompleteKeys(t *testing.T) {
	a := testApp()
	a.input.SetValue("hey :sm")
	a.input.SetCursor(7)
	a.refreshEmojiAC()
	if a.emojiAC == nil {
		t.Fatal("popup did not open")
	}
	n := len(a.emojiAC.matches)
	if n < 2 {
		t.Fatalf("need 2+ matches for nav test, got %d", n)
	}
	_, _ = a.Update(tea.KeyMsg{Type: tea.KeyDown})
	if a.emojiAC.sel != 1 {
		t.Fatalf("down: sel = %d; want 1", a.emojiAC.sel)
	}
	_, _ = a.Update(tea.KeyMsg{Type: tea.KeyUp})
	_, _ = a.Update(tea.KeyMsg{Type: tea.KeyUp}) // wraps to last
	if a.emojiAC.sel != n-1 {
		t.Fatalf("up wrap: sel = %d; want %d", a.emojiAC.sel, n-1)
	}
	// tab inserts the selected shortcode instead of switching buffers.
	name := a.emojiAC.matches[n-1].Name
	_, _ = a.Update(tea.KeyMsg{Type: tea.KeyTab})
	if a.emojiAC != nil {
		t.Fatal("popup still open after tab")
	}
	if want := "hey :" + name + ": "; a.input.Value() != want {
		t.Fatalf("value = %q; want %q", a.input.Value(), want)
	}
	// esc closes the popup.
	a.input.SetValue("x :sm")
	a.input.SetCursor(5)
	a.refreshEmojiAC()
	if a.emojiAC == nil {
		t.Fatal("popup did not reopen")
	}
	_, _ = a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if a.emojiAC != nil {
		t.Fatal("popup still open after esc")
	}
	// enter accepts instead of sending the message.
	a.input.SetValue(":joy")
	a.input.SetCursor(4)
	a.refreshEmojiAC()
	_, _ = a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if a.emojiAC != nil {
		t.Fatal("popup still open after enter")
	}
	if got := a.input.Value(); got != ":joy: " {
		t.Fatalf("enter sent instead of accepting: value = %q", got)
	}
}

// Typing that eliminates every match closes the popup.
func TestAutocompleteNoMatchCloses(t *testing.T) {
	a := testApp()
	a.input.SetValue("a :con")
	a.input.SetCursor(6)
	a.refreshEmojiAC()
	if a.emojiAC == nil {
		t.Fatal("popup did not open")
	}
	a.input.SetValue("a :confx")
	a.input.SetCursor(8)
	a.refreshEmojiAC()
	if a.emojiAC != nil {
		t.Fatal("popup stayed open with no matches")
	}
}

// A click dismisses the popup.
func TestAutocompleteClickDismiss(t *testing.T) {
	a := testApp()
	a.input.SetValue("a :con")
	a.input.SetCursor(6)
	a.refreshEmojiAC()
	if a.emojiAC == nil {
		t.Fatal("popup did not open")
	}
	_, _ = a.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if a.emojiAC != nil {
		t.Fatal("popup stayed open after click")
	}
}

func TestAutocompleteOverlay(t *testing.T) {
	a := linkTestApp(t, "hello")
	baseLines := len(strings.Split(stripANSI(a.View()), "\n"))
	a.input.SetValue(":con")
	a.input.SetCursor(4)
	a.refreshEmojiAC()
	if a.emojiAC == nil {
		t.Fatal("popup did not open")
	}
	plain := stripANSI(a.View())
	if lines := strings.Split(plain, "\n"); len(lines) != baseLines {
		t.Fatalf("overlay changed line count: %d vs %d", len(lines), baseLines)
	}
	for _, want := range []string{"😕", ":confused:", "╭", "╰"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("popup box missing %q in rendered view", want)
		}
	}
	// The popup sits just above the input line (second to last row).
	rows := strings.Split(plain, "\n")
	found := false
	for i := len(rows) - 2 - 5; i < len(rows)-2; i++ {
		if strings.Contains(rows[i], ":confused:") {
			found = true
		}
	}
	if !found {
		t.Fatal("popup not anchored above the input line")
	}
}

func TestACAnchorX(t *testing.T) {
	if x := acAnchorX("> ", "hey :con", 4); x != 6 {
		t.Fatalf("anchorX = %d; want 6", x)
	}
	if x := acAnchorX("> ", ":con", 0); x != 2 {
		t.Fatalf("anchorX = %d; want 2", x)
	}
}

// Typing more characters filters the match list, but the popup's geometry
// must hold steady while it stays open — no border flapping.
func TestAutocompleteGeometryStableWhileTyping(t *testing.T) {
	a := testApp()
	var rows, w int
	for i, frag := range []string{":c", ":cl", ":cla"} {
		a.input.SetValue(frag)
		a.input.SetCursor(len([]rune(frag)))
		a.refreshEmojiAC()
		if a.emojiAC == nil {
			t.Fatalf("popup closed for %q", frag)
		}
		if i == 0 {
			rows, w = a.emojiAC.boxRows, a.emojiAC.boxW
			continue
		}
		if a.emojiAC.boxRows != rows || a.emojiAC.boxW != w {
			t.Fatalf("geometry changed typing %q: rows %d->%d, w %d->%d",
				frag, rows, a.emojiAC.boxRows, w, a.emojiAC.boxW)
		}
	}
	// The rendered box must keep the same line count too.
	fake := strings.Repeat("x\n", 30)
	first := overlayEmojiAC(fake, a.emojiAC, 2, 80)
	a.input.SetValue(":cla")
	a.input.SetCursor(4)
	a.refreshEmojiAC()
	second := overlayEmojiAC(fake, a.emojiAC, 2, 80)
	if len(strings.Split(first, "\n")) != len(strings.Split(second, "\n")) {
		t.Fatal("overlay line count changed while typing")
	}
}

// VS16 (U+FE0F) must be stripped in boxed popups: it requests emoji
// presentation, which terminals render at inconsistent widths (1 or 2
// cells), breaking the popup's borders. The bare character measures the
// same in lipgloss and in the terminal.
func TestACEmojiStripsVS16(t *testing.T) {
	if got := acEmoji("☁\uFE0F"); got != "☁" {
		t.Fatalf("acEmoji did not strip VS16: %q", got)
	}
	for _, s := range emoji.Suggest("") {
		e := acEmoji(s.Emoji)
		if strings.ContainsRune(e, '\uFE0F') {
			t.Fatalf("VS16 survived stripping for %q", s.Name)
		}
		if w := lipgloss.Width(e); w != 1 && w != 2 {
			t.Fatalf("emoji %q has nondeterministic width %d after stripping", s.Name, w)
		}
	}
}

// A popup containing a VS16 emoji (e.g. :cloud:) must render every box
// line at the same width so the borders align.
func TestACPopupBoxAlignsWithVS16Emoji(t *testing.T) {
	a := testApp()
	a.input.SetValue(":clou")
	a.input.SetCursor(5)
	a.refreshEmojiAC()
	if a.emojiAC == nil {
		t.Fatal("popup did not open for :clou")
	}
	found := false
	for _, m := range a.emojiAC.matches {
		if m.Name == "cloud" {
			found = true
		}
	}
	if !found {
		t.Fatal("cloud not among matches for :clou")
	}
	fake := strings.Repeat("                                                  \n", 30)
	out := overlayEmojiAC(fake, a.emojiAC, 2, 80)
	widths := map[int]bool{}
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "╭") || strings.Contains(l, "│") || strings.Contains(l, "╰") {
			widths[lipgloss.Width(l)] = true
		}
	}
	// 50 = full screen width (box spliced onto 50-wide fake lines);
	// every line carrying box content must agree.
	if len(widths) != 1 {
		t.Fatalf("box lines have inconsistent widths: %v", widths)
	}
}
