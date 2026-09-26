package ui

import (
	"sort"
	"strings"
)

// memberSet tracks who's in one channel: nick -> status prefix
// ("@" for ops, "+" for voice, "" for regulars).
type memberSet struct {
	nicks map[string]string
}

func memberKey(server, channel string) string { return server + "\x00" + channel }

func (a *App) memberAdd(server, channel, nick, prefix string) {
	if nick == "" {
		return
	}
	if a.members == nil {
		a.members = map[string]*memberSet{}
	}
	k := memberKey(server, channel)
	ms, ok := a.members[k]
	if !ok {
		ms = &memberSet{nicks: map[string]string{}}
		a.members[k] = ms
	}
	ms.nicks[nick] = prefix
}

func (a *App) memberRemove(server, channel, nick string) {
	if ms := a.members[memberKey(server, channel)]; ms != nil {
		delete(ms.nicks, nick)
	}
}

// memberQuit drops nick from every channel on server.
func (a *App) memberQuit(server, nick string) {
	prefix := server + "\x00"
	for k, ms := range a.members {
		if strings.HasPrefix(k, prefix) {
			delete(ms.nicks, nick)
		}
	}
}

// memberRename carries nick's status over to the new nick.
func (a *App) memberRename(server, oldNick, newNick string) {
	if oldNick == "" || newNick == "" {
		return
	}
	prefix := server + "\x00"
	for k, ms := range a.members {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		if p, ok := ms.nicks[oldNick]; ok {
			delete(ms.nicks, oldNick)
			ms.nicks[newNick] = p
		}
	}
}

// modePrefix maps channel status modes to the nick-list prefix they grant.
var modePrefix = map[byte]string{
	'q': "~", // owner
	'a': "&", // admin
	'o': "@", // op
	'h': "%", // half-op
	'v': "+", // voice
}

// applyModePrefixes updates the nick list for channel status-mode changes
// ("+o-v nick …"). Only prefix modes are tracked; everything else is
// display-only. Args are consumed for every parameter-taking mode (including
// untracked ones like +b) so alignment survives mixed mode strings.
func (a *App) applyModePrefixes(server, channel, modes string, args []string) {
	sign := byte('+')
	ai := 0
	for i := 0; i < len(modes); i++ {
		c := modes[i]
		if c == '+' || c == '-' {
			sign = c
			continue
		}
		takesArg := strings.ContainsRune("ovqahbeiI", rune(c)) ||
			(sign == '+' && (c == 'k' || c == 'l'))
		arg := ""
		if takesArg && ai < len(args) {
			arg = args[ai]
			ai++
		}
		prefix, tracked := modePrefix[c]
		if !tracked || arg == "" {
			continue
		}
		ms := a.members[memberKey(server, channel)]
		if ms == nil {
			continue
		}
		cur, ok := ms.nicks[arg]
		if sign == '+' {
			// Grant: only upgrade (never demote @ to +).
			if !ok || prefixRank(prefix) < prefixRank(cur) {
				ms.nicks[arg] = prefix
			}
		} else if ok && cur == prefix {
			ms.nicks[arg] = ""
		}
	}
}

// handleNames processes RPL_NAMREPLY (353): ":srv 353 me = #chan :@a +b c".
func (a *App) handleNames(server, trailing string, params []string) {
	ch := ""
	for _, p := range params {
		if isChannel(p) {
			ch = p
			break
		}
	}
	if ch == "" {
		return
	}
	for _, n := range strings.Fields(trailing) {
		prefix := ""
		name := n
		for len(name) > 0 && strings.ContainsRune("@+%~&", rune(name[0])) {
			if prefix == "" {
				prefix = string(name[0])
			}
			name = name[1:]
		}
		a.memberAdd(server, ch, name, prefix)
	}
	a.renderNicks()
}

type displayNick struct {
	nick   string
	prefix string
}

func prefixRank(p string) int {
	switch p {
	case "~", "&", "@":
		return 0 // owner / admin / op
	case "%":
		return 1 // half-op
	case "+":
		return 2 // voice
	default:
		return 3
	}
}

// sorted lists nicks by status, then alphabetically.
func (s *memberSet) sorted() []displayNick {
	if s == nil {
		return nil
	}
	out := make([]displayNick, 0, len(s.nicks))
	for nick, prefix := range s.nicks {
		out = append(out, displayNick{nick, prefix})
	}
	sort.Slice(out, func(i, j int) bool {
		ri, rj := prefixRank(out[i].prefix), prefixRank(out[j].prefix)
		if ri != rj {
			return ri < rj
		}
		return strings.ToLower(out[i].nick) < strings.ToLower(out[j].nick)
	})
	return out
}
