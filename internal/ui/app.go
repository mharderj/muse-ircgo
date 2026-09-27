// Package ui is the Bubble Tea interface: a sidebar of buffers, a chat
// viewport, an input line, and a status bar.
package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"ircgo/internal/config"
	"ircgo/internal/history"
	"ircgo/internal/img"
	"ircgo/internal/irc"
	"ircgo/internal/link"
	"ircgo/internal/store"
	"ircgo/internal/version"
)

// ircEventMsg carries a protocol event into the Update loop.
type ircEventMsg struct{ ev irc.Event }

// eventsClosedMsg fires when the event channel is closed.
type eventsClosedMsg struct{}

// imageFetchedMsg arrives when a background image fetch finishes; art is
// empty when the fetch failed.
type imageFetchedMsg struct {
	url string
	art string
}

// artSlot identifies one image-URL slot on one message line: the preview
// belongs directly under that message, not at the end of the buffer.
type artSlot struct {
	server, buf string
	seq         uint64
	slot        int
}

// waitForEvents returns a Cmd that yields the next IRC event.
func waitForEvents(ch <-chan irc.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return eventsClosedMsg{}
		}
		return ircEventMsg{ev: ev}
	}
}

var (
	tsStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	nickStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	sysStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true)
	sidebarStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, true, true, false).
			BorderForeground(lipgloss.Color("8"))
	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7")).
			Background(lipgloss.Color("4"))
	nicksStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, true, true).
			BorderForeground(lipgloss.Color("8"))
	chatBottomStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(lipgloss.Color("8"))
	linkStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("12")).
			Underline(true)
	// unreadStyle marks sidebar buffers with unseen activity.
	unreadStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("11"))
	// dragStyle dims the sidebar row being dragged; dropStyle highlights
	// the current drop target.
	dragStyle = lipgloss.NewStyle().Faint(true)
	dropStyle = lipgloss.NewStyle().Reverse(true)
	// hoverStyle highlights the sidebar row under the mouse. It is
	// deliberately subtler than the focused row's reverse video so the
	// two states are never confused.
	hoverStyle = lipgloss.NewStyle().Background(lipgloss.Color("237"))
	topicStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(lipgloss.Color("8"))
)

// topicBarHeight is the topic panel height: one text row plus a divider.
const topicBarHeight = 2

// App is the root Bubble Tea model.
type App struct {
	cfg     *config.Config
	st      *store.Store
	events  <-chan irc.Event
	clients map[string]*irc.Client

	// ConfigPath is the path to config.toml, used to persist last_buffer
	// on quit. Empty means don't persist (e.g. in tests).
	ConfigPath string

	bufs  []*store.Buffer
	focus int

	members map[string]*memberSet

	// nicks tracks our current nick per server, starting from config and
	// updated from 001 and NICK echoes (including "_" 433 fallbacks).
	ownNicks map[string]string

	// Image previews: fetched art by URL, URLs currently fetching, and the
	// message-line slots waiting on each in-flight fetch.
	imgCache    map[string]string
	imgInflight map[string]bool
	imgPending  map[string][]artSlot

	sidebar viewport.Model
	chat    viewport.Model
	nicks   viewport.Model
	input   textinput.Model

	// chatContentRows mirrors the chat viewport's content lines with ANSI
	// stripped, index-aligned with them, so clicks can be mapped to URLs.
	chatContentRows []string

	// pendingFocus is the buffer to focus once it appears, used by the
	// startup restore: channels don't exist until JOIN creates them. A
	// manual buffer switch clears it.
	pendingFocus lastFocus

	// hovered is the sidebar row under the mouse (-1 when the mouse isn't
	// over a buffer), used for the hover close/recover affordances.
	hovered int

	// archived parks closed buffers under the "-- Archive --" divider;
	// hidden removes archived buffers from view entirely. Both are keyed
	// by memberKey(server, name) and are session-only. New activity in a
	// parked or hidden buffer returns it to the sidebar.
	archived map[string]bool
	hidden   map[string]bool

	// partedOnPark tracks buffers drag-parked with an auto-PART, keyed by
	// memberKey; recovering one re-JOINs the channel.
	partedOnPark map[string]bool

	// order is the persisted sidebar order: buffer keys (memberKey) in
	// display sequence. refreshBuffers sorts each (server, rank) group by
	// position in this list; buffers absent from it go last. It is
	// written to buffer_order in the config whenever the user reorders,
	// parks or recovers a buffer, and restored on startup.
	order    []string
	orderIdx map[string]int

	// Drag-to-reorder state. pressIdx/pressY/pressAfford capture a left
	// press on a sidebar row; motion with the button held past a
	// threshold activates the drag (dragIdx). dropKind/dropIdx describe
	// the current drop target: "reorder" onto a buffer row, "archive"
	// onto the archive zone (auto-parts channels), "recover" out of the
	// archive back into a group.
	pressIdx    int
	pressY      int
	pressAfford string
	dragIdx     int
	dragActive  bool
	dragKey     string
	dropKind    string
	dropIdx     int
	dropRow     int

	// dropRows maps sidebar content rows to drop zones, rebuilt by
	// renderSidebar while a drag is active: the archive divider (park a
	// buffer) and the recover hint (unpark into an empty group).
	dropRows map[int]dropTarget
	// archivePad is the number of blank lines renderSidebar inserted
	// above the bottom-pinned archive section; sidebarBufferAt walks the
	// same layout, so both stay in sync.
	archivePad int

	width  int
	height int
	ready  bool
}

// dropTarget is a non-buffer drop zone in the sidebar.
type dropTarget struct {
	server string
	kind   string // "archive" or "recover"
}

// lastFocus identifies a buffer to focus.
type lastFocus struct {
	server, name string
}

// New builds the root model. Event flow: the IRC clients publish to events,
// Init arms waitForEvents, and each received event re-arms it.
func New(cfg *config.Config, st *store.Store, events <-chan irc.Event, clients map[string]*irc.Client) *App {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.CharLimit = 440
	ti.Focus()
	a := &App{
		cfg: cfg, st: st, events: events, clients: clients,
		input:        ti,
		members:      map[string]*memberSet{},
		ownNicks:     map[string]string{},
		imgCache:     map[string]string{},
		imgInflight:  map[string]bool{},
		imgPending:   map[string][]artSlot{},
		hovered:      -1,
		archived:     map[string]bool{},
		hidden:       map[string]bool{},
		partedOnPark: map[string]bool{},
		orderIdx:     map[string]int{},
		pressIdx:     -1,
		dragIdx:      -1,
		dropIdx:      -1,
		dropRow:      -1,
		dropRows:     map[int]dropTarget{},
	}
	a.loadBufferOrder()
	return a
}

// resetDrag clears pending press/drag state without rendering.
func (a *App) resetDrag() {
	a.pressIdx = -1
	a.pressY = 0
	a.pressAfford = ""
	a.dragIdx = -1
	a.dragActive = false
	a.dragKey = ""
	a.dropKind = ""
	a.dropIdx = -1
	a.dropRow = -1
}

// dragBuffer returns the buffer being dragged, or nil.
func (a *App) dragBuffer() *store.Buffer {
	if !a.dragActive || a.dragIdx < 0 || a.dragIdx >= len(a.bufs) {
		return nil
	}
	return a.bufs[a.dragIdx]
}

// contentRowAt maps a mouse position to a sidebar content row, accounting
// for viewport scroll.
func (a *App) contentRowAt(x, y int) (int, bool) {
	sw := a.sidebarWidth()
	if x < 0 || x > sw || y < 0 {
		return 0, false
	}
	return y + a.sidebar.YOffset, true
}

// updateDrag advances an in-progress sidebar drag: it arms the drag once
// the pointer leaves the press row, then resolves the drop target under
// the cursor — a buffer row (reorder within a group, park onto the
// archive, recover out of it) or a drop-zone divider row.
func (a *App) updateDrag(x, y int) {
	if a.pressIdx < 0 || a.pressIdx >= len(a.bufs) {
		return
	}
	d := a.bufs[a.pressIdx]
	if bufferRank(d) == 0 || a.pressAfford != "" {
		return // the server window and affordance clicks aren't draggable
	}
	if !a.dragActive {
		if i, ok := a.sidebarBufferAt(x, y); ok && i == a.pressIdx {
			return // still on the press row: plain click so far
		}
		a.dragActive = true
		a.dragIdx = a.pressIdx
		a.dragKey = memberKey(d.Server, d.Name)
	} else if a.dragKey != "" {
		// The sidebar may have shifted under an active drag (new
		// buffers, joins/parts); track the dragged buffer by key so
		// the highlight and drop stay on the right row.
		a.dragIdx = -1
		for i, b := range a.bufs {
			if memberKey(b.Server, b.Name) == a.dragKey {
				a.dragIdx = i
				break
			}
		}
	}
	a.dropKind, a.dropIdx, a.dropRow = a.resolveDrop(x, y)
	a.renderSidebar()
}

