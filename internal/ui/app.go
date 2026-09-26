// Package ui is the Bubble Tea interface: a sidebar of buffers, a chat
// viewport, an input line, and a status bar.
package ui

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
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
	server, buf, url string
	art              string
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

	bufs  []*store.Buffer
	focus int

	members map[string]*memberSet

	// Image previews: fetched art by URL, and URLs currently fetching.
	imgCache    map[string]string
	imgInflight map[string]bool

	sidebar viewport.Model
	chat    viewport.Model
	nicks   viewport.Model
	input   textinput.Model

	// chatContentRows mirrors the chat viewport's content lines with ANSI
	// stripped, index-aligned with them, so clicks can be mapped to URLs.
	chatContentRows []string

	width  int
	height int
	ready  bool
}

// New builds the root model. Event flow: the IRC clients publish to events,
// Init arms waitForEvents, and each received event re-arms it.
func New(cfg *config.Config, st *store.Store, events <-chan irc.Event, clients map[string]*irc.Client) *App {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.CharLimit = 440
	ti.Focus()
	return &App{
		cfg: cfg, st: st, events: events, clients: clients,
		input:       ti,
		members:     map[string]*memberSet{},
		imgCache:    map[string]string{},
		imgInflight: map[string]bool{},
	}
}

func (a *App) Init() tea.Cmd {
	return tea.Batch(waitForEvents(a.events), textinput.Blink)
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		a.ready = true
		a.resize()
		return a, nil
	case eventsClosedMsg:
		return a, tea.Quit
	case ircEventMsg:
		return a, tea.Batch(a.handleEvent(msg.ev), waitForEvents(a.events))
	case imageFetchedMsg:
		a.handleImageFetched(msg)
		return a, nil
	case tea.MouseMsg:
		// Left-click a sidebar buffer to switch to it, or a chat link to
		// open it in the browser. All other mouse input is swallowed so
		// clicks don't land in the input line.
		if msg.Action == tea.MouseActionRelease && msg.Button == tea.MouseButtonLeft {
			if i, ok := a.sidebarBufferAt(msg.X, msg.Y); ok && i != a.focus {
				a.focus = i
				// Mirror tab: re-lay out, since the nick pane
				// only appears for channel buffers.
				a.resize()
			} else if url, ok := a.linkAt(msg.X, msg.Y); ok {
				openURL(url)
			}
		}
		return a, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			if len(a.bufs) > 0 {
				a.focus = (a.focus + 1) % len(a.bufs)
				// resize re-lays out the columns (the nick pane only
				// appears for channels) and re-renders.
				a.resize()
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
		a.addLine(ev.Server, ev.Server, store.Line{At: time.Now(), Text: "connected", Kind: store.KindSystem})
	case irc.KindDisconnected:
		a.addLine(ev.Server, ev.Server, store.Line{At: time.Now(), Text: "disconnected", Kind: store.KindSystem})
	case irc.KindError:
		a.addLine(ev.Server, ev.Server, store.Line{At: time.Now(), Text: "error: " + ev.Text, Kind: store.KindSystem})
	case irc.KindMessage:
		cmd = a.handleMessage(ev.Server, ev.Msg)
	}
	a.bufs = a.st.Buffers()
	a.renderSidebar()
	if a.ready {
		a.renderChat()
		a.renderNicks()
	}
	return cmd
}

func (a *App) addLine(server, target string, l store.Line) {
	if target == "" {
		// Never create a nameless buffer (e.g. a PART with no channel
		// param); file it in the server window instead.
		target = server
	}
	isNew := !a.st.Has(server, target)
	buf := a.st.Get(server, target)
	if isNew {
		a.playbackHistory(server, target, buf)
	}
	buf.Append(l)
	if text, ok := history.FormatLine(l); ok {
		history.AppendLine(server, target, text)
	}
}

