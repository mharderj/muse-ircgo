package link

import "testing"

func TestFindSpans(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"none", "just chatting", nil},
		{"basic", "see https://example.com/a.png ok", []string{"https://example.com/a.png"}},
		{"trailing punct", "pic: https://x.test/a.png.", []string{"https://x.test/a.png"}},
		{"parens", "(https://x.test/a.png)", []string{"https://x.test/a.png"}},
		{"multiple", "a https://one.test/ b http://two.test/x", []string{"https://one.test/", "http://two.test/x"}},
		{"query", "https://cdn.discord.com/a.png?ex=1&is=2", []string{"https://cdn.discord.com/a.png?ex=1&is=2"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FindAll(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("FindAll(%q) = %v, want %v", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("FindAll(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestStyle(t *testing.T) {
	got := Style("see https://x.test/a.png.", func(s string) string { return "<" + s + ">" })
	want := "see <https://x.test/a.png>."
	if got != want {
		t.Fatalf("Style = %q, want %q", got, want)
	}
	if got := Style("no links here", func(s string) string { return "<" + s + ">" }); got != "no links here" {
		t.Fatalf("Style without links = %q", got)
	}
}