// resolveDrop computes the drop target under (x, y) for the active drag:
// "reorder" onto a buffer row in the same group, "archive" onto the
// archive zone (auto-parts channels), "recover" out of the archive.
// idx is the target buffer row (-1 for divider-zone drops, where row is
// the content row instead).
func (a *App) resolveDrop(x, y int) (kind string, idx, row int) {
	d := a.dragBuffer()
	if d == nil {
		return "", -1, -1
	}
	dk := memberKey(d.Server, d.Name)
	archivedDrag := a.archived[dk]
	if i, ok := a.sidebarBufferAt(x, y); ok {
		t := a.bufs[i]
		if t.Server == d.Server && i != a.dragIdx {
			dr, tr := a.displayRank(d), a.displayRank(t)
			switch {
			case dr == tr:
				return "reorder", i, -1
			case !archivedDrag && tr == 3:
				return "archive", i, -1
			case archivedDrag && tr != 3:
				return "recover", i, -1
			}
		}
		return "", -1, -1
	}
	if r, ok := a.contentRowAt(x, y); ok {
		if dt, ok := a.dropRows[r]; ok && dt.server == d.Server {
			switch {
			case dt.kind == "archive" && !archivedDrag:
				return "archive", -1, r
			case dt.kind == "recover" && archivedDrag:
				return "recover", -1, r
			}
		}
	}
	return "", -1, -1
}

// finishDrag applies the drop target under the release point. The dragged
// buffer is re-resolved by key and the target recomputed: IRC events may
// have shifted the sidebar mid-drag.
func (a *App) finishDrag(x, y int) {
	a.dragIdx = -1
	for i, b := range a.bufs {
		if memberKey(b.Server, b.Name) == a.dragKey {
			a.dragIdx = i
			break
		}
	}
	d := a.dragBuffer()
	if d == nil {
		return
	}
	kind, idx, _ := a.resolveDrop(x, y)
	dk := a.dragKey
	switch kind {
	case "reorder":
		if idx == a.dragIdx {
			return
		}
		t := a.bufs[idx]
		tk := memberKey(t.Server, t.Name)
		seq := a.displayKeySeq()
		// The dragged buffer takes the target's slot in the display
		// order, so dragging down onto the next row actually moves it
		// down (insert-before would be a no-op there).
		slot := 0
		for i, k := range seq {
			if k == tk {
				slot = i
				break
			}
		}
		rest := make([]string, 0, len(seq))
		for _, k := range seq {
			if k != dk {
				rest = append(rest, k)
			}
		}
		if slot > len(rest) {
			slot = len(rest)
		}
		res := append(append([]string{}, rest[:slot]...), dk)
		res = append(res, rest[slot:]...)
		a.setOrder(res)
		a.refreshBuffers()
		a.writeBufferOrder()
	case "archive":
		if isChannel(d.Name) {
			if cl, ok := a.clients[d.Server]; ok {
				_ = cl.Send("PART " + d.Name)
			}
			a.partedOnPark[dk] = true
		}
		a.closeBuffer(a.dragIdx) // parks the buffer
	case "recover":
		a.recoverBuffer(a.dragIdx) // unparks; re-JOINs if we parted it
	}
}

// displayKeySeq returns the sidebar's current buffer keys in display order.
func (a *App) displayKeySeq() []string {
	seq := make([]string, 0, len(a.bufs))
	for _, b := range a.bufs {
		seq = append(seq, memberKey(b.Server, b.Name))
	}
	return seq
}

// carryOrder appends previously ordered keys that aren't in seq (hidden or
// gone buffers), preserving their relative order so they keep their slots
// if they return.
func (a *App) carryOrder(seq []string) []string {
	seen := make(map[string]bool, len(seq))
	for _, k := range seq {
		seen[k] = true
	}
	out := append([]string{}, seq...)
	for _, k := range a.order {
		if !seen[k] {
			out = append(out, k)
		}
	}
	return out
}

// trackOrder appends any visible buffer missing from the order list, in
// display order. It reports whether the list changed.
func (a *App) trackOrder() bool {
	changed := false
	for _, b := range a.bufs {
		k := memberKey(b.Server, b.Name)
		if _, ok := a.orderIdx[k]; !ok {
			a.order = append(a.order, k)
			a.orderIdx[k] = len(a.order) - 1
			changed = true
		}
	}
	return changed
}

// setOrder replaces the persisted order with seq (display order), carrying
// absent keys, and re-indexes it.
func (a *App) setOrder(seq []string) {
	a.order = a.carryOrder(seq)
	a.rebuildOrderIdx()
}

// writeBufferOrder writes the current order list to buffer_order in the
// config file. It is a no-op when no config path is known (tests).
func (a *App) writeBufferOrder() {
	if a.ConfigPath == "" {
		return
	}
	var pairs [][2]string
	for _, k := range a.order {
		if i := strings.IndexByte(k, 0); i >= 0 {
			pairs = append(pairs, [2]string{k[:i], k[i+1:]})
		}
	}
	_ = config.WriteBufferOrder(a.ConfigPath, pairs)
}

// loadBufferOrder restores the persisted sidebar order from the config.
func (a *App) loadBufferOrder() {
	for _, p := range a.cfg.BufferOrder {
		if len(p) == 2 {
			a.order = append(a.order, memberKey(p[0], p[1]))
		}
	}
	a.rebuildOrderIdx()
}

// rebuildOrderIdx re-indexes the order list for the refreshBuffers sort.
func (a *App) rebuildOrderIdx() {
	a.orderIdx = make(map[string]int, len(a.order))
	for i, k := range a.order {
		if _, ok := a.orderIdx[k]; !ok {
			a.orderIdx[k] = i
		}
	}
}

// orderSeq is a buffer's position in the persisted order; unknown buffers
// sort after all known ones.
func (a *App) orderSeq(b *store.Buffer) int {
	if i, ok := a.orderIdx[memberKey(b.Server, b.Name)]; ok {
		return i
	}
	return 1 << 30
}

func (a *App) Init() tea.Cmd {
	return tea.Batch(waitForEvents(a.events), textinput.Blink)
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.QuitMsg:
		// Remember the focused buffer for the next launch. Focus switches
		// already persist it (see setFocus); this final write covers focus
		// changes that bypass setFocus, e.g. a buffer removed under us.
		a.saveLastBuffer()
		return a, tea.Quit
	case tea.WindowSizeMsg:
		first := !a.ready
		a.width, a.height = msg.Width, msg.Height
		a.ready = true
		a.resize()
		if !first {
			return a, nil
		}
		// First layout: the chat width is now real, so restored buffers
		// regenerate image previews at the right size.
		cmd := a.restoreQueryBuffers()
		a.restoreLastFocus()
		a.renderSidebar()
		a.renderChat()
		return a, cmd
	case eventsClosedMsg:
		return a, tea.Quit
	case ircEventMsg:
		return a, tea.Batch(a.handleEvent(msg.ev), waitForEvents(a.events))
	case imageFetchedMsg:
		a.handleImageFetched(msg)
		return a, nil
	case tea.MouseMsg:
		// Mouse motion with the left button held is a sidebar drag; free
		// motion tracks the hovered row for the close/recover affordances.
		// All other mouse input is swallowed so clicks don't land in the
		// input line.
		if msg.Action == tea.MouseActionMotion {
			if msg.Button == tea.MouseButtonLeft && a.pressIdx >= 0 {
				a.updateDrag(msg.X, msg.Y)
				return a, nil
			}
			if msg.Button == tea.MouseButtonNone {
				if i, ok := a.sidebarBufferAt(msg.X, msg.Y); ok {
					if a.hovered != i {
						a.hovered = i
						a.renderSidebar()
					}
				} else if a.hovered != -1 {
					a.hovered = -1
					a.renderSidebar()
				}
			}
			return a, nil
		}
		// A press over a sidebar row pins the hover state there and arms a
		// potential drag. Terminals that don't report free mouse motion
		// still send press/release, so this keeps the close affordance
		// reachable everywhere.
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			a.resetDrag()
			if i, ok := a.sidebarBufferAt(msg.X, msg.Y); ok {
				if a.hovered != i {
					a.hovered = i
					a.renderSidebar()
				}
				a.pressIdx = i
				a.pressY = msg.Y
				a.pressAfford = a.sidebarAffordance(i, msg.X)
			} else if a.hovered != -1 {
				a.hovered = -1
				a.renderSidebar()
			}
			return a, nil
		}
		// Left-click release: finish a drag, fire a pressed affordance, or
		// switch to the clicked buffer / open the clicked chat link.
		if msg.Action == tea.MouseActionRelease && msg.Button == tea.MouseButtonLeft {
			if a.dragActive {
				a.finishDrag(msg.X, msg.Y)
				a.resetDrag()
				a.renderSidebar()
				return a, nil
			}
			if a.pressAfford != "" {
				if i, ok := a.sidebarBufferAt(msg.X, msg.Y); ok && a.sidebarAffordance(i, msg.X) == a.pressAfford {
					switch a.pressAfford {
					case "close":
						a.closeBuffer(i)
					case "recover":
						a.recoverBuffer(i)
					}
					a.resetDrag()
					return a, nil
				}
				// Pressed an affordance but released elsewhere: fall
				// through to the normal click handling below.
			}
			if i, ok := a.sidebarBufferAt(msg.X, msg.Y); ok {
				switch a.sidebarAffordance(i, msg.X) {
				case "close":
					a.closeBuffer(i)
				case "recover":
					a.recoverBuffer(i)
				default:
					if i != a.focus {
						a.focusBuffer(i)
					}
				}
			} else if url, ok := a.linkAt(msg.X, msg.Y); ok {
				openURL(url)
			}
			a.resetDrag()
		}
		return a, nil
	case tea.KeyMsg:
		// A keypress abandons any in-progress drag.
		if a.dragActive {
			a.resetDrag()
			a.renderSidebar()
		}
		// alt+1..alt+9 jumps straight to the buffer in that sidebar
		// position (top to bottom). Note: ctrl+digit would be the more
		// familiar binding, but terminals don't transmit it in a form
		// this Bubble Tea version can decode, so it can never arrive.
		if s := msg.String(); len(s) == 5 && strings.HasPrefix(s, "alt+") {
			if d := s[4]; d >= '1' && d <= '9' {
				if i := int(d - '1'); i < len(a.bufs) && i != a.focus {
					a.focusBuffer(i)
				}
				return a, nil
			}
		}
		switch msg.String() {
		case "tab":
			if len(a.bufs) > 0 {
				a.focusBuffer((a.focus + 1) % len(a.bufs))
			}
			return a, nil
		case "enter":
			v := strings.TrimSpace(a.input.Value())
			a.input.SetValue("")
			if v != "" {
				return a, a.sendInput(v)
			}
			return a, nil
		}
	}
	var cmd tea.Cmd
	a.input, cmd = a.input.Update(msg)
	return a, cmd
}

