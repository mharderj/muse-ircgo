// Package ui is the Bubble Tea interface: a sidebar of buffers, a chat
// viewport, an input line, and a status bar.
package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"ircgo/internal/config"
	"ircgo/internal/irc"
	"ircgo/internal/store"
	"ircgo/internal/version"
)

// ircEventMsg carries a protocol event into the Update loop.
type ircEventMsg struct{ ev irc.Event }

// eventsClosedMsg fires when the event channel is closed.
type eventsClosedMsg struct{}

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
			Border(lipgloss.NormalBorder(), false, true, false, false).
			BorderForeground(lipgloss.Color("8"))
	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7")).
			Background(lipgloss.Color("4"))
	nicksStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(lipgloss.Color("8"))
)

// App is the root Bubble Tea model.
type App struct {
	cfg     *config.Config
	st      *store.Store
	events  <-chan irc.Event
	clients map[string]*irc.Client

	bufs  []*store.Buffer
	focus int

	members map[string]*memberSet

	sidebar viewport.Model
	chat    viewport.Model
	nicks   viewport.Model
	input   textinput.Model

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
		input:   ti,
		members: map[string]*memberSet{},
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
		a.handleEvent(msg.ev)
		return a, waitForEvents(a.events)
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

// handleEvent files a protocol event into scrollback and re-renders.
func (a *App) handleEvent(ev irc.Event) {
	switch ev.Kind {
	case irc.KindConnected:
		a.addLine(ev.Server, ev.Server, store.Line{At: time.Now(), Text: "connected", Kind: store.KindSystem})
	case irc.KindDisconnected:
		a.addLine(ev.Server, ev.Server, store.Line{At: time.Now(), Text: "disconnected", Kind: store.KindSystem})
	case irc.KindError:
		a.addLine(ev.Server, ev.Server, store.Line{At: time.Now(), Text: "error: " + ev.Text, Kind: store.KindSystem})
	case irc.KindMessage:
		a.handleMessage(ev.Server, ev.Msg)
	}
	a.bufs = a.st.Buffers()
	a.renderSidebar()
	if a.ready {
		a.renderChat()
		a.renderNicks()
	}
}

func (a *App) addLine(server, target string, l store.Line) {
	if target == "" {
		// Never create a nameless buffer (e.g. a PART with no channel
		// param); file it in the server window instead.
		target = server
	}
	a.st.Get(server, target).Append(l)
}

func (a *App) handleMessage(server string, m *irc.Message) {
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
			return
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
			// CTCP replies read better in the sender's buffer than
			// buried in the server window.
			text = "CTCP " + cmd
			if args != "" {
				text += ": " + args
			}
			if target == server && nick != "" {
				target = nick
			}
		}
		a.addLine(server, target, store.Line{At: at, Nick: nick, Text: text, Kind: kind})
	case "JOIN":
		if len(m.Params) < 1 {
			return
		}
		ch := m.Params[0]
		// A bare "#" or "&" is never a real channel; ZNC's chansaver
		// can replay one if it was ever saved by accident. Ignore it
		// rather than opening a junk buffer.
		if ch == "#" || ch == "&" {
			return
		}
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
				l.Text)
		case store.KindAction:
			fmt.Fprintf(&b, "%s %s\n", tsStyle.Render(ts),
				sysStyle.Render("* "+l.Nick+" "+l.Text))
		default:
			fmt.Fprintf(&b, "%s %s\n", tsStyle.Render(ts), sysStyle.Render(l.Text))
		}
	}
	a.chat.SetContent(b.String())
	a.chat.GotoBottom()
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
	chatH := a.height - 2 // input line + status line
	if chatH < 5 {
		chatH = 5
	}
	a.sidebar = viewport.New(sw, chatH)
	a.chat = viewport.New(chatW, chatH)
	a.nicks = viewport.New(max(nw-1, 1), chatH)
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
		a.chat.View(),
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
	return nil
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
			a.addLine(buf.Server, to, store.Line{At: time.Now(), Nick: a.ownNick(buf.Server), Text: "CTCP " + cmd + args + " -> " + to, Kind: store.KindNotice})
			a.bufs = a.st.Buffers()
			a.renderSidebar()
			a.renderChat()
		}
	case "/quit", "/q":
		return tea.Quit
	}
	return nil
}
