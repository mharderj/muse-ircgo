// Package config loads ircgo's TOML configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is the top-level ircgo configuration.
type Config struct {
	Servers []Server `toml:"server"`
	UI      UIConfig `toml:"ui"`

	// HistoryPlayback is the history_playback setting: how many lines of
	// local channel log are replayed into a buffer when it is created.
	HistoryPlayback PlaybackLines `toml:"history_playback"`

	// Logging is the logging setting: nil (unset) or true enables channel
	// logging; false disables it entirely — no log writes, no history
	// playback, no query-buffer restore from logs.
	Logging *bool `toml:"logging"`

	// LogDir overrides the channel log directory
	// (default ~/.local/ircgo/logs). "~" and environment variables are
	// expanded.
	LogDir string `toml:"log_dir"`

	// LastBuffer is the buffer focused when the app last quit, as
	// [server, buffer]. The app writes it on exit; it is read on startup
	// to restore focus.
	LastBuffer []string `toml:"last_buffer"`

	// BufferOrder is the custom sidebar order as [server, buffer] pairs,
	// set by drag-and-drop reordering. The app rewrites it whenever the
	// order changes; it is read on startup to restore positions.
	BufferOrder [][]string `toml:"buffer_order"`
}

// DefaultHistoryLines is the playback depth when history_playback is unset.
const DefaultHistoryLines = 50

// PlaybackLines is a history_playback value: a line count given as a TOML
// number or a numeric string (both 50 and "50" work).
type PlaybackLines struct {
	Set   bool
	Lines int
}

// UnmarshalTOML implements toml.Unmarshaler.
func (p *PlaybackLines) UnmarshalTOML(v interface{}) error {
	switch n := v.(type) {
	case int64:
		p.Set, p.Lines = true, int(n)
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(n))
		if err != nil {
			return fmt.Errorf("history_playback: %q is not a number", n)
		}
		p.Set, p.Lines = true, i
	default:
		return fmt.Errorf("history_playback: expected a number, got %T", v)
	}
	if p.Lines < 0 {
		p.Lines = 0
	}
	return nil
}

// HistoryLines returns the configured playback depth in lines.
func (c *Config) HistoryLines() int {
	if !c.HistoryPlayback.Set {
		return DefaultHistoryLines
	}
	return c.HistoryPlayback.Lines
}

// LoggingOn reports whether channel logging is enabled. Logging defaults
// to on; only an explicit `logging = false` turns it off.
func (c *Config) LoggingOn() bool {
	return c.Logging == nil || *c.Logging
}

// ExpandedLogDir returns the configured log_dir with "~" and environment
// variables expanded, or "" when log_dir is unset (the caller keeps the
// default directory in that case).
func (c *Config) ExpandedLogDir() string {
	if c.LogDir == "" {
		return ""
	}
	dir := os.ExpandEnv(c.LogDir)
	if home, err := os.UserHomeDir(); err == nil {
		switch {
		case dir == "~":
			dir = home
		case strings.HasPrefix(dir, "~/"):
			dir = filepath.Join(home, dir[2:])
		}
	}
	return dir
}

// UIConfig holds cosmetic preferences.
type UIConfig struct {
	TimestampFormat string `toml:"timestamp_format"`
	SidebarWidth    int    `toml:"sidebar_width"`
}

// Server is a single IRC server or bouncer connection.
type Server struct {
	Name               string   `toml:"name"`
	Host               string   `toml:"host"`
	Port               int      `toml:"port"`
	TLS                bool     `toml:"tls"`
	InsecureSkipVerify bool     `toml:"insecure_skip_verify"`
	Nick               string   `toml:"nick"`
	Username           string   `toml:"username"` // bouncer account, e.g. ZNC login
	Network            string   `toml:"network"`  // bouncer network, e.g. ZNC network
	Password           string   `toml:"password"`
	SASL               bool     `toml:"sasl"`
	Channels           []string `toml:"channels"`
}

// Addr returns host:port, filling in the conventional default port.
func (s Server) Addr() string {
	port := s.Port
	if port == 0 {
		if s.TLS {
			port = 6697
		} else {
			port = 6667
		}
	}
	return fmt.Sprintf("%s:%d", s.Host, port)
}