// handleEvent files a protocol event into scrollback and re-renders,
// returning any follow-up command (e.g. image preview fetches).
func (a *App) handleEvent(ev irc.Event) tea.Cmd {
	var cmd tea.Cmd
	switch ev.Kind {
	case irc.KindConnected:
		cmd = a.addLine(ev.Server, ev.Server, store.Line{At: time.Now(), Text: "connected", Kind: store.KindSystem})
	case irc.KindDisconnected:
		cmd = a.addLine(ev.Server, ev.Server, store.Line{At: time.Now(), Text: "disconnected", Kind: store.KindSystem})
	case irc.KindError:
		cmd = a.addLine(ev.Server, ev.Server, store.Line{At: time.Now(), Text: "error: " + ev.Text, Kind: store.KindSystem})
	case irc.KindMessage:
		cmd = a.handleMessage(ev.Server, ev.Msg)
	}
	a.refreshBuffers()
	a.renderSidebar()
	if a.ready {
		a.renderChat()
		a.renderNicks()
	}
	return cmd
}

// addLine files a line into a buffer, creating the buffer on first use. When
// the buffer is new, the tail of its on-disk log is replayed first (so a
// reopened query shows yesterday's conversation), and image previews for
// URLs in the replayed lines are re-fetched, since art is never logged.
// It returns any follow-up command (image preview fetches).
func (a *App) addLine(server, target string, l store.Line) tea.Cmd {
	if target == "" {
		// Never create a nameless buffer (e.g. a PART with no channel
		// param); file it in the server window instead.
		target = server
	}
	// IRC names are case-insensitive: file "Belial" under the existing
	// "belial" buffer instead of opening a second one.
	target = a.st.Resolve(server, target)
	// New activity returns a parked or hidden buffer to the sidebar.
	key := memberKey(server, target)
	if a.archived[key] || a.hidden[key] {
		delete(a.archived, key)
		delete(a.hidden, key)
		a.refreshBuffers()
		a.renderSidebar()
	}
	isNew := !a.st.Has(server, target)
	buf := a.st.Get(server, target)
	var cmd tea.Cmd
	if isNew {
		cmd = a.playbackHistory(server, target, buf)
	}
	seq := buf.Append(l)
	if text, ok := history.FormatLine(l); ok {
		history.AppendLine(server, target, text)
	}
	// Queue image previews for the line itself, attaching them to it when
	// they arrive (see artSlot): live and replayed lines share this path.
	switch l.Kind {
	case store.KindChat, store.KindAction, store.KindNotice:
		if q := a.queueImageFetches(server, target, seq, l.Text); q != nil {
			cmd = tea.Batch(cmd, q)
		}
	}
	return cmd
}

// playbackHistory replays the tail of the buffer's local log into a newly
// created buffer, oldest first. Played-back lines are already in the log,
// so they are appended directly without re-logging. It returns fetch
// commands that regenerate image previews for URLs in the replayed lines:
// art is never logged, so without this a reopened buffer would show bare
// URLs where previews used to be.
func (a *App) playbackHistory(server, target string, buf *store.Buffer) tea.Cmd {
	for _, raw := range history.LastLines(server, target, a.cfg.HistoryLines()) {
		if l, ok := history.ParseLine(raw); ok {
			buf.Append(l)
		}
	}
	var cmds []tea.Cmd
	for _, l := range buf.Lines() {
		switch l.Kind {
		case store.KindChat, store.KindAction, store.KindNotice:
			cmds = append(cmds, a.queueImageFetches(server, target, l.Seq, l.Text))
		}
	}
	return tea.Batch(cmds...)
}

