package config

import (
	"os"
	"path/filepath"
	"testing"
)

func loadTemp(t *testing.T, body string) *Config {
	t.Helper()
	body += "\n[[server]]\nname = \"s\"\nhost = \"example.com\"\nnick = \"n\"\n"
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestHistoryPlaybackNumber(t *testing.T) {
	cfg := loadTemp(t, "history_playback = 50\n")
	if got := cfg.HistoryLines(); got != 50 {
		t.Fatalf("HistoryLines = %d, want 50", got)
	}
}

func TestHistoryPlaybackString(t *testing.T) {
	cfg := loadTemp(t, "history_playback = \"50\"\n")
	if got := cfg.HistoryLines(); got != 50 {
		t.Fatalf("HistoryLines = %d, want 50", got)
	}
}

func TestHistoryPlaybackDefault(t *testing.T) {
	cfg := loadTemp(t, "")
	if got := cfg.HistoryLines(); got != DefaultHistoryLines {
		t.Fatalf("HistoryLines = %d, want default %d", got, DefaultHistoryLines)
	}
}

func TestHistoryPlaybackZero(t *testing.T) {
	cfg := loadTemp(t, "history_playback = 0\n")
	if got := cfg.HistoryLines(); got != 0 {
		t.Fatalf("HistoryLines = %d, want 0", got)
	}
}

func TestHistoryPlaybackBad(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(p, []byte("history_playback = \"lots\"\n[[server]]\nname = \"s\"\nhost = \"example.com\"\nnick = \"n\"\n"), 0o644)
	if _, err := Load(p); err == nil {
		t.Fatal("expected an error for a non-numeric history_playback")
	}
}

func TestLoggingToggle(t *testing.T) {
	for _, tc := range []struct {
		body string
		want bool
	}{
		{"", true},                   // default on
		{"logging = true\n", true},   // explicit on
		{"logging = false\n", false}, // off
	} {
		if got := loadTemp(t, tc.body).LoggingOn(); got != tc.want {
			t.Errorf("config %q: LoggingOn = %v, want %v", tc.body, got, tc.want)
		}
	}
	if got := loadTemp(t, "").ExpandedLogDir(); got != "" {
		t.Errorf("default ExpandedLogDir = %q, want empty", got)
	}
}

func TestLogDirExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	t.Setenv("IRCGO_TEST_LOGS", "/tmp/ircgo-test-logs")
	for _, tc := range []struct {
		body string
		want string
	}{
		{`log_dir = "~/irc-logs"`, filepath.Join(home, "irc-logs")},
		{`log_dir = "~"`, home},
		{`log_dir = "$IRCGO_TEST_LOGS"`, "/tmp/ircgo-test-logs"},
		{`log_dir = "/var/log/ircgo"`, "/var/log/ircgo"},
	} {
		if got := loadTemp(t, tc.body+"\n").ExpandedLogDir(); got != tc.want {
			t.Errorf("config %q: ExpandedLogDir = %q, want %q", tc.body, got, tc.want)
		}
	}
}

func TestDefaultPathNew(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	next := filepath.Join(home, ".config", "ircgo", "config.toml")
	if err := os.MkdirAll(filepath.Dir(next), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(next, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := DefaultPath(); got != next {
		t.Fatalf("DefaultPath = %q, want %q", got, next)
	}
}

func TestDefaultPathLegacy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	legacy := filepath.Join(home, ".config", "ircclient", "config.toml")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := DefaultPath(); got != legacy {
		t.Fatalf("DefaultPath = %q, want legacy %q", got, legacy)
	}
}

func TestDefaultPathMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	want := filepath.Join(home, ".config", "ircgo", "config.toml")
	if got := DefaultPath(); got != want {
		t.Fatalf("DefaultPath = %q, want %q", got, want)
	}
}
