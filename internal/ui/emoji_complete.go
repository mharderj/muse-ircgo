package ui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"ircgo/internal/emoji"
)

// maxACRows caps how many suggestions the autocomplete popup shows at once;
// longer lists scroll.
const maxACRows = 8

// emojiComplete is the :shortcode: autocomplete popup for the input line.
// It opens when the text before the cursor ends with a ":" trigger, filters
// as you type, and inserts the chosen shortcode on tab/enter.
type emojiComplete struct {
	anchor  int // rune index of the ":" trigger in the input value
	prefix  string
	matches []emoji.Suggestion
	sel     int
	scroll  int // first visible match index
	// boxRows and boxW snapshot the popup's geometry when it opens and
	// hold it steady while it stays open, so the borders don't flap
	// around as the match list filters down with each keystroke.
	boxRows int // visible rows (capped at maxACRows)
	boxW    int // content width in cells (excludes border/padding)
}

// acContentW returns the widest ":name:" row for a match list, in cells.
func acContentW(matches []emoji.Suggestion) int {
	w := 0
	for _, m := range matches {
		if cw := lipgloss.Width(acEmoji(m.Emoji) + " :" + m.Name + ":"); cw > w {
			w = cw
		}
	}
	return w
}

// acEmoji returns the emoji with VS16 (U+FE0F) stripped for use in boxed
// popups. VS16 requests emoji presentation, which terminals render at
// inconsistent widths (1 or 2 cells); the bare character has a
// deterministic width that matches lipgloss's measurement, so the popup's
// borders stay aligned.
func acEmoji(e string) string {
	return strings.ReplaceAll(e, "\uFE0F", "")
}

// acTriggerRe finds a ":" trigger at the end of the text before the cursor.
// At least one word character must follow the colon: a bare ":" never
// triggers, which also keeps the popup from reopening right after a
// completion is accepted.
var acTriggerRe = regexp.MustCompile(`:([A-Za-z0-9_+\-]+)$`)

// findACTrigger reports the trigger under the cursor: the rune index of the
// ":" and the fragment after it. It reports false when there is none.
func findACTrigger(value string, pos int) (anchor int, prefix string, ok bool) {
	runes := []rune(value)
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	before := string(runes[:pos])
	m := acTriggerRe.FindStringSubmatchIndex(before)
	if m == nil {
		return 0, "", false
	}
	anchor = len([]rune(before[:m[0]]))
	return anchor, before[m[2]:m[3]], true
}

// move shifts the selection, wrapping around and keeping it visible.
func (ac *emojiComplete) move(d int) {
	n := len(ac.matches)
	if n == 0 {
		return
	}
	ac.sel = (ac.sel + d%n + n) % n
	if ac.sel < ac.scroll {
		ac.scroll = ac.sel
	}
	if ac.sel >= ac.scroll+maxACRows {
		ac.scroll = ac.sel - maxACRows + 1
	}
}

// refreshEmojiAC recomputes the popup from the input's current value and
// cursor. It closes the popup when there is no trigger or no matches.
func (a *App) refreshEmojiAC() {
	anchor, prefix, ok := findACTrigger(a.input.Value(), a.input.Position())
	if !ok {
		a.emojiAC = nil
		return
	}
	matches := emoji.Suggest(prefix)
	if len(matches) == 0 {
		a.emojiAC = nil
		return
	}
	if a.emojiAC != nil && a.emojiAC.anchor == anchor && a.emojiAC.prefix == prefix {
		a.emojiAC.matches = matches
		if a.emojiAC.sel >= len(matches) {
			a.emojiAC.sel = len(matches) - 1
		}
		return
	}
	if a.emojiAC != nil && a.emojiAC.anchor == anchor {
		// Same trigger, new prefix: update in place so the popup's
		// geometry holds steady. It may only grow (backspacing widens
		// the match set), never shrink — no border flapping.
		ac := a.emojiAC
		ac.prefix = prefix
		ac.matches = matches
		ac.sel = 0
		ac.scroll = 0
		if w := acContentW(matches); w > ac.boxW {
			ac.boxW = w
		}
		if r := min(len(matches), maxACRows); r > ac.boxRows {
			ac.boxRows = r
		}
		return
	}
	a.emojiAC = &emojiComplete{
		anchor:  anchor,
		prefix:  prefix,
		matches: matches,
		boxRows: min(len(matches), maxACRows),
		boxW:    acContentW(matches),
	}
}