// Pass builds the PASS command payload. Bouncers like ZNC expect
// "user/network:password"; plain servers just get the password.
func (s Server) Pass() string {
	if s.Username != "" && s.Network != "" {
		return fmt.Sprintf("%s/%s:%s", s.Username, s.Network, s.Password)
	}
	if s.Username != "" {
		return fmt.Sprintf("%s:%s", s.Username, s.Password)
	}
	return s.Password
}

// HasPass reports whether a PASS command should be sent.
func (s Server) HasPass() bool { return s.Password != "" }

// Load reads and validates the TOML config at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.UI.TimestampFormat == "" {
		c.UI.TimestampFormat = "15:04"
	}
	if c.UI.SidebarWidth == 0 {
		c.UI.SidebarWidth = 26
	}
	for i := range c.Servers {
		if c.Servers[i].Name == "" {
			c.Servers[i].Name = c.Servers[i].Host
		}
	}
}

func (c *Config) validate() error {
	if len(c.Servers) == 0 {
		return fmt.Errorf("config: define at least one [[server]]")
	}
	for _, s := range c.Servers {
		if s.Host == "" {
			return fmt.Errorf("config: server %q is missing host", s.Name)
		}
		if s.Nick == "" {
			return fmt.Errorf("config: server %q is missing nick", s.Name)
		}
	}
	return nil
}

// DefaultPath returns ~/.config/ircclient/config.toml.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.toml"
	}
	return filepath.Join(home, ".config", "ircclient", "config.toml")
}

// tomlString quotes s as a TOML basic string.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// isTopLevelKey reports whether a trimmed config line sets key ("key = ...").
func isTopLevelKey(line, key string) bool {
	rest, ok := strings.CutPrefix(line, key)
	if !ok {
		return false
	}
	rest = strings.TrimSpace(rest)
	return strings.HasPrefix(rest, "=")
}

// rewriteTopLevelArray replaces (or inserts) a top-level `key = [...]`
// line, preserving comments and formatting: the existing line is replaced
// in place, or a new one is inserted before the first table header
// (top-level keys must precede tables in TOML). A misplaced key inside a
// table is dropped and re-written at the top level.
func rewriteTopLevelArray(path, key, newLine string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	mode := os.FileMode(0o600)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	var out []string
	inTable, written := false, false
	for _, l := range strings.SplitAfter(string(data), "\n") {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "[") {
			if !written {
				out = append(out, newLine+"\n")
				written = true
			}
			inTable = true
			out = append(out, l)
			continue
		}
		if isTopLevelKey(t, key) {
			if !inTable && !written {
				out = append(out, newLine+"\n")
				written = true
			}
			continue // drop the old line
		}
		out = append(out, l)
	}
	if !written {
		if n := len(out); n > 0 && !strings.HasSuffix(out[n-1], "\n") {
			out[n-1] += "\n"
		}
		out = append(out, newLine+"\n")
	}
	return os.WriteFile(path, []byte(strings.Join(out, "")), mode)
}

// WriteLastBuffer records the focused buffer in the config file, preserving
// comments and formatting: the existing last_buffer line is replaced in
// place, or a new one is inserted before the first table header (top-level
// keys must precede tables in TOML). A misplaced last_buffer inside a table
// is dropped and re-written at the top level.
func WriteLastBuffer(path, server, name string) error {
	newLine := "last_buffer = [" + tomlString(server) + ", " + tomlString(name) + "]"
	return rewriteTopLevelArray(path, "last_buffer", newLine)
}

// WriteBufferOrder records the sidebar buffer order in the config file as
// buffer_order = [[server, name], ...], preserving comments and formatting
// the same way WriteLastBuffer does.
func WriteBufferOrder(path string, order [][2]string) error {
	var b strings.Builder
	b.WriteString("buffer_order = [")
	for i, p := range order {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString("[" + tomlString(p[0]) + ", " + tomlString(p[1]) + "]")
	}
	b.WriteString("]")
	return rewriteTopLevelArray(path, "buffer_order", b.String())
}
