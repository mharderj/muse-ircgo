package history

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ircgo/internal/store"
)

func TestFormatParseRoundTrip(t *testing.T) {
	at := time.Date(2026, 9, 26, 14, 18, 4, 0, time.Local)
	tests := []struct {
		line     store.Line
		form     string
		kind     store.Kind
		nick     string
		wantText string
	}{
		{store.Line{At: at, Nick: "bob", Text: "hello", Kind: store.KindChat},
			"[2026-09-26 14:18:04] <bob> hello", store.KindChat, "bob", "hello"},
		{store.Line{At: at, Nick: "bob", Text: "waves", Kind: store.KindAction},
			"[2026-09-26 14:18:04] * bob waves", store.KindAction, "bob", "waves"},
		{store.Line{At: at, Nick: "ns", Text: "identify", Kind: store.KindNotice},
			"[2026-09-26 14:18:04] -ns- identify", store.KindNotice, "ns", "identify"},
		{store.Line{At: at, Text: "connected", Kind: store.KindSystem},
			"[2026-09-26 14:18:04] -- connected", store.KindSystem, "", "connected"},
		// Joins/parts/quits log as system lines with the nick folded in.
		{store.Line{At: at, Nick: "bob", Text: "joined #a", Kind: store.KindJoin},
			"[2026-09-26 14:18:04] -- bob joined #a", store.KindSystem, "", "bob joined #a"},
	}
	for _, tc := range tests {
		form, ok := FormatLine(tc.line)
		if !ok || form != tc.form {
			t.Fatalf("FormatLine(%+v) = %q, %v; want %q", tc.line, form, ok, tc.form)
		}
		got, ok := ParseLine(form)
		if !ok {
			t.Fatalf("ParseLine(%q) failed", form)
		}
		if got.Kind != tc.kind || got.Nick != tc.nick || got.Text != tc.wantText || !got.At.Equal(tc.line.At) {
			t.Fatalf("ParseLine(%q) = %+v; want kind=%d nick=%q text=%q", form, got, tc.kind, tc.nick, tc.wantText)
		}
	}
}

func TestFormatLineSkipsImage(t *testing.T) {
	// Art attached to a message line is transient and never logged.
	l := store.Line{Kind: store.KindChat, Nick: "bob", Text: "see pic"}
	l.Art = []string{"ART"}
	form, ok := FormatLine(l)
	if !ok {
		t.Fatal("chat line with art should still be logged")
	}
	if strings.Contains(form, "ART") {
		t.Fatalf("art was logged: %q", form)
	}
	if _, ok := ParseLine("not a log line"); ok {
		t.Fatal("ParseLine accepted garbage")
	}
}

func withTempDir(t *testing.T) {
	t.Helper()
	old := LogDir
	LogDir = t.TempDir()
	t.Cleanup(func() { LogDir = old })
}

func TestAppendLineDedupes(t *testing.T) {
	withTempDir(t)
	AppendLine("srv", "#a", "[2026-09-26 14:18:04] <bob> hi")
	AppendLine("srv", "#a", "[2026-09-26 14:18:04] <bob> hi") // replay dup
	AppendLine("srv", "#a", "[2026-09-26 14:18:05] <bob> hi") // different ts: new
	data, err := os.ReadFile(Path("srv", "#a"))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), "\n"); n != 2 {
		t.Fatalf("log has %d lines, want 2:\n%s", n, data)
	}
}

func TestLastLinesTail(t *testing.T) {
	withTempDir(t)
	for i := 0; i < 10; i++ {
		AppendLine("srv", "#a", "[2026-09-26 14:18:04] <bob> "+strings.Repeat("x", i+1))
	}
	got := LastLines("srv", "#a", 3)
	if len(got) != 3 {
		t.Fatalf("LastLines = %d lines, want 3", len(got))
	}
	if !strings.HasSuffix(got[2], strings.Repeat("x", 10)) {
		t.Fatalf("last line = %q", got[2])
	}
	if got := LastLines("srv", "#missing", 5); len(got) != 0 {
		t.Fatalf("LastLines on missing file = %v", got)
	}
}

func TestPathSanitizes(t *testing.T) {
	withTempDir(t)
	p := Path("srv", "#a")
	if !strings.HasSuffix(p, filepath.Join("srv", "#a.log")) {
		t.Fatalf("Path = %q", p)
	}
	if strings.Contains(Path("a/b", ".."), "..") {
		t.Fatalf("Path not sanitized: %q", Path("a/b", ".."))
	}
}

func TestLocalDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if want := filepath.Join(home, ".local", "ircgo"); LocalDir() != want {
		t.Fatalf("LocalDir = %q, want %q", LocalDir(), want)
	}
	if want := filepath.Join(home, ".local", "ircgo", "logs"); defaultLogDir() != want {
		t.Fatalf("defaultLogDir = %q, want %q", defaultLogDir(), want)
	}
}
