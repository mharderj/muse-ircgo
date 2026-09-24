# muse-ircgo
Vibed to hell app, testing out MuseAI as a novice

A terminal IRC client in Go, built on [Bubble Tea](https://github.com/charmbracelet/bubbletea).
Split-view TUI, TLS connections, and first-class ZNC/bouncer support.

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
  conn.go              TCP / TLS dial, line I/O
  client.go            registration, read loop, event pump
  caps.go              CAP LS/REQ/ACK negotiation
  sasl.go              SASL PLAIN
internal/store       per-buffer scrollback (channels, queries, server windows)
internal/ui          Bubble Tea models: sidebar, chat viewport, input, status bar
```

## Keys

- `tab` — cycle buffers (channels, queries, server windows)
- `enter` — send
- `ctrl+c` — quit

Slash commands: `/join #chan`, `/part [#chan]`, `/msg nick text`, `/quit`.

## ZNC notes

- Auth uses `PASS user/network:password`, assembled from the `username`,
  `network`, and `password` config fields. SASL PLAIN is available with
  `sasl = true`.
- The client requests `server-time` and `znc.in/server-time-iso` so backlog
  replays render with their original timestamps.
- Messages from `*status` land in the server window.

## Roadmap

- Reconnect with backoff
- Nick list pane and `/names` tracking
- Scrollback-preserving scroll (pgup/pgdn without snap-to-bottom)
- Mouse support, clickable buffer list
- SASL EXTERNAL (client certs)
- Configurable keybinds and themes
