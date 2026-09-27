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

func TestLoggingDefaultsOn(t *testing.T) {
	cfg := loadTemp(t, "")
	if !cfg.LoggingOn() {
		t.Fatal("LoggingOn = false, want true (default)")
	}
	if cfg.ExpandedLogDir() != "" {
		t.Fatalf("ExpandedLogDir = %q, want empty (default)", cfg.ExpandedLogDir())
	}
}

func TestLoggingExplicitOn(t *testing.T) {
	cfg := loadTemp(t, "logging = true\n")
	if !cfg.LoggingOn() {
		t.Fatal("LoggingOn = false, want true")
	}
}

func TestLoggingOff(t *testing.T) {
	cfg := loadTemp(t, "logging = false\n")
	if cfg.LoggingOn() {
		t.Fatal("LoggingOn = true, want false")
	}
}

func TestLogDirTildeExpansion(t *testing.T) {
	cfg := loadTemp(t, "log_dir = \"~/irc-logs\"\n")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	if want := filepath.Join(home, "irc-logs"); cfg.ExpandedLogDir() != want {
		t.Fatalf("ExpandedLogDir = %q, want %q", cfg.ExpandedLogDir(), want)
	}
}

func TestLogDirBareTilde(t *testing.T) {
	cfg := loadTemp(t, "log_dir = \"~\"\n")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	if cfg.ExpandedLogDir() != home {
		t.Fatalf("ExpandedLogDir = %q, want %q", cfg.ExpandedLogDir(), home)
	}
}

func TestLogDirEnvExpansion(t *testing.T) {
	t.Setenv("IRCGO_TEST_LOGS", "/tmp/ircgo-test-logs")
	cfg := loadTemp(t, "log_dir = \"$IRCGO_TEST_LOGS\"\n")
	if cfg.ExpandedLogDir() != "/tmp/ircgo-test-logs" {
		t.Fatalf("ExpandedLogDir = %q, want env expansion", cfg.ExpandedLogDir())
	}
}

func TestLogDirAbsoluteUntouched(t *testing.T) {
	cfg := loadTemp(t, "log_dir = \"/var/log/ircgo\"\n")
	if cfg.ExpandedLogDir() != "/var/log/ircgo" {
		t.Fatalf("ExpandedLogDir = %q, want untouched", cfg.ExpandedLogDir())
	}
}