func (a *App) handleMessage(server string, m *irc.Message) tea.Cmd {
	// Prefer the server-time tag (bouncer backlog) over wall-clock time,
	// then display in local time: server-time arrives as UTC.
	at := m.Time()
	if at.IsZero() {
		at = time.Now()
	}
	at = at.Local()
	nick := m.Nick()

	// addLine can return fetch commands (image previews for replayed
	// history when a buffer is first created); every branch below must
	// propagate them instead of dropping them.
	var cmd tea.Cmd

	switch m.Command {
	case "PRIVMSG":
		if len(m.Params) < 2 {
			return nil
		}
		target, text := m.Params[0], m.Params[1]
		buf := target
		if !isChannel(target) {
			// Direct message: file under the other party's nick.
			buf = nick
			if buf == "" {
				buf = server
			}
		}
		kind := store.KindChat
		if cmd, args, ok := irc.ParseCTCP(text); ok {
			switch cmd {
			case "ACTION":
				kind = store.KindAction
				text = args
			default:
				// CTCP query: show it, and answer direct (never
				// channel) queries with a NOTICE, per the CTCP spec.
				// Unknown commands get no reply.
				text = "CTCP " + cmd
				if args != "" {
					text += " " + args
				}
				kind = store.KindNotice
				if !isChannel(target) && nick != "" && nick != a.ownNick(server) {
					if reply := ctcpReply(cmd, args); reply != "" {
						if cl, ok := a.clients[server]; ok {
							_ = cl.Send("NOTICE " + nick + " :\x01" + reply + "\x01")
						}
					}
				}
			}
		}
		addCmd := a.addLine(server, buf, store.Line{At: at, Nick: nick, Text: text, Kind: kind})
		a.bumpUnread(server, buf, nick)
		return addCmd
	case "NOTICE":
		text := m.Trailing()
		kind := store.KindNotice
		target := server
		if len(m.Params) > 0 && isChannel(m.Params[0]) {
			target = m.Params[0]
		}
		// ZNC talks through *status; keep it in the server window.
		if nick == "*status" {
			target = server
		} else if cmd, args, ok := irc.ParseCTCP(text); ok {
			// CTCP replies land in the active buffer for now.
			text = "CTCP " + cmd
			if args != "" {
				text += ": " + args
			}
			if len(a.bufs) > 0 && a.focus < len(a.bufs) && a.bufs[a.focus].Server == server {
				target = a.bufs[a.focus].Name
			}
		}
		cmd = a.addLine(server, target, store.Line{At: at, Nick: nick, Text: text, Kind: kind})
		a.bumpUnread(server, target, nick)
	case "TOPIC":
		if len(m.Params) < 1 {
			return nil
		}
		ch := m.Params[0]
		topic := m.Trailing()
		a.st.Get(server, a.st.Resolve(server, ch)).Topic = topic
		cmd = a.addLine(server, ch, store.Line{At: at, Nick: nick, Text: "changed the topic to: " + topic, Kind: store.KindSystem})
	case "JOIN":
		if len(m.Params) < 1 {
			return nil
		}
		ch := m.Params[0]
		cmd = a.addLine(server, ch, store.Line{At: at, Nick: nick, Text: "joined " + ch, Kind: store.KindJoin})
		if isChannel(ch) {
			a.memberAdd(server, ch, nick, "")
			a.renderNicks()
			// Ask for the full roster: ZNC replays JOINs on attach but
			// doesn't always send NAMES, so the pane would stay sparse.
			if cl, ok := a.clients[server]; ok && nick == a.ownNick(server) {
				_ = cl.Send("NAMES " + ch)
			}
		}
	case "PART":
		ch := ""
		if len(m.Params) > 0 {
			ch = m.Params[0]
		}
		cmd = a.addLine(server, ch, store.Line{At: at, Nick: nick, Text: "left " + ch, Kind: store.KindPart})
		if isChannel(ch) {
			a.memberRemove(server, ch, nick)
			a.renderNicks()
			if nick == a.ownNick(server) {
				// We left: drop the buffer so the sidebar cleans up,
				// unless it was parked in the archive (drag-to-archive
				// auto-parts but keeps the parked buffer).
				if !a.archived[memberKey(server, ch)] {
					a.st.Remove(server, ch)
				}
			}
		}
	case "QUIT":
		cmd = a.addLine(server, server, store.Line{At: at, Nick: nick, Text: "quit: " + m.Trailing(), Kind: store.KindQuit})
		a.memberQuit(server, nick)
		a.renderNicks()
	case "KICK":
		if len(m.Params) < 2 {
			return nil
		}
		ch, target := m.Params[0], m.Params[1]
		text := target + " was kicked by " + nick
		if reason := m.Trailing(); reason != "" && reason != target {
			text += " (" + reason + ")"
		}
		cmd = a.addLine(server, ch, store.Line{At: at, Nick: nick, Text: text, Kind: store.KindSystem})
		if isChannel(ch) {
			a.memberRemove(server, ch, target)
			a.renderNicks()
		}
	case "MODE":
		if len(m.Params) < 2 {
			return nil
		}
		target, modes := m.Params[0], m.Params[1]
		args := m.Params[2:]
		text := nick + " set mode " + modes
		if len(args) > 0 {
			text += " " + strings.Join(args, " ")
		}
		buf := target
		if isChannel(target) {
			a.applyModePrefixes(server, target, modes, args)
			a.renderNicks()
		} else {
			buf = server // our own user modes land in the server window
		}
		cmd = a.addLine(server, buf, store.Line{At: at, Nick: nick, Text: text, Kind: store.KindSystem})
	case "INVITE":
		ch := m.Trailing()
		if ch == "" && len(m.Params) > 1 {
			ch = m.Params[1]
		}
		if ch == "" {
			return nil
		}
		cmd = a.addLine(server, server, store.Line{At: at, Nick: nick, Text: "invited you to " + ch + " — /join " + ch, Kind: store.KindSystem})
	case "NICK":
		newNick := m.Trailing()
		cmd = a.addLine(server, server, store.Line{At: at, Nick: nick, Text: "is now known as " + newNick, Kind: store.KindSystem})
		a.memberRename(server, nick, newNick)
		if nick == a.ownNick(server) {
			a.setOwnNick(server, newNick)
		}
		a.renderNicks()
	default:
		if isNumeric(m.Command) {
			// Keep the welcome/MOTD numerics, skip the noise.
			switch m.Command {
			case "001", "002", "003", "004", "005", "372", "375", "376":
				if m.Command == "001" && len(m.Params) > 0 {
					// 001's target is the nick the server actually
					// assigned, including any "_" 433 fallback.
					a.setOwnNick(server, m.Params[0])
				}
				text := m.Trailing()
				if text == "" && len(m.Params) > 1 {
					text = strings.Join(m.Params[1:], " ")
				}
				cmd = a.addLine(server, server, store.Line{At: at, Text: text, Kind: store.KindSystem})
			case "353":
				// RPL_NAMREPLY: track membership, keep it out of scrollback.
				a.handleNames(server, m.Trailing(), m.Params)
			case "332":
				// RPL_TOPIC: params are [nick, channel], trailing is the topic.
				if len(m.Params) > 1 {
					a.st.Get(server, a.st.Resolve(server, m.Params[1])).Topic = m.Trailing()
				}
			case "366":
				// RPL_ENDOFNAMES: roster complete; refresh the pane.
				a.renderNicks()
			case "403", "442", "471", "473", "474", "475", "476":
				// Join failures used to vanish silently: the server
				// never echoes our JOIN, so no channel buffer appears
				// and the user is left guessing. Surface them in the
				// server window with the channel name attached.
				text := strings.Join(m.Params[1:], " ")
				if t := m.Trailing(); t != "" {
					if text != "" {
						text += " "
					}
					text += ": " + t
				}
				cmd = a.addLine(server, server, store.Line{At: at, Text: text, Kind: store.KindSystem})
			}
		}
	}
	return cmd
}

// queueImageFetches starts background fetches for image URLs in a chat
// message. Each fetch renders half-block art and reports back as an
// imageFetchedMsg; already-cached or in-flight URLs are skipped (a second
// message sharing an in-flight URL gets the art from the same fetch).
// seq/slot identify the message line each URL came from, so the finished
// preview lands directly under its message instead of at the end of the
// buffer.
func (a *App) queueImageFetches(server, buf string, seq uint64, text string) tea.Cmd {
	urls := img.FindURLs(text)
	if len(urls) == 0 {
		return nil
	}
	w := a.chat.Width - 2 // leave room for the indent
	if w > 48 {
		w = 48
	}
	if w < 8 {
		w = 8
	}
	var cmds []tea.Cmd
	for i, u := range urls {
		if art, ok := a.imgCache[u]; ok {
			a.setLineArt(server, buf, seq, i, art)
			continue
		}
		a.imgPending[u] = append(a.imgPending[u], artSlot{server: server, buf: buf, seq: seq, slot: i})
		if a.imgInflight[u] {
			continue
		}
		a.imgInflight[u] = true
		url := u
		cmds = append(cmds, func() tea.Msg {
			m, err := img.Fetch(context.Background(), url)
			if err != nil {
				return imageFetchedMsg{url: url}
			}
			return imageFetchedMsg{url: url, art: img.Render(m, w, 12)}
		})
	}
	return tea.Batch(cmds...)
}

// setLineArt attaches a finished preview to its message line. The buffer
// may be gone (parted while fetching): don't resurrect it.
func (a *App) setLineArt(server, buf string, seq uint64, slot int, art string) {
	if !a.st.Has(server, buf) {
		return
	}
	a.st.Get(server, buf).SetImageArt(seq, slot, art)
}

// handleImageFetched files a finished preview into scrollback. Failures
// (empty art) are not cached: a re-posted URL gets a fresh attempt instead
// of being poisoned for the rest of the session.
func (a *App) handleImageFetched(msg imageFetchedMsg) {
	delete(a.imgInflight, msg.url)
	slots := a.imgPending[msg.url]
	delete(a.imgPending, msg.url)
	if msg.art == "" {
		return
	}
	a.imgCache[msg.url] = msg.art
	for _, s := range slots {
		a.setLineArt(s.server, s.buf, s.seq, s.slot, msg.art)
	}
	a.refreshBuffers()
	if a.ready {
		a.renderChat()
	}
}

func isChannel(s string) bool {
	return strings.HasPrefix(s, "#") || strings.HasPrefix(s, "&")
}

// nickPaneWidth reserves a right-hand nick list when the focused buffer is
// a channel and the terminal is wide enough to spare the space.
func (a *App) nickPaneWidth() int {
	if a.width < 90 || len(a.bufs) == 0 || a.focus >= len(a.bufs) {
		return 0
	}
	if !isChannel(a.bufs[a.focus].Name) {
		return 0
	}
	return 21 // 20 for nicks + 1 for the border
}

// renderNicks fills the right-hand pane with the focused channel's roster,
// ops first, then alphabetical.
func (a *App) renderNicks() {
	if len(a.bufs) == 0 || a.focus >= len(a.bufs) {
		a.nicks.SetContent("")
		return
	}
	buf := a.bufs[a.focus]
	if !isChannel(buf.Name) {
		a.nicks.SetContent("")
		return
	}
	names := a.members[memberKey(buf.Server, buf.Name)].sorted()
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%d)\n", buf.Name, len(names))
	// Dim rule separating the channel header from the roster, matching
	// the sidebar's channel/DM divider.
	w := a.nicks.Width - 4
	if w < 8 {
		w = 12
	}
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("  "+strings.Repeat("─", w)) + "\n")
	for _, n := range names {
		b.WriteString(n.prefix + n.nick + "\n")
	}
	a.nicks.SetContent(b.String())
	a.nicks.GotoTop()
}

