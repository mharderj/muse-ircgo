package emoji

import "testing"

func TestReplace(t *testing.T) {
	cases := []struct{ in, want string }{
		{"sounds like a no :confused:", "sounds like a no 😕"},
		{":joy: :+1: :-1:", "😂 👍 👎"},
		{"no :nosuchcode: here", "no :nosuchcode: here"},
		{"see https://x.test/:smile:/a :joy:", "see https://x.test/:smile:/a 😂"},
		{"12:02 <bob> hi", "12:02 <bob> hi"},
		{":SMILE: loud", "😄 loud"},
		{"a:b:c", "a:b:c"},
		{"x :a: y", "x :a: y"},
		{"score was 3:2 at half", "score was 3:2 at half"},
		{"", ""},
	}
	for _, c := range cases {
		if got := Replace(c.in); got != c.want {
			t.Errorf("Replace(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

func TestAtLongestMatch(t *testing.T) {
	// heart_on_fire (❤️‍🔥) starts with the same runes as heart (❤️);
	// the longest match must win.
	r := []rune("x ❤️\u200d🔥 y")
	e, sc, ok := At(r, 2)
	if !ok || sc != "heart_on_fire" {
		t.Fatalf("At = %q, %q, %v; want heart_on_fire", e, sc, ok)
	}
	// Plain heart still resolves on its own.
	r = []rune("x ❤️ y")
	if _, sc, ok := At(r, 2); !ok || sc != "heart" {
		t.Fatalf("At(heart) = %q, %v; want heart", sc, ok)
	}
	// Plain text is a miss, and so is an out-of-range index.
	r = []rune("abc")
	if _, _, ok := At(r, 1); ok {
		t.Fatal("At matched plain text")
	}
	if _, _, ok := At(r, 99); ok {
		t.Fatal("At matched out of range")
	}
}

func TestLookup(t *testing.T) {
	if sc, ok := Lookup("😕"); !ok || sc != "confused" {
		t.Fatalf("Lookup(😕) = %q, %v; want confused", sc, ok)
	}
	if _, ok := Lookup("🦄"); ok {
		t.Fatal("Lookup matched an unmapped emoji")
	}
}
