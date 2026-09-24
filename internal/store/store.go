// Package store keeps per-buffer scrollback for the UI.
package store

import (
	"sync"
	"time"
)

// Kind classifies a scrollback line.
type Kind int

const (
	KindChat Kind = iota
	KindJoin
	KindPart
	KindQuit
	KindNotice
	KindAction
	KindSystem
)

// Line is one rendered row in a buffer.
type Line struct {
	At   time.Time
	Nick string
	Text string
	Kind Kind
}

// maxLines caps scrollback per buffer.
const maxLines = 5000

// Buffer is the scrollback for one channel, query, or server window.
type Buffer struct {
	Server string
	Name   string
	lines  []Line
}

// Append adds a line, trimming old scrollback past the cap.
func (b *Buffer) Append(l Line) {
	b.lines = append(b.lines, l)
	if len(b.lines) > maxLines {
		b.lines = b.lines[len(b.lines)-maxLines:]
	}
}

// Lines returns a copy of the buffer's lines.
func (b *Buffer) Lines() []Line {
	out := make([]Line, len(b.lines))
	copy(out, b.lines)
	return out
}

// Store owns every buffer, keyed by server and name.
type Store struct {
	mu      sync.Mutex
	buffers map[string]*Buffer
	order   []*Buffer
}

// New returns an empty store.
func New() *Store {
	return &Store{buffers: make(map[string]*Buffer)}
}

func key(server, name string) string { return server + "\x00" + name }

// Get returns the buffer, creating it on first use.
func (s *Store) Get(server, name string) *Buffer {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(server, name)
	if b, ok := s.buffers[k]; ok {
		return b
	}
	b := &Buffer{Server: server, Name: name}
	s.buffers[k] = b
	s.order = append(s.order, b)
	return b
}

// Buffers returns all buffers in creation order.
func (s *Store) Buffers() []*Buffer {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Buffer, len(s.order))
	copy(out, s.order)
	return out
}
