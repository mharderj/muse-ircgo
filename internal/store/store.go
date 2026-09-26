// Package store keeps per-buffer scrollback for the UI.
package store

import (
	"strings"
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
	KindImage // rendered image preview (half-block art)
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
	Topic  string // channel topic, from RPL_TOPIC (332) / TOPIC
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

// Has reports whether a buffer exists.
func (s *Store) Has(server, name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.buffers[key(server, name)]
	return ok
}

// Resolve returns the stored spelling of a buffer name when a
// case-insensitive match already exists, or name unchanged. IRC nick and
// channel names are case-insensitive ("Belial" and "belial" are the same
// peer), so without this a reply can open a second buffer next to the one
// you messaged. Server windows (name == server) never match: a nick that
// merely resembles a server name must not capture the server window.
func (s *Store) Resolve(server, name string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, b := range s.order {
		if b.Server == server && b.Name != server && strings.EqualFold(b.Name, name) {
			return b.Name
		}
	}
	return name
}

// Remove drops a buffer, e.g. after parting a channel.
func (s *Store) Remove(server, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(server, name)
	if _, ok := s.buffers[k]; !ok {
		return
	}
	delete(s.buffers, k)
	for i, b := range s.order {
		if b.Server == server && b.Name == name {
			s.order = append(s.order[:i], s.order[i+1:]...)
			return
		}
	}
}
