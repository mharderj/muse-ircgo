package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const testServerBlock = "\n[[server]]\nname = \"srv\"\nhost = \"example.com\"\nnick = \"me\"\n"

func TestWriteLastBufferRoundTrip(t *testing.T) {
	p := writeTempConfig(t, "# a comment\n[ui]\nsidebar_width = 30\n"+testServerBlock)
	if err := WriteLastBuffer(p, "TheZoneIRC", "#linux"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "# a comment") {
		t.Fatalf("comment lost:\n%s", s)
	}
	if c := strings.Count(s, "last_buffer"); c != 1 {
		t.Fatalf("want exactly one last_buffer line, got %d:\n%s", c, s)
	}
	// Top-level keys must precede tables in TOML.
	if strings.Index(s, "last_buffer") > strings.Index(s, "[ui]") {
		t.Fatalf("last_buffer not before first table:\n%s", s)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.LastBuffer) != 2 || cfg.LastBuffer[0] != "TheZoneIRC" || cfg.LastBuffer[1] != "#linux" {
		t.Fatalf("LastBuffer = %q", cfg.LastBuffer)
	}
}

func TestWriteLastBufferReplacesExisting(t *testing.T) {
	p := writeTempConfig(t, "last_buffer = [\"old\", \"#gone\"]\n[ui]\n"+testServerBlock)
	if err := WriteLastBuffer(p, "TheZoneIRC", "#linux"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if c := strings.Count(s, "last_buffer"); c != 1 {
		t.Fatalf("want one last_buffer line, got %d:\n%s", c, s)
	}
	if !strings.Contains(s, `last_buffer = ["TheZoneIRC", "#linux"]`) {
		t.Fatalf("not replaced:\n%s", s)
	}
	if strings.Contains(s, "#gone") {
		t.Fatalf("old value lingered:\n%s", s)
	}
}

func TestWriteLastBufferNoTables(t *testing.T) {
	p := writeTempConfig(t, "# only a comment\n")
	if err := WriteLastBuffer(p, "srv", "bob"); err != nil {
		t.Fatal(err)
	}
	// No [[server]] means Load fails validation; just check the line lands.
	data, _ := os.ReadFile(p)
	if !strings.Contains(string(data), `last_buffer = ["srv", "bob"]`) {
		t.Fatalf("missing:\n%s", data)
	}
}

func TestWriteLastBufferEscapesNames(t *testing.T) {
	p := writeTempConfig(t, testServerBlock)
	if err := WriteLastBuffer(p, `srv"x`, `#a\b`); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.LastBuffer) != 2 || cfg.LastBuffer[0] != `srv"x` || cfg.LastBuffer[1] != `#a\b` {
		t.Fatalf("LastBuffer = %q", cfg.LastBuffer)
	}
}

func TestWriteLastBufferMissingFile(t *testing.T) {
	if err := WriteLastBuffer(filepath.Join(t.TempDir(), "nope.toml"), "s", "b"); err == nil {
		t.Fatal("want error for missing file")
	}
}
