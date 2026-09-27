package ui

import (
	"strings"
	"testing"

	"ircgo/internal/config"
	"ircgo/internal/irc"
	"ircgo/internal/store"
)

func kickTestApp() *App {
	a := testApp()
	a.cfg = &config.Config{Servers: []config.Server{{Name: "srv", Nick: "me"}}}
	return a
}

func lastLineText(a *App, server, buf string) string {
	lines := a.st.Get(server, buf).Lines()
	if len(lines) == 0 {
		return ""
	}
	return lines[len(lines)-1].Text
}

func TestKickLogsAndRemovesMember(t *testing.T) {
	a := kickTestApp()
	a.memberAdd("srv", "#c", "bob", "@")
	a.memberAdd("srv", "#c", "alice", "")

	a.handleMessage("srv", &irc.Message{
		Prefix: "alice!u@h", Command: "KICK", Params: []string{"#c", "bob", "spamming"},
	})

	if got := lastLineText(a, "srv", "#c"); !strings.Contains(got, "bob was kicked by alice") || !strings.Contains(got, "spamming") {
		t.Fatalf("kick line = %q", got)
	}
	if ms := a.members[memberKey("srv", "#c")]; ms.nicks["bob"] != "" {
		t.Fatalf("bob still in roster: %v", ms.nicks)
	}
	if _, ok := a.members[memberKey("srv", "#c")].nicks["alice"]; !ok {
		t.Fatal("alice dropped from roster")
	}
	// Malformed KICK is ignored.
	a.handleMessage("srv", &irc.Message{Command: "KICK", Params: []string{"#c"}})
}

func TestModePrefixGrantRevoke(t *testing.T) {
	a := kickTestApp()
	a.memberAdd("srv", "#c", "bob", "")
	a.memberAdd("srv", "#c", "amy", "+")

	// Grant op.
	a.handleMessage("srv", &irc.Message{
		Prefix: "alice!u@h", Command: "MODE", Params: []string{"#c", "+o", "bob"},
	})
	if p := a.members[memberKey("srv", "#c")].nicks["bob"]; p != "@" {
		t.Fatalf("bob prefix = %q, want @", p)
	}
	// Granting voice to an op must not demote.
	a.handleMessage("srv", &irc.Message{
		Prefix: "alice!u@h", Command: "MODE", Params: []string{"#c", "+v", "bob"},
	})
	if p := a.members[memberKey("srv", "#c")].nicks["bob"]; p != "@" {
		t.Fatalf("bob prefix = %q, want @ (no demote)", p)
	}
	// Revoke op.
	a.handleMessage("srv", &irc.Message{
		Prefix: "alice!u@h", Command: "MODE", Params: []string{"#c", "-o", "bob"},
	})
	if p := a.members[memberKey("srv", "#c")].nicks["bob"]; p != "" {
		t.Fatalf("bob prefix = %q, want empty", p)
	}
	// Revoking voice amy doesn't have as op must not clear her +.
	a.handleMessage("srv", &irc.Message{
		Prefix: "alice!u@h", Command: "MODE", Params: []string{"#c", "-o", "amy"},
	})
	if p := a.members[memberKey("srv", "#c")].nicks["amy"]; p != "+" {
		t.Fatalf("amy prefix = %q, want +", p)
	}
	// Mode change is visible in the channel.
	if got := lastLineText(a, "srv", "#c"); !strings.Contains(got, "set mode -o amy") {
		t.Fatalf("mode line = %q", got)
	}
}

func TestModeArgAlignmentWithUntrackedModes(t *testing.T) {
	a := kickTestApp()
	a.memberAdd("srv", "#c", "bob", "")
	a.memberAdd("srv", "#c", "amy", "")

	// +o consumes bob, +b consumes the mask (untracked), -v consumes amy.
	a.handleMessage("srv", &irc.Message{
		Prefix: "alice!u@h", Command: "MODE",
		Params: []string{"#c", "+o+b-v", "bob", "*!*@bad.host", "amy"},
	})
	ms := a.members[memberKey("srv", "#c")]
	if ms.nicks["bob"] != "@" {
		t.Fatalf("bob prefix = %q, want @", ms.nicks["bob"])
	}
	if _, ok := ms.nicks["*!*@bad.host"]; ok {
		t.Fatalf("ban mask leaked into roster: %v", ms.nicks)
	}
	if ms.nicks["amy"] != "" {
		t.Fatalf("amy prefix = %q, want empty", ms.nicks["amy"])
	}
}

func TestModeNonPrefixOnlyLogs(t *testing.T) {
	a := kickTestApp()
	a.memberAdd("srv", "#c", "bob", "")
	a.handleMessage("srv", &irc.Message{
		Prefix: "alice!u@h", Command: "MODE", Params: []string{"#c", "+m"},
	})
	if got := lastLineText(a, "srv", "#c"); !strings.Contains(got, "set mode +m") {
		t.Fatalf("mode line = %q", got)
	}
	if p := a.members[memberKey("srv", "#c")].nicks["bob"]; p != "" {
		t.Fatalf("bob prefix changed to %q", p)
	}
}

func TestInviteSurfacesInServerWindow(t *testing.T) {
	a := kickTestApp()
	a.handleMessage("srv", &irc.Message{
		Prefix: "alice!u@h", Command: "INVITE", Params: []string{"me", "#secret"},
	})
	if got := lastLineText(a, "srv", "srv"); !strings.Contains(got, "invited you to #secret") {
		t.Fatalf("invite line = %q", got)
	}
}

func TestOwnNickTrackedFrom001AndNick(t *testing.T) {
	a := kickTestApp()
	// Fresh connection: 001 carries the assigned nick (post-433 "me_").
	a.handleMessage("srv", &irc.Message{Command: "001", Params: []string{"me_", "Welcome"}})
	if got := a.ownNick("srv"); got != "me_" {
		t.Fatalf("ownNick = %q, want me_", got)
	}
	// Someone else changing nick must not touch ours.
	a.handleMessage("srv", &irc.Message{
		Prefix: "alice!u@h", Command: "NICK", Params: []string{"alicia"},
	})
	if got := a.ownNick("srv"); got != "me_" {
		t.Fatalf("ownNick = %q, want me_", got)
	}
	// Our own NICK echo updates it.
	a.handleMessage("srv", &irc.Message{
		Prefix: "me_!u@h", Command: "NICK", Params: []string{"me"},
	})
	if got := a.ownNick("srv"); got != "me" {
		t.Fatalf("ownNick = %q, want me", got)
	}
}

func TestMeCommandSendsAction(t *testing.T) {
	a := kickTestApp()
	a.connUp["srv"] = true // simulate a seen connect; the client has no conn
	a.st.Get("srv", "#c")
	a.bufs = a.st.Buffers()
	cl := irc.New(config.Server{Name: "srv", Nick: "me"}, nil) // no conn: Send error ignored

	a.sendCommand(cl, a.bufs[0], "/me waves hello")

	lines := a.st.Get("srv", "#c").Lines()
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	l := lines[0]
	if l.Kind != store.KindAction || l.Nick != "me" || l.Text != "waves hello" {
		t.Fatalf("line = %+v", l)
	}
}

func TestNickCommandNoArgIsNoop(t *testing.T) {
	a := kickTestApp()
	a.st.Get("srv", "#c")
	a.bufs = a.st.Buffers()
	cl := irc.New(config.Server{Name: "srv", Nick: "me"}, nil)
	// Must not crash; the server echo (or 433) updates state later.
	a.sendCommand(cl, a.bufs[0], "/nick")
	a.sendCommand(cl, a.bufs[0], "/nick newme")
}
