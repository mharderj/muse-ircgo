package ui

import (
	"strings"
	"testing"

	"ircgo/internal/config"
	"ircgo/internal/store"
)

// The frame must fill exactly the terminal height, with a single continuous
// divider row (corner joints ┘ and └) above the input line.
func TestViewFrameHeightWithTopic(t *testing.T) {
	a := New(&config.Config{}, store.New(), nil, nil)
	a.addLine("srv", "srv", store.Line{Text: "hi"})
	a.addLine("srv", "#a", store.Line{Text: "hi"})
	a.bufs = a.st.Buffers()
	a.focus = 1 // channel: topic panel visible
	a.width, a.height = 100, 30
	a.ready = true
	a.resize()

	view := a.View()
	if rows := strings.Count(view, "\n") + 1; rows != 30 {
		t.Fatalf("view rows = %d, want 30", rows)
	}
	dividers := 0
	for _, row := range strings.Split(view, "\n") {
		if strings.Contains(row, "┘") && strings.Contains(row, "└") {
			dividers++
		}
	}
	if dividers != 1 {
		t.Fatalf("bottom divider rows = %d, want 1", dividers)
	}
}

func TestViewFrameHeightWithoutTopic(t *testing.T) {
	a := New(&config.Config{}, store.New(), nil, nil)
	a.addLine("srv", "srv", store.Line{Text: "hi"})
	a.bufs = a.st.Buffers()
	a.focus = 0 // server window: no topic panel, no nick pane
	a.width, a.height = 100, 30
	a.ready = true
	a.resize()

	view := a.View()
	if rows := strings.Count(view, "\n") + 1; rows != 30 {
		t.Fatalf("view rows = %d, want 30", rows)
	}
}