// ctcpReply answers the common CTCP queries. Unknown commands return "",
// meaning no reply: answering those is how CTCP loops start.
func ctcpReply(cmd, args string) string {
	switch cmd {
	case "VERSION":
		return "VERSION Muse-IRCGO " + version.Version + " (vibed lul)"
	case "PING":
		if args == "" {
			return ""
		}
		return "PING " + args
	case "TIME":
		return "TIME " + time.Now().Format("Mon Jan 2 15:04:05 2006")
	case "CLIENTINFO":
		return "CLIENTINFO VERSION PING TIME FINGER USERINFO CLIENTINFO ACTION"
	case "FINGER", "USERINFO":
		return cmd + " ircgo user"
	}
	return ""
}

func isNumeric(s string) bool {
	if len(s) != 3 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// sidebarBufferAt maps a mouse click to a buffer index. The sidebar renders
// server headers and blank separators between servers, so screen rows don't
// line up 1:1 with buffers; this replays that layout to find the buffer on
// the clicked row. ok is false for headers, separators, blank space, and
// clicks outside the sidebar.
// sidebarWidth is the on-screen width of the sidebar column.
func (a *App) sidebarWidth() int {
	if sw := a.cfg.UI.SidebarWidth; sw >= 16 {
		return sw
	}
	return 16
}

func (a *App) sidebarBufferAt(x, y int) (i int, ok bool) {
	sw := a.sidebarWidth()
	if x < 0 || x > sw || y < 0 {
		return 0, false // x == sw is the sidebar's right border
	}
	row := y + a.sidebar.YOffset // the sidebar viewport may be scrolled
	r := 0
	lastServer := ""
	lastRank := -1
	// Normal section: the server window is its section's first row (no
	// separate header line). Archived buffers are skipped here; they live
	// in the bottom-pinned section walked below.
	for idx, buf := range a.bufs {
		if a.displayRank(buf) == 3 {
			continue
		}
		if buf.Server != lastServer {
			if lastServer != "" {
				if r == row {
					return 0, false // separator line
				}
				r++
			}
			lastServer = buf.Server
			lastRank = -1
		}
		rk := a.displayRank(buf)
		if rk == 2 && lastRank < 2 {
			if r == row {
				return 0, false // "-- Messages --" divider line
			}
			r++
		}
		lastRank = rk
		if r == row {
			return idx, true
		}
		r++
	}
	// Blank padding that pushes the archive section to the bottom.
	for p := 0; p < a.archivePad; p++ {
		if r == row {
			return 0, false
		}
		r++
	}
	// Bottom-pinned archive section.
	if a.showArchiveSection() {
		d := a.dragBuffer()
		if d != nil && a.archived[memberKey(d.Server, d.Name)] && !a.serverHasVisible(d.Server) {
			if r == row {
				return 0, false // "drop to recover" hint line
			}
			r++
		}
		if r == row {
			return 0, false // "-- Archive --" divider line
		}
		r++
		lastServer = ""
		for idx, buf := range a.bufs {
			if a.displayRank(buf) != 3 {
				continue
			}
			if buf.Server != lastServer {
				if r == row {
					return 0, false // archive server label line
				}
				r++
				lastServer = buf.Server
			}
			if r == row {
				return idx, true
			}
			r++
		}
	}
	return 0, false
}

// sgrRe strips the SGR color/attribute sequences the renderer emits.
var sgrRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return sgrRe.ReplaceAllString(s, "") }

// wrapRow word-wraps s exactly the way the chat viewport does, so click
// coordinates can be mapped onto content lines.
func wrapRow(s string, w int) []string {
	if w < 1 {
		return []string{s}
	}
	return strings.Split(lipgloss.NewStyle().Width(w).Render(s), "\n")
}

// drow is one visible display row of the chat viewport and the content line
// it was wrapped from.
type drow struct {
	text string
	line int
}

// urlHit is a clickable URL region on a display row, in cell coordinates.
type urlHit struct {
	url        string
	start, end int
}

// visibleRows re-wraps the chat content exactly like the viewport, returning
// the currently visible display rows.
func (a *App) visibleRows() []drow {
	var rows []drow
	w := a.chat.Width
	for i := a.chat.YOffset; i < len(a.chatContentRows) && len(rows) < a.chat.Height; i++ {
		for _, sub := range wrapRow(a.chatContentRows[i], w) {
			rows = append(rows, drow{text: sub, line: i})
			if len(rows) >= a.chat.Height {
				break
			}
		}
	}
	return rows
}

// wrappedURL reports the full content-line URL when rawFrag (a row-end URL
// fragment) is the strict prefix of a URL on that content line: the URL was
// broken across display rows by word wrap.
func (a *App) wrappedURL(line int, rawFrag string) string {
	if line < 0 || line >= len(a.chatContentRows) {
		return ""
	}
	match := ""
	for _, sp := range link.FindSpans(a.chatContentRows[line]) {
		if strings.HasPrefix(sp.URL, rawFrag) && len(sp.URL) > len(rawFrag) {
			if match != "" {
				return "" // ambiguous
			}
			match = sp.URL
		}
	}
	return match
}

// rowLinkHits returns the clickable URL regions on visible display row d.
func (a *App) rowLinkHits(rows []drow, d int) []urlHit {
	text := strings.TrimRight(stripANSI(rows[d].text), " ")
	var hits []urlHit
	spans := link.FindSpans(text)
	for i, sp := range spans {
		start := lipgloss.Width(text[:sp.Start])
		url, end := sp.URL, start+lipgloss.Width(sp.URL)
		if i == len(spans)-1 && sp.End == len(text) {
			// A URL running to the row's end may continue on the next
			// display row; glue it via the unwrapped content line.
			if full := a.wrappedURL(rows[d].line, text[sp.Start:sp.End]); full != "" {
				url, end = full, lipgloss.Width(text)
			}
		}
		hits = append(hits, urlHit{url: url, start: start, end: end})
	}
	// A row may start with the continuation of a URL wrapped from the
	// previous row (the fragment has no scheme, so FindSpans misses it).
	if d > 0 && rows[d].line == rows[d-1].line {
		prev := strings.TrimRight(stripANSI(rows[d-1].text), " ")
		if pspans := link.FindSpans(prev); len(pspans) > 0 {
			if p := pspans[len(pspans)-1]; p.End == len(prev) {
				if full := a.wrappedURL(rows[d-1].line, prev[p.Start:p.End]); full != "" {
					frag := text
					if j := strings.IndexAny(frag, " \t"); j >= 0 {
						frag = frag[:j]
					}
					if frag != "" {
						hits = append(hits, urlHit{url: full, start: 0, end: lipgloss.Width(frag)})
					}
				}
			}
		}
	}
	return hits
}

// linkAt returns the URL under a chat-pane click, if any.
func (a *App) linkAt(x, y int) (string, bool) {
	sw := a.cfg.UI.SidebarWidth
	if sw < 16 {
		sw = 16
	}
	x0 := sw + 1 // chat pane starts after the sidebar border
	if x < x0 || x >= x0+a.chat.Width || a.chat.Width <= 0 {
		return "", false
	}
	top := 0
	if a.topicVisible() {
		top = topicBarHeight
	}
	d := y - top // display row within the viewport
	rows := a.visibleRows()
	if d < 0 || d >= len(rows) {
		return "", false
	}
	hits := a.rowLinkHits(rows, d)
	if len(hits) == 1 {
		return hits[0].url, true // single link on the row: be lenient
	}
	cell := x - x0
	for _, h := range hits {
		if cell >= h.start && cell < h.end {
			return h.url, true
		}
	}
	return "", false
}

// openURL opens url in the desktop browser without blocking the UI.
func openURL(url string) {
	cmd := exec.Command("xdg-open", url)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if err := cmd.Start(); err != nil {
		return
	}
	go func() { _ = cmd.Wait() }() // reap the child
}

// bufferRank orders buffers within a server group: the server window first,
// then channels, then query (DM) buffers below a divider.
func bufferRank(b *store.Buffer) int {
	switch {
	case b.Name == b.Server:
		return 0
	case isChannel(b.Name):
		return 1
	default:
		return 2
	}
}

// displayRank orders buffers within a server group: the server window,
// then channels, then queries under "-- Messages --", then parked buffers
// under "-- Archive --".
func (a *App) displayRank(b *store.Buffer) int {
	if a.archived[memberKey(b.Server, b.Name)] {
		return 3
	}
	return bufferRank(b)
}

