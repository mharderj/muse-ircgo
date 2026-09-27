package tea

import (
	"fmt"
	"strings"
	"testing"
)

// A burst of SGR mouse motion events split across 256-byte reads (the way
// readAnsiInputs chunks stdin) must all parse as MouseMsg. Regression test:
// a read ending mid-sequence (e.g. "\x1b[<3") used to leak as Alt+[ plus
// literal "[<35;..;..M" runes into the app's input.
func TestBurstMotionParsing(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&sb, "\x1b[<35;%d;%dM", 90-i, 25-i/4)
	}
	data := []byte(sb.String())

	var chunks [][]byte
	for len(data) > 0 {
		n := 256
		if n > len(data) {
			n = len(data)
		}
		chunks = append(chunks, data[:n])
		data = data[n:]
	}

	var leftover []byte
	mouse, leaked := 0, 0
	for ci, c := range chunks {
		b := append(append([]byte{}, leftover...), c...)
		leftover = nil
		canMore := ci < len(chunks)-1
		for i := 0; i < len(b); {
			w, msg := detectOneMsg(b[i:], canMore)
			if w == 0 {
				leftover = append([]byte{}, b[i:]...)
				break
			}
			if _, ok := msg.(MouseMsg); ok {
				mouse++
			} else {
				leaked++
				t.Logf("leaked %T: %v", msg, msg)
			}
			i += w
		}
	}
	if leftover != nil {
		t.Fatalf("leftover bytes after final chunk: %q", leftover)
	}
	if leaked > 0 {
		t.Fatalf("leaked %d non-mouse messages", leaked)
	}
	if mouse != 60 {
		t.Fatalf("got %d mouse events, want 60", mouse)
	}
}

// A partial SGR mouse sequence at the end of a full read must wait for more
// input (w=0), not parse as Alt+[.
func TestPartialSGRMouseWaits(t *testing.T) {
	for _, tc := range []struct {
		b       []byte
		canMore bool
		wantW   int
	}{
		{[]byte("\x1b[<3"), true, 0},
		{[]byte("\x1b[<"), true, 0},
		{[]byte("\x1b[<35;69;"), true, 0},
		{[]byte("\x1b["), true, 0},
		// Short read: the boundary is real, parse as before.
		{[]byte("\x1b[<3"), false, 2},
		// Complete sequences are unaffected.
		{[]byte("\x1b[<35;69;20M"), true, 12},
		{[]byte("\x1b[A"), true, 3},
	} {
		w, _ := detectOneMsg(tc.b, tc.canMore)
		if w != tc.wantW {
			t.Errorf("input %q canMore=%v: w=%d, want %d", tc.b, tc.canMore, w, tc.wantW)
		}
	}
}
