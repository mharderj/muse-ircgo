package irc

import (
	"encoding/base64"
	"testing"
)

func TestParseBasic(t *testing.T) {
	m, err := Parse(":nick!user@host PRIVMSG #chan :hello world")
	if err != nil {
		t.Fatal(err)
	}
	if m.Command != "PRIVMSG" {
		t.Fatalf("command = %q", m.Command)
	}
	if m.Nick() != "nick" {
		t.Fatalf("nick = %q", m.Nick())
	}
	if len(m.Params) != 2 || m.Params[0] != "#chan" || m.Params[1] != "hello world" {
		t.Fatalf("params = %q", m.Params)
	}
}

func TestParseTags(t *testing.T) {
	m, err := Parse(`@time=2026-09-23T12:34:56.789Z;account=matt :nick!u@h PRIVMSG #c :hi`)
	if err != nil {
		t.Fatal(err)
	}
	if m.Tags["account"] != "matt" {
		t.Fatalf("tags = %v", m.Tags)
	}
	ts := m.Time()
	if ts.IsZero() {
		t.Fatal("expected server-time to parse")
	}
	if ts.Year() != 2026 || ts.Hour() != 12 || ts.Minute() != 34 {
		t.Fatalf("time = %v", ts)
	}
}

func TestParseTagEscapes(t *testing.T) {
	m, err := Parse(`@foo=a\:b\sc\\d :srv NOTICE * :x`)
	if err != nil {
		t.Fatal(err)
	}
	if m.Tags["foo"] != "a;b c\\d" {
		t.Fatalf("unescaped = %q", m.Tags["foo"])
	}
}

func TestParseNoPrefix(t *testing.T) {
	m, err := Parse("PING :12345")
	if err != nil {
		t.Fatal(err)
	}
	if m.Command != "PING" || m.Trailing() != "12345" {
		t.Fatalf("got %+v", m)
	}
}

func TestParseCapLS(t *testing.T) {
	m, err := Parse("CAP * LS :multi-prefix sasl=PLAIN away-notify")
	if err != nil {
		t.Fatal(err)
	}
	if m.Command != "CAP" || m.Params[1] != "LS" {
		t.Fatalf("got %+v", m)
	}
}

func TestSaslChunks(t *testing.T) {
	// 300 raw bytes -> exactly 400 b64 chars -> one chunk plus "+".
	raw := make([]byte, 300)
	b64 := base64.StdEncoding.EncodeToString(raw)
	if len(b64) != 400 {
		t.Fatalf("b64 len = %d", len(b64))
	}
	chunks := saslChunks(b64)
	if len(chunks) != 2 || chunks[0] != b64 || chunks[1] != "+" {
		t.Fatalf("chunks = %q", chunks)
	}

	// Short blob -> single chunk, no terminator.
	short := saslChunks(base64.StdEncoding.EncodeToString([]byte("hi")))
	if len(short) != 1 {
		t.Fatalf("chunks = %q", short)
	}
}