// playbackHistory replays the tail of the buffer's local log into a newly
// created buffer, oldest first. Played-back lines are already in the log,
// so they are appended directly without re-logging.
func (a *App) playbackHistory(server, target string, buf *store.Buffer) {
	for _, raw := range history.LastLines(server, target, a.cfg.HistoryLines()) {
		if l, ok := history.ParseLine(raw); ok {
			buf.Append(l)
		}
	}
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
		a.addLine(server, buf, store.Line{At: at, Nick: nick, Text: text, Kind: kind})
		if kind == store.KindChat {
			return a.queueImageFetches(server, buf, text)
		}
		return nil
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
		a.addLine(server, target, store.Line{At: at, Nick: nick, Text: text, Kind: kind})
	case "TOPIC":
		if len(m.Params) < 1 {
			return nil
		}
		ch := m.Params[0]
		topic := m.Trailing()
		a.st.Get(server, ch).Topic = topic
		a.addLine(server, ch, store.Line{At: at, Nick: nick, Text: "changed the topic to: " + topic, Kind: store.KindSystem})
	case "JOIN":
		if len(m.Params) < 1 {
			return nil
		}
		ch := m.Params[0]
		a.addLine(server, ch, store.Line{At: at, Nick: nick, Text: "joined " + ch, Kind: store.KindJoin})
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
		a.addLine(server, ch, store.Line{At: at, Nick: nick, Text: "left " + ch, Kind: store.KindPart})
		if isChannel(ch) {
			a.memberRemove(server, ch, nick)
			a.renderNicks()
			if nick == a.ownNick(server) {
				// We left: drop the buffer so the sidebar cleans up.
				a.st.Remove(server, ch)
			}
		}
	case "QUIT":
		a.addLine(server, server, store.Line{At: at, Nick: nick, Text: "quit: " + m.Trailing(), Kind: store.KindQuit})
		a.memberQuit(server, nick)
		a.renderNicks()
	case "NICK":
		newNick := m.Trailing()
		a.addLine(server, server, store.Line{At: at, Nick: nick, Text: "is now known as " + newNick, Kind: store.KindSystem})
		a.memberRename(server, nick, newNick)
		a.renderNicks()
	default:
		if isNumeric(m.Command) {
			// Keep the welcome/MOTD numerics, skip the noise.
			switch m.Command {
			case "001", "002", "003", "004", "005", "372", "375", "376":
				text := m.Trailing()
				if text == "" && len(m.Params) > 1 {
					text = strings.Join(m.Params[1:], " ")
				}
				a.addLine(server, server, store.Line{At: at, Text: text, Kind: store.KindSystem})
			case "353":
				// RPL_NAMREPLY: track membership, keep it out of scrollback.
				a.handleNames(server, m.Trailing(), m.Params)
			case "332":
				// RPL_TOPIC: params are [nick, channel], trailing is the topic.
				if len(m.Params) > 1 {
					a.st.Get(server, m.Params[1]).Topic = m.Trailing()
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
				a.addLine(server, server, store.Line{At: at, Text: text, Kind: store.KindSystem})
			}
		}
	}
	return nil
}

// queueImageFetches starts background fetches for image URLs in a chat
// message. Each fetch renders half-block art and reports back as an
// imageFetchedMsg; already-cached or in-flight URLs are skipped.
func (a *App) queueImageFetches(server, buf, text string) tea.Cmd {
	w := a.chat.Width - 2 // leave room for the indent
	if w > 48 {
		w = 48
	}
	if w < 8 {
		w = 8
	}
	var cmds []tea.Cmd
	for _, u := range img.FindURLs(text) {
		if _, ok := a.imgCache[u]; ok {
			continue
		}
		if a.imgInflight[u] {
			continue
		}
		a.imgInflight[u] = true
		url := u
		cmds = append(cmds, func() tea.Msg {
			m, err := img.Fetch(context.Background(), url)
			if err != nil {
				return imageFetchedMsg{server: server, buf: buf, url: url}
			}
			return imageFetchedMsg{server: server, buf: buf, url: url, art: img.Render(m, w, 12)}
		})
	}
	return tea.Batch(cmds...)
}