// refreshBuffers rebuilds the sidebar buffer list, ordering each server's
// buffers as server window, channels, then queries. Focus follows the
// buffer by identity, so a newly created buffer slotting into its sorted
// position can't steal focus from under you.
func (a *App) refreshBuffers() {
	var fs, fn string
	if a.focus < len(a.bufs) {
		fs, fn = a.bufs[a.focus].Server, a.bufs[a.focus].Name
	}
	var hs, hn string
	if a.hovered >= 0 && a.hovered < len(a.bufs) {
		hs, hn = a.bufs[a.hovered].Server, a.bufs[a.hovered].Name
	}
	var bufs []*store.Buffer
	for _, b := range a.st.Buffers() {
		if a.hidden[memberKey(b.Server, b.Name)] {
			continue
		}
		bufs = append(bufs, b)
	}
	sort.SliceStable(bufs, func(i, j int) bool {
		if bufs[i].Server != bufs[j].Server {
			return false
		}
		ri, rj := a.displayRank(bufs[i]), a.displayRank(bufs[j])
		if ri != rj {
			return ri < rj
		}
		return a.orderSeq(bufs[i]) < a.orderSeq(bufs[j])
	})
	a.bufs = bufs
	// Track every visible buffer in the persisted order: new arrivals take
	// the next free slot (sorting them at the end of their group), and the
	// list is written back so a restart restores positions. Parking,
	// recovering or removing a buffer never re-sequences the list, so a
	// recovered buffer returns to its old slot.
	if a.trackOrder() {
		a.writeBufferOrder()
	}
	found := false
	for i, b := range a.bufs {
		if b.Server == fs && b.Name == fn {
			a.focus = i
			found = true
			break
		}
	}
	if !found {
		// The focused buffer was parked or removed: keep the same row,
		// which now holds its neighbor, clamping at the end.
		a.focus = min(a.focus, max(len(a.bufs)-1, 0))
	}
	a.hovered = -1
	for i, b := range a.bufs {
		if b.Server == hs && b.Name == hn {
			a.hovered = i
			break
		}
	}
	// Startup restore: focus the remembered buffer as soon as it appears
	// (channels are created on JOIN, after the restore ran).
	if pf := a.pendingFocus; pf.server != "" {
		name := a.st.Resolve(pf.server, pf.name)
		for i, b := range a.bufs {
			if b.Server == pf.server && b.Name == name {
				a.setFocus(i)
				a.pendingFocus = lastFocus{}
				break
			}
		}
	}
}

// sidebarDivider renders a labeled dim divider: "-- Messages --" between a
// server's channels and its query buffers, "-- Archive --" above parked
// buffers. The archive divider only appears when parked buffers exist (or
// as a drag drop target while a drag is active).
func (a *App) sidebarDivider(label string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("  -- " + label + " --")
}

func (a *App) renderSidebar() {
	a.dropRows = map[int]dropTarget{}
	var lines []string
	emit := func(s string) { lines = append(lines, s) }
	d := a.dragBuffer()
	dragServer, dragArchived := "", false
	if d != nil {
		dragServer = d.Server
		dragArchived = a.archived[memberKey(d.Server, d.Name)]
	}
	// Normal section: one row per buffer. The server window renders as its
	// section's header row (bold, clickable): there is no separate header
	// line. Archived buffers are skipped here; they render in the
	// bottom-pinned section below.
	lastServer := ""
	lastRank := -1
	for i, buf := range a.bufs {
		if a.displayRank(buf) == 3 {
			continue
		}
		if buf.Server != lastServer {
			if lastServer != "" {
				emit("") // separator line
			}
			lastServer = buf.Server
			lastRank = -1
		}
		r := a.displayRank(buf)
		if r == 2 && lastRank < 2 {
			emit(a.sidebarDivider("Messages"))
		}
		lastRank = r
		emit(a.sidebarRow(i, buf))
	}
	// Bottom-pinned archive section, built separately so blank padding can
	// push it to the last rows of the viewport and conserve space above.
	type archDrop struct {
		row    int
		target dropTarget
	}
	var arch []string
	var archDrops []archDrop // arch-relative rows, offset once padding is known
	archEmit := func(s string) { arch = append(arch, s) }
	if a.showArchiveSection() {
		// Recover hint: an archived drag whose home group has no visible
		// rows to land on gets a landing spot above the archive.
		if d != nil && dragArchived && !a.serverHasVisible(dragServer) {
			archDrops = append(archDrops, archDrop{len(arch), dropTarget{server: dragServer, kind: "recover"}})
			archEmit(a.dropDivider("drop to recover", a.dropKind == "recover"))
		}
		if d != nil && !dragArchived {
			archDrops = append(archDrops, archDrop{len(arch), dropTarget{server: dragServer, kind: "archive"}})
		}
		archEmit(a.dropDivider("Archive", d != nil && !dragArchived && a.dropKind == "archive"))
		lastServer = ""
		for i, buf := range a.bufs {
			if a.displayRank(buf) != 3 {
				continue
			}
			if buf.Server != lastServer {
				archEmit(lipgloss.NewStyle().Bold(true).Render(buf.Server))
				lastServer = buf.Server
			}
			archEmit(a.sidebarRow(i, buf))
		}
	}
	pad := a.sidebar.Height - len(lines) - len(arch)
	if pad < 0 {
		pad = 0
	}
	a.archivePad = pad
	base := len(lines) + pad
	for _, ad := range archDrops {
		a.dropRows[base+ad.row] = ad.target
	}
	for p := 0; p < pad; p++ {
		emit("")
	}
	lines = append(lines, arch...)
	a.sidebar.SetContent(strings.Join(lines, "\n") + "\n")
}

// showArchiveSection reports whether the bottom-pinned archive section
// renders: whenever something is parked, or mid-drag as a park target for
// a normal buffer.
func (a *App) showArchiveSection() bool {
	if a.hasArchived() {
		return true
	}
	d := a.dragBuffer()
	return d != nil && !a.archived[memberKey(d.Server, d.Name)]
}

// hasArchived reports whether any buffer is currently parked.
func (a *App) hasArchived() bool {
	for _, b := range a.bufs {
		if a.archived[memberKey(b.Server, b.Name)] {
			return true
		}
	}
	return false
}

// serverHasVisible reports whether server has any non-archived buffer row
// in the sidebar's normal section.
func (a *App) serverHasVisible(server string) bool {
	for _, b := range a.bufs {
		if b.Server == server && !a.archived[memberKey(b.Server, b.Name)] {
			return true
		}
	}
	return false
}

