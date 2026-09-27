// Package history logs channel buffers to the log directory (one file per
// channel) and plays back the tail of those logs when a buffer is created.
// The directory defaults to ~/.local/ircgo/logs and can be overridden with
// the log_dir config setting; setting history.LogDir to "" disables
// logging, playback, and query-buffer restore entirely.
package history

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"ircgo/internal/store"
)

// LogDir is the base directory for channel logs. Tests may override it.
var LogDir = defaultLogDir()

func defaultLogDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "ircgo", "logs")
}

// safeName makes a server or channel name safe for use as a path element.
func safeName(s string) string {
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, s)
	if s == "" || s == "." || s == ".." {
		s = "_"
	}
	return s
}

// Path returns the log file for a buffer.
func Path(server, target string) string {
	return filepath.Join(ServerDir(server), safeName(target)+".log")
}

// ServerDir returns the directory holding a server's log files.
func ServerDir(server string) string {
	return filepath.Join(LogDir, safeName(server))
}

// FormatLine renders a scrollback line for the log. It returns false for
// line kinds that are not logged (image preview art is regenerable noise).
func FormatLine(l store.Line) (string, bool) {
	ts := l.At.Format("2006-01-02 15:04:05")
	text := strings.ReplaceAll(strings.ReplaceAll(l.Text, "\r", " "), "\n", " ")
	switch l.Kind {
	case store.KindChat:
		return fmt.Sprintf("[%s] <%s> %s", ts, l.Nick, text), true
	case store.KindAction:
		return fmt.Sprintf("[%s] * %s %s", ts, l.Nick, text), true
	case store.KindNotice:
		return fmt.Sprintf("[%s] -%s- %s", ts, l.Nick, text), true
	default:
		if l.Nick != "" {
			return fmt.Sprintf("[%s] -- %s %s", ts, l.Nick, text), true
		}
		return fmt.Sprintf("[%s] -- %s", ts, text), true
	}
}

// logRe parses a log line: timestamp, a marker (<nick>, * nick, -nick-,
// or --), and the text.
var logRe = regexp.MustCompile(`^\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})\] (<[^>]*>|\* \S+|-[^-]+-|--)(?: (.*))?$`)

// ParseLine parses a log line back into a scrollback line.
func ParseLine(s string) (store.Line, bool) {
	m := logRe.FindStringSubmatch(s)
	if m == nil {
		return store.Line{}, false
	}
	at, err := time.ParseInLocation("2006-01-02 15:04:05", m[1], time.Local)
	if err != nil {
		return store.Line{}, false
	}
	marker, text := m[2], m[3]
	l := store.Line{At: at, Text: text}
	switch {
	case marker == "--":
		l.Kind = store.KindSystem
	case strings.HasPrefix(marker, "<"):
		l.Kind, l.Nick = store.KindChat, marker[1:len(marker)-1]
	case strings.HasPrefix(marker, "* "):
		l.Kind, l.Nick = store.KindAction, marker[2:]
	default: // -nick-
		l.Kind, l.Nick = store.KindNotice, marker[1:len(marker)-1]
	}
	return l, true
}

// dedupeTailBytes bounds the duplicate check to the recent tail of the log.
const dedupeTailBytes = 32 << 10

// AppendLine appends a pre-formatted line to the buffer's log, skipping it
// when the identical line is already in the recent tail (bouncer replays
// re-deliver messages that were logged live in an earlier session).
func AppendLine(server, target, line string) {
	if LogDir == "" {
		return
	}
	p := Path(server, target)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	if tailContains(p, line) {
		return
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	fmt.Fprintln(f, line)
	f.Close()
}

// tailContains reports whether the exact line appears in the tail of the file.
func tailContains(path, line string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return false
	}
	off := st.Size() - dedupeTailBytes
	if off < 0 {
		off = 0
	}
	buf := make([]byte, st.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil {
		return false
	}
	// Only compare whole lines, and only when we read from the file start
	// or the tail begins on a line boundary.
	s := string(buf)
	if off > 0 {
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
		} else {
			s = ""
		}
	}
	for _, l := range strings.Split(s, "\n") {
		if l == line {
			return true
		}
	}
	return false
}

// LastLines returns up to n of the most recent log lines for a buffer.
func LastLines(server, target string, n int) []string {
	if LogDir == "" || n <= 0 {
		return nil
	}
	data, err := os.ReadFile(Path(server, target))
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	var out []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}