// handleImageFetched files a finished preview into scrollback. Failures
// (empty art) are cached so a URL isn't refetched every time it appears.
func (a *App) handleImageFetched(msg imageFetchedMsg) {
	delete(a.imgInflight, msg.url)
	a.imgCache[msg.url] = msg.art
	if msg.art == "" {
		return
	}
	// The buffer may be gone (parted while fetching): don't resurrect it.
	if !a.st.Has(msg.server, msg.buf) {
		return
	}
	a.addLine(msg.server, msg.buf, store.Line{At: time.Now(), Text: msg.art, Kind: store.KindImage})
	a.bufs = a.st.Buffers()
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
func (a *App) sidebarBufferAt(x, y int) (i int, ok bool) {
	sw := a.cfg.UI.SidebarWidth
	if sw < 16 {
		sw = 16
	}
	if x < 0 || x > sw || y < 0 {
		return 0, false // x == sw is the sidebar's right border
	}
	row := y + a.sidebar.YOffset // the sidebar viewport may be scrolled
	r := 0
	lastServer := ""
	for idx, buf := range a.bufs {
		if buf.Server != lastServer {
			if lastServer != "" {
				if r == row {
					return 0, false // separator line
				}
				r++
			}
			if r == row {
				return 0, false // server header line
			}
			r++
			lastServer = buf.Server
		}
		if r == row {
			return idx, true
		}
		r++
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

func (a *App) renderSidebar() {
	var b strings.Builder
	lastServer := ""
	for i, buf := range a.bufs {
		if buf.Server != lastServer {
			if lastServer != "" {
				b.WriteString("\n")
			}
			fmt.Fprintf(&b, "%s\n", lipgloss.NewStyle().Bold(true).Render(buf.Server))
			lastServer = buf.Server
		}
		line := "  " + buf.Name
		if i == a.focus {
			line = lipgloss.NewStyle().Reverse(true).Render("> " + buf.Name)
		}
		b.WriteString(line + "\n")
	}
	a.sidebar.SetContent(b.String())
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
		switch l.Kind {
		case store.KindChat:
			fmt.Fprintf(&b, "%s %s %s\n",
				tsStyle.Render(ts),
				nickStyle.Render(fmt.Sprintf("%-14s", l.Nick)),
				link.Style(irc.FormatText(l.Text), func(s string) string {
					return linkStyle.Render(s)
				}))
		case store.KindAction:
			fmt.Fprintf(&b, "%s %s\n", tsStyle.Render(ts),
				sysStyle.Render("* "+l.Nick+" ")+irc.FormatStyled(l.Text, sysStyle))
		case store.KindImage:
			// Half-block art rows are pre-wrapped; indent under the message.
			for _, row := range strings.Split(l.Text, "\n") {
				fmt.Fprintf(&b, "  %s\n", row)
			}
		default:
			fmt.Fprintf(&b, "%s %s\n", tsStyle.Render(ts), sysStyle.Render(l.Text))
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
	a.addLine(buf.Server, target, store.Line{At: time.Now(), Nick: a.ownNick(buf.Server), Text: v, Kind: store.KindChat})
	a.renderChat()
	return a.queueImageFetches(buf.Server, target, v)
}

func (a *App) ownNick(server string) string {
	for _, s := range a.cfg.Servers {
		if s.Name == server {
			return s.Nick
		}
	}
	return "me"
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
			a.addLine(buf.Server, to, store.Line{At: time.Now(), Nick: a.ownNick(buf.Server), Text: text, Kind: store.KindChat})
			a.bufs = a.st.Buffers()
			a.renderSidebar()
			a.renderChat()
			return a.queueImageFetches(buf.Server, to, text)
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
			a.bufs = a.st.Buffers()
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