// acceptEmojiAC inserts the selected shortcode over the trigger fragment. A
// trailing space is added when the cursor is at the end of the input so the
// user can keep typing.
func (a *App) acceptEmojiAC() {
	ac := a.emojiAC
	a.emojiAC = nil
	if ac == nil || ac.sel < 0 || ac.sel >= len(ac.matches) {
		return
	}
	name := ac.matches[ac.sel].Name
	runes := []rune(a.input.Value())
	pos := a.input.Position()
	if pos > len(runes) {
		pos = len(runes)
	}
	ins := ":" + name + ":"
	atEnd := pos == len(runes)
	if atEnd {
		ins += " "
	}
	a.input.SetValue(string(runes[:ac.anchor]) + ins + string(runes[pos:]))
	newPos := ac.anchor + len([]rune(":"+name+":"))
	if atEnd {
		newPos++ // the trailing space
	}
	a.input.SetCursor(newPos)
}

var acSelStyle = lipgloss.NewStyle().Reverse(true)

// overlayEmojiAC draws the autocomplete popup onto the rendered screen,
// anchored at the ":" trigger's cell column just above the input line. It
// splices over existing lines so the screen height — and mouse coordinates
// — stay exactly as rendered.
func overlayEmojiAC(screen string, ac *emojiComplete, width, divX int) string {
	// The popup keeps its opening geometry: render boxRows rows even when
	// fewer matches remain, padding the shortfall with blank rows so the
	// borders never move while the popup is open.
	rows := ac.boxRows
	contentW := ac.boxW
	lines := make([]string, 0, rows)
	for i := 0; i < rows && ac.scroll+i < len(ac.matches); i++ {
		m := ac.matches[ac.scroll+i]
		lines = append(lines, acEmoji(m.Emoji)+" :"+m.Name+":")
	}
	for len(lines) < rows {
		lines = append(lines, "")
	}
	boxW := contentW + 4 // border + padding
	boxH := rows + 2
	scrLines := strings.Split(screen, "\n")
	height := len(scrLines)
	if boxW > width || boxH > height || boxH < 3 {
		return screen
	}
	// Pad rows by hand and highlight the selection after padding. The box
	// gets no Width(): lipgloss v1.1.0 mismeasures ANSI-wrapped rows with
	// wide emoji and wraps them mid-row.
	for i := range lines {
		if pad := contentW - lipgloss.Width(lines[i]); pad > 0 {
			lines[i] += strings.Repeat(" ", pad)
		}
		if ac.scroll+i == ac.sel {
			lines[i] = acSelStyle.Render(lines[i])
		}
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))
	boxLines := strings.Split(box, "\n")
	// The popup is pinned just right of the sidebar divider: a fixed spot
	// above the input that never moves and never covers a divider.
	x0 := divX + 1
	if x0+boxW > width {
		x0 = width - boxW
	}
	if x0 < 0 {
		x0 = 0
	}
	inputIdx := height - 2 // input line sits above the status line
	y0 := inputIdx - boxH
	if y0 < 0 {
		y0 = 0
	}
	for i, bl := range boxLines {
		if y0+i < 0 || y0+i >= len(scrLines) {
			continue
		}
		scrLines[y0+i] = spliceCells(scrLines[y0+i], x0, bl)
	}
	return strings.Join(scrLines, "\n")
}

