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
