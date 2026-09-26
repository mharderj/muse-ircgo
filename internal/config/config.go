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