// dropDivider renders a drop-zone divider, highlighted when it is the
// current drop target.
func (a *App) dropDivider(label string, hot bool) string {
	s := "  -- " + label + " --"
	if hot {
		return dropStyle.Render(s)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(s)
}

// sidebarRow renders one buffer line. The server window doubles as its
// section's header row and renders bold; channels and PMs indent beneath
// it, sketching the server -> buffers hierarchy. The hovered row (server
// windows excepted) is highlighted across its full width and grows click
// affordances on the right: x parks the buffer in the archive, and
// archived rows also get + to recover them. The focused row keeps its
// brighter reverse highlight even while hovered.
func (a *App) sidebarRow(i int, buf *store.Buffer) string {
	marker := "  "
	if i == a.focus {
		marker = "> "
	}
	indent := ""
	if r := bufferRank(buf); r == 1 || r == 2 {
		indent = "  "
	}
	prefix := indent + marker
	hovered := i == a.hovered && !a.dragActive && bufferRank(buf) != 0
	text := prefix + buf.Name
	if !hovered && buf.Unread > 0 && i != a.focus {
		text += " *"
	}
	switch {
	case a.dragActive && i == a.dragIdx:
		return dragStyle.Render(text)
	case a.dragActive && i == a.dropIdx && a.dropKind != "":
		return dropStyle.Render(text)
	case i == a.focus:
		rev := lipgloss.NewStyle().Reverse(true)
		if hovered {
			return a.hoverText(prefix, buf, rev, rev)
		}
		// Pad to the full width like a hovered row, so the highlight
		// reads as one continuous row instead of ending at the text.
		return rev.Render(a.padRow(text))
	case hovered:
		dim := lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Background(lipgloss.Color("237"))
		return a.hoverText(prefix, buf, hoverStyle, dim)
	case bufferRank(buf) == 0:
		return lipgloss.NewStyle().Bold(true).Render(text)
	case buf.Unread > 0:
		return unreadStyle.Render(text)
	default:
		return text
	}
}

// padRow pads s with spaces to the full sidebar width, so row highlights
// (focus, hover) span the whole row instead of ending at the text.
func (a *App) padRow(s string) string {
	sw := a.sidebarWidth()
	r := []rune(s)
	for len(r) < sw {
		r = append(r, ' ')
	}
	return string(r)
}

// hoverText renders a hovered sidebar row: the row style spans the full
// padded width so the highlight reads as one continuous row, with dim
// affordances pinned to the right edge ("x" at sw-2; archived rows also
// get "+" at sw-4 to recover them). Columns match sidebarAffordance's hit
// zones.
func (a *App) hoverText(marker string, buf *store.Buffer, row, dim lipgloss.Style) string {
	sw := a.sidebarWidth()
	r := []rune(truncateRunes(marker+buf.Name, sw-3))
	for len(r) < sw-2 {
		r = append(r, ' ')
	}
	if a.archived[memberKey(buf.Server, buf.Name)] {
		return row.Render(string(r[:sw-4])) + dim.Render("+") + row.Render(" ") + dim.Render("x")
	}
	return row.Render(string(r)) + dim.Render("x")
}

// sidebarAffordance maps a click's x-coordinate on row i to the hover
// affordance action there: "close" (x), "recover" (+), or "" for a plain
// click. Affordances only exist on the hovered row.
func (a *App) sidebarAffordance(i int, x int) string {
	if i != a.hovered || i < 0 || i >= len(a.bufs) {
		return ""
	}
	b := a.bufs[i]
	if bufferRank(b) == 0 {
		return "" // the server window can't be parked
	}
	sw := a.sidebarWidth()
	switch {
	case x >= sw-2:
		return "close"
	case x >= sw-4 && a.archived[memberKey(b.Server, b.Name)]:
		return "recover"
	}
	return ""
}

// closeBuffer handles the hover x: a sidebar buffer is parked in the
// archive (channels stay joined); an archived buffer is removed from view
// entirely. New activity in either returns the buffer to the sidebar.
func (a *App) closeBuffer(i int) {
	if i < 0 || i >= len(a.bufs) || bufferRank(a.bufs[i]) == 0 {
		return
	}
	b := a.bufs[i]
	key := memberKey(b.Server, b.Name)
	wasFocused := i == a.focus
	if a.archived[key] {
		a.hidden[key] = true
		delete(a.archived, key)
	} else {
		a.archived[key] = true
	}
	a.refreshBuffers()
	if wasFocused && len(a.bufs) > 0 {
		a.setFocus(min(i, len(a.bufs)-1))
	}
	a.renderSidebar()
	a.renderChat()
}

// recoverBuffer handles the hover + on an archived buffer: it returns to
// its grouping (channels with channels, queries under Messages) and takes
// focus. Buffers drag-parked with an auto-PART are re-JOINed.
func (a *App) recoverBuffer(i int) {
	if i < 0 || i >= len(a.bufs) {
		return
	}
	b := a.bufs[i]
	key := memberKey(b.Server, b.Name)
	delete(a.archived, key)
	if a.partedOnPark[key] {
		delete(a.partedOnPark, key)
		if cl, ok := a.clients[b.Server]; ok {
			_ = cl.Send("JOIN " + b.Name)
		}
	}
	a.refreshBuffers()
	for j, ob := range a.bufs {
		if ob.Server == b.Server && ob.Name == b.Name {
			a.setFocus(j)
			break
		}
	}
	a.renderSidebar()
	a.renderChat()
}

// topicVisible reports whether the focused buffer gets a topic panel:
// channel buffers only.
func (a *App) topicVisible() bool {
	if len(a.bufs) == 0 || a.focus >= len(a.bufs) {
		return false
	}
	return isChannel(a.bufs[a.focus].Name)
}

// topicBar renders the focused channel's topic as a small panel above the
// chat feed. mIRC formatting codes in the topic are rendered, not leaked.
func (a *App) topicBar() string {
	b := a.bufs[a.focus]
	topic := b.Topic
	if topic == "" {
		topic = "no topic"
	}
	w := a.chat.Width
	label := "Topic: "
	topic = truncateRunes(topic, max(w-len(label), 0))
	return topicStyle.Width(w).Render(label + irc.FormatText(topic))
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return ""
	}
	return string(r[:n-1]) + "…"
}

func (a *App) renderChat() {
	if len(a.bufs) == 0 {
		a.chat.SetContent("connecting…")
		return
	}
	if a.focus >= len(a.bufs) {
		a.focus = 0
	}
	buf := a.bufs[a.focus]
	var b strings.Builder
	for _, l := range buf.Lines() {
		ts := l.At.Format(a.cfg.UI.TimestampFormat)
		tsW := visibleWidth(ts)
		switch l.Kind {
		case store.KindChat:
			// 14-wide nick column plus the two separating spaces.
			prefixW := tsW + 1 + 15
			prefix := tsStyle.Render(ts) + " " +
				nickStyle.Render(fmt.Sprintf("%-14s", l.Nick)) + " "
			text := link.Style(irc.FormatText(l.Text), func(s string) string {
				return linkStyle.Render(s)
			})
			a.writeChatRow(&b, prefix, prefixW, text)
		case store.KindAction:
			prefixW := tsW + 1
			prefix := tsStyle.Render(ts) + " "
			text := sysStyle.Render("* "+l.Nick+" ") + irc.FormatStyled(l.Text, sysStyle)
			a.writeChatRow(&b, prefix, prefixW, text)
		default:
			prefixW := tsW + 1
			prefix := tsStyle.Render(ts) + " "
			a.writeChatRow(&b, prefix, prefixW, sysStyle.Render(l.Text))
		}
		// Image previews live on their message line; render them directly
		// underneath it. Art rows are pre-wrapped: never re-wrap them.
		for _, art := range l.Art {
			if art == "" {
				continue
			}
			for _, row := range strings.Split(art, "\n") {
				fmt.Fprintf(&b, "  %s\n", row)
			}
		}
	}
	a.chat.SetContent(b.String())
	a.chat.GotoBottom()
	// Mirror the content lines without ANSI so linkAt can map clicks to
	// URLs; the split matches viewport.SetContent exactly.
	rows := strings.Split(b.String(), "\n")
	plain := make([]string, len(rows))
	for i, r := range rows {
		plain[i] = stripANSI(r)
	}
	a.chatContentRows = plain
}

// writeChatRow writes one logical chat line as physical rows: prefix plus
// the first wrapped row, then continuation rows (already indented) under it.
func (a *App) writeChatRow(b *strings.Builder, prefix string, prefixW int, text string) {
	rows := a.wrapText(text, prefixW)
	b.WriteString(prefix + rows[0] + "\n")
	for _, r := range rows[1:] {
		b.WriteString(r + "\n")
	}
}

// wrapText wraps styled text to the chat width, indenting continuation
// lines by indentW cells. Before the first real layout (or on an absurdly
// narrow pane) it returns the text unwrapped.
func (a *App) wrapText(text string, indentW int) []string {
	w := a.chat.Width
	if w < 40 || w-indentW < 20 {
		return []string{text}
	}
	return wrapANSI(text, w-indentW, strings.Repeat(" ", indentW))
}

func (a *App) resize() {
	sw := a.cfg.UI.SidebarWidth
	if sw < 16 {
		sw = 16
	}
	nw := a.nickPaneWidth()
	chatW := a.width - sw - 1 - nw // 1 for the sidebar border
	if chatW < 20 {
		chatW = 20
	}
	// When the topic panel is visible the middle column is taller; pad the
	// sidebar and nick columns so all three bottom borders line up.
	extra := 0
	if a.topicVisible() {
		extra = topicBarHeight
	}
	chatH := a.height - 3 - extra // input line + status line + bottom divider
	if chatH < 5 {
		chatH = 5
	}
	a.sidebar = viewport.New(sw, chatH+extra)
	a.chat = viewport.New(chatW, chatH)
	a.nicks = viewport.New(max(nw-1, 1), chatH+extra)
	a.input.Width = chatW - 2
	a.renderSidebar()
	a.renderChat()
	a.renderNicks()
}

func (a *App) View() string {
	if !a.ready {
		return "connecting…"
	}
	main := lipgloss.JoinHorizontal(lipgloss.Top,
		sidebarStyle.Render(a.sidebar.View()),
		chatBottomStyle.Render(a.chatColumn()),
	)
	if a.nickPaneWidth() > 0 {
		main = lipgloss.JoinHorizontal(lipgloss.Top,
			main,
			nicksStyle.Render(a.nicks.View()),
		)
	}
	body := lipgloss.JoinVertical(lipgloss.Left, main, a.input.View())
	return lipgloss.JoinVertical(lipgloss.Left, body, a.statusLine())
}

// chatColumn stacks the topic panel above the chat viewport when the
// focused buffer is a channel.
func (a *App) chatColumn() string {
	if a.topicVisible() {
		return lipgloss.JoinVertical(lipgloss.Left, a.topicBar(), a.chat.View())
	}
	return a.chat.View()
}

