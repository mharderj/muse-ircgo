package config

import (
	"os"
	"strings"
	"testing"
)

func TestWriteBufferOrderRoundTrip(t *testing.T) {
	p := writeTempConfig(t, "# a comment\n[ui]\nsidebar_width = 30\n"+testServerBlock)
	order := [][2]string{{"TheZoneIRC", "#idlewhores"}, {"TheZoneIRC", "#TheZone"}, {"TheZoneIRC", "belial"}}
	if err := WriteBufferOrder(p, order); err != nil {
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
	if c := strings.Count(s, "buffer_order"); c != 1 {
		t.Fatalf("want exactly one buffer_order line, got %d:\n%s", c, s)
	}
	if strings.Index(s, "buffer_order") > strings.Index(s, "[ui]") {
		t.Fatalf("buffer_order not before first table:\n%s", s)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.BufferOrder) != 3 {
		t.Fatalf("BufferOrder = %q", cfg.BufferOrder)
	}
	for i, want := range order {
		if cfg.BufferOrder[i][0] != want[0] || cfg.BufferOrder[i][1] != want[1] {
			t.Fatalf("BufferOrder[%d] = %q, want %q", i, cfg.BufferOrder[i], want)
		}
	}
}

func TestWriteBufferOrderReplacesExisting(t *testing.T) {
	p := writeTempConfig(t, "buffer_order = [[\"old\", \"#gone\"]]\n[ui]\n"+testServerBlock)
	if err := WriteBufferOrder(p, [][2]string{{"srv", "#a"}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if c := strings.Count(s, "buffer_order"); c != 1 {
		t.Fatalf("want one buffer_order line, got %d:\n%s", c, s)
	}
	if !strings.Contains(s, `buffer_order = [["srv", "#a"]]`) {
		t.Fatalf("not replaced:\n%s", s)
	}
}

func TestWriteBufferOrderEscapes(t *testing.T) {
	p := writeTempConfig(t, testServerBlock)
	if err := WriteBufferOrder(p, [][2]string{{`we"ird`, "#a"}}); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.BufferOrder) != 1 || cfg.BufferOrder[0][0] != `we"ird` {
		t.Fatalf("BufferOrder = %q", cfg.BufferOrder)
	}
}
