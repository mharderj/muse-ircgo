# muse-ircgo
Vibed to hell app, testing out MuseAI as a novice

A terminal IRC client in Go, built on [Bubble Tea](https://github.com/charmbracelet/bubbletea).
Split-view TUI, TLS connections, and first-class ZNC/bouncer support.

## Features

- Split-view TUI: buffer sidebar, chat pane, input, status bar
- Mouse: left-click a buffer in the sidebar to switch to it
- Channel nick list (ops first, then voiced, then alphabetical)
- CTCP queries answered automatically (`VERSION`, `PING`, `TIME`, …)
- ZNC/bouncer support with `server-time` backlog timestamps
- mIRC formatting rendered inline (colors, bold, italic, underline)
- Lossless event delivery: replay bursts apply backpressure instead of dropping messages
- Channel topic panel pinned above the chat feed (`/topic [new topic]`)
- Inline image previews: image URLs in chat render as half-block art
  (chafa-style), fetched in the background — works in any terminal
- Clickable links: URLs render underlined; click one to open it in your browser
- Channel logging: each channel/query buffer logs to one file under
  `~/.local/ircgo/logs/<server>/<channel>.log`; the last `history_playback`
  lines (default 50, set `history_playback = 0` to disable) replay into a
  buffer when it is first opened
- TLS, with `insecure_skip_verify` for self-signed certs
- Auto-reconnect with backoff when the connection drops
- Nick collision fallback: a `433` at connect retries as `nick_`
- Kicks, mode changes, and channel invites are shown (op/voice changes
  update the nick list live)
- Buffers match case-insensitively: a reply from `Belial` lands in your
  existing `belial` query instead of opening a second buffer
- Unread markers: buffers with unseen activity show a bold yellow `*` in
  the sidebar, cleared when you switch to them

## Build

```sh
go build -o ircgo ./cmd/ircgo
```

## Run

```sh
mkdir -p ~/.config/ircclient
cp config.example.toml ~/.config/ircclient/config.toml
# edit it: nicks, servers, bouncer credentials
./ircgo
# or: ./ircgo -config /path/to/config.toml
```

Stuck at "connecting…"? Run with `-debug` and check the log — it records
each connection step (dial, TLS handshake, registration) with passwords
redacted:

```sh
./ircgo -debug
tail -f ~/.config/ircclient/debug.log
```

## Layout

```
cmd/ircgo            entrypoint: loads config, starts clients + TUI
internal/config      TOML config loading and validation
internal/irc         protocol layer
  message.go           IRCv3 parser (tags, prefix, params)
  ctcp.go              CTCP detection and parsing
  conn.go              TCP / TLS dial, line I/O
  client.go            registration, read loop, event pump
  caps.go              CAP LS/REQ/ACK negotiation
  sasl.go              SASL PLAIN
  debug.go             redacted connection diagnostics
internal/store       per-buffer scrollback (channels, queries, server windows)
internal/img         image URL detection, background fetch, half-block rendering
internal/ui          Bubble Tea models: sidebar, chat viewport, nick list,
                   input, status bar
  members.go         channel membership tracking (NAMES, JOIN/PART/QUIT/NICK)
internal/version     release version (`-version`, CTCP VERSION replies)
```

## Keys

- `tab` — cycle buffers (channels, queries, server windows)
- `alt+1` … `alt+9` — jump to a buffer by its sidebar position, top to bottom
- `enter` — send
- mouse: left-click a buffer in the sidebar to switch to it

Quit with `/quit` (or `/q`) — `ctrl+c` no longer closes the app.

Slash commands: `/join #chan`, `/part [#chan]`, `/msg nick text`, `/me text`,
`/nick newnick`, `/topic [new topic]`, `/ctcp nick command [args]`,
`/quit` (or `/q`).

CTCP is supported: `/me`-style actions render inline, incoming queries
(`VERSION`, `PING`, `TIME`, `FINGER`, `USERINFO`, `CLIENTINFO`) are answered
automatically via NOTICE, and CTCP exchanges stay in the active buffer.
`./ircgo -version` prints the release (currently v0.1.10).

A nick list appears on the right for channel buffers (ops first, then
alphabetical), built from NAMES replies and JOIN/PART/QUIT/NICK updates.
It hides on narrow terminals.

## ZNC notes

- Auth uses `PASS user/network:password`, assembled from the `username`,
  `network`, and `password` config fields. SASL PLAIN is available with
  `sasl = true`.
- The client requests `server-time` and `znc.in/server-time-iso` so backlog
  replays render with their original timestamps.
- Messages from `*status` land in the server window.

## Roadmap

- Reconnect with backoff
- Scrollback-preserving scroll (pgup/pgdn without snap-to-bottom)
- Scroll wheel: scroll chat history
- SASL EXTERNAL (client certs)
- Configurable keybinds and themes