func (a *App) statusLine() string {
	focused := ""
	if len(a.bufs) > 0 && a.focus < len(a.bufs) {
		b := a.bufs[a.focus]
		focused = b.Server + "/" + b.Name
	}
	return statusStyle.Width(a.width).
		Render(fmt.Sprintf(" %s  •  tab: switch buffer  •  /q: quit ", focused))
}

// sendInput routes the input line: slash commands or a PRIVMSG to the
// focused buffer.
func (a *App) sendInput(v string) tea.Cmd {
	if len(a.bufs) == 0 {
		return nil
	}
	buf := a.bufs[a.focus]
	cl, ok := a.clients[buf.Server]
	if !ok {
		return nil
	}
	if strings.HasPrefix(v, "/") {
		return a.sendCommand(cl, buf, v)
	}
	target := buf.Name
	if target == buf.Server {
		return nil // nowhere to send from the server window
	}
	_ = cl.Send("PRIVMSG " + target + " :" + v)
	addCmd := a.addLine(buf.Server, target, store.Line{At: time.Now(), Nick: a.ownNick(buf.Server), Text: v, Kind: store.KindChat})
	a.renderChat()
	return addCmd
}

func (a *App) ownNick(server string) string {
	if n := a.ownNicks[server]; n != "" {
		return n
	}
	for _, s := range a.cfg.Servers {
		if s.Name == server {
			return s.Nick
		}
	}
	return "me"
}

// setOwnNick records the nick the server actually assigned us: 001's target
// parameter after registration (which reflects any "_" 433 fallback), or a
// NICK echo confirming a /nick change.
func (a *App) setOwnNick(server, nick string) {
	if nick == "" {
		return
	}
	if a.ownNicks == nil {
		a.ownNicks = map[string]string{}
	}
	a.ownNicks[server] = nick
}

// focusBuffer switches to buffer i, clearing its unread marker. All
// buffer-switching paths (tab, alt+digit, mouse click) go through here.
func (a *App) focusBuffer(i int) {
	if i < 0 || i >= len(a.bufs) {
		return
	}
	a.setFocus(i)
	// A manual switch cancels any pending startup restore.
	a.pendingFocus = lastFocus{}
}

// setFocus moves focus to buffer i, clearing its unread marker, and
// persists it immediately. The startup restore path uses it too, so the
// restored buffer doesn't rewrite the very state being restored — the
// write is a harmless no-op there. Persisting on every switch (not just
// on quit) means an unclean shutdown — closed terminal, kill, crash —
// still restores the buffer that was actually focused.
func (a *App) setFocus(i int) {
	a.focus = i
	a.bufs[i].Unread = 0
	// Re-lay out, since the nick pane only appears for channel buffers.
	a.resize()
	a.saveLastBuffer()
}

// saveLastBuffer records the focused buffer in the config file so the next
// launch can restore it. It runs on every focus switch (via setFocus) and
// once more on quit, so the on-disk value is always current even if the
// process dies without a clean shutdown.
func (a *App) saveLastBuffer() {
	if a.ConfigPath == "" || a.focus < 0 || a.focus >= len(a.bufs) {
		return
	}
	b := a.bufs[a.focus]
	_ = config.WriteLastBuffer(a.ConfigPath, b.Server, b.Name)
}

// restoreLastFocus focuses the buffer that was focused when the app last
// quit (from the config's last_buffer). Buffers that don't exist yet
// (channels waiting on JOIN, the server window waiting on connect) are
// remembered in pendingFocus and picked up by refreshBuffers when they
// appear.
func (a *App) restoreLastFocus() {
	if len(a.cfg.LastBuffer) != 2 {
		return
	}
	server, name := a.cfg.LastBuffer[0], a.cfg.LastBuffer[1]
	if server == "" || name == "" {
		return
	}
	name = a.st.Resolve(server, name)
	for i, b := range a.bufs {
		if b.Server == server && b.Name == name {
			a.setFocus(i)
			return
		}
	}
	a.pendingFocus = lastFocus{server: server, name: name}
}

// restoreQueryBuffers recreates query (DM) buffers from their log files on
// startup, so DMs are visible in the sidebar without waiting for a new
// message. Channels are skipped: joined channels recreate their own
// buffers on JOIN, and parted ones should stay gone. The server window is
// skipped too: it's created on connect. It returns image-fetch commands so
// previews regenerate for URLs in the restored scrollback.
func (a *App) restoreQueryBuffers() tea.Cmd {
	if history.LogDir == "" {
		return nil
	}
	var cmds []tea.Cmd
	for _, s := range a.cfg.Servers {
		entries, err := os.ReadDir(history.ServerDir(s.Name))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".log")
			if name == s.Name || isChannel(name) {
				continue
			}
			// The log filename is the sanitized target name; for ordinary
			// nicks that's the buffer name itself.
			if a.st.Has(s.Name, name) {
				continue
			}
			cmds = append(cmds, a.playbackHistory(s.Name, name, a.st.Get(s.Name, name)))
		}
	}
	a.refreshBuffers()
	return tea.Batch(cmds...)
}

// bumpUnread flags unseen activity on a buffer. Our own messages and the
// focused buffer never bump.
func (a *App) bumpUnread(server, buf, nick string) {
	if nick == "" || nick == a.ownNick(server) {
		return
	}
	// Resolve the case-insensitive spelling first: Get would otherwise
	// create a "Belial" buffer next to the "belial" one addLine just used.
	buf = a.st.Resolve(server, buf)
	if len(a.bufs) > 0 && a.focus < len(a.bufs) {
		if b := a.bufs[a.focus]; b.Server == server && b.Name == buf {
			return
		}
	}
	a.st.Get(server, buf).Unread++
}

func (a *App) sendCommand(cl *irc.Client, buf *store.Buffer, v string) tea.Cmd {
	parts := strings.Fields(v)
	if len(parts) == 0 {
		return nil
	}
	switch strings.ToLower(parts[0]) {
	case "/join":
		if len(parts) > 1 {
			_ = cl.Send("JOIN " + parts[1])
		}
	case "/part":
		target := buf.Name
		if len(parts) > 1 {
			target = parts[1]
		}
		_ = cl.Send("PART " + target)
	case "/msg":
		if len(parts) > 2 {
			to := parts[1]
			text := strings.Join(parts[2:], " ")
			_ = cl.Send("PRIVMSG " + to + " :" + text)
			addCmd := a.addLine(buf.Server, to, store.Line{At: time.Now(), Nick: a.ownNick(buf.Server), Text: text, Kind: store.KindChat})
			a.refreshBuffers()
			a.renderSidebar()
			a.renderChat()
			return addCmd
		}
	case "/me":
		// /me does an emote in the focused buffer (CTCP ACTION).
		if len(parts) > 1 && buf.Name != buf.Server {
			text := strings.Join(parts[1:], " ")
			_ = cl.Send("PRIVMSG " + buf.Name + " :\x01ACTION " + text + "\x01")
			addCmd := a.addLine(buf.Server, buf.Name, store.Line{At: time.Now(), Nick: a.ownNick(buf.Server), Text: text, Kind: store.KindAction})
			a.renderChat()
			return addCmd
		}
	case "/nick":
		// The server confirms with a NICK echo (or 433 -> "_" fallback);
		// either way the tracked nick updates when the reply arrives.
		if len(parts) > 1 {
			_ = cl.Send("NICK " + parts[1])
		}
	case "/ctcp":
		// /ctcp <target> <command> [args] -> PRIVMSG target :\x01COMMAND args\x01
		if len(parts) > 2 {
			to := parts[1]
			cmd := strings.ToUpper(parts[2])
			args := ""
			if len(parts) > 3 {
				args = " " + strings.Join(parts[3:], " ")
			}
			_ = cl.Send("PRIVMSG " + to + " :\x01" + cmd + args + "\x01")
			// Log the outgoing query in the active buffer instead of
			// opening a buffer for the target.
			a.addLine(buf.Server, buf.Name, store.Line{At: time.Now(), Nick: a.ownNick(buf.Server), Text: "CTCP " + cmd + args + " -> " + to, Kind: store.KindNotice})
			a.refreshBuffers()
			a.renderSidebar()
			a.renderChat()
		}
	case "/quit", "/q":
		return tea.Quit
	case "/topic":
		// /topic [new topic]: with text, set the channel topic;
		// without, ask the server for it (replies with 332).
		if isChannel(buf.Name) {
			if len(parts) > 1 {
				_ = cl.Send("TOPIC " + buf.Name + " :" + strings.Join(parts[1:], " "))
			} else {
				_ = cl.Send("TOPIC " + buf.Name)
			}
		}
	}
	return nil
}
