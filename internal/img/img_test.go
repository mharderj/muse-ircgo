package img

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestFindURLs(t *testing.T) {
	cases := []struct {
		text string
		want []string
	}{
		{"no links here", nil},
		{"see https://example.com/page.html", nil},
		{
			"pic https://cdn.discordapp.com/attachments/1/2/photo.png",
			[]string{"https://cdn.discordapp.com/attachments/1/2/photo.png"},
		},
		{
			"upper https://x.test/A.JPG?size=large",
			[]string{"https://x.test/A.JPG?size=large"},
		},
		{
			"trailing https://x.test/a.webp.",
			[]string{"https://x.test/a.webp"},
		},
		{
			"two https://x.test/a.png and https://x.test/b.gif ok",
			[]string{"https://x.test/a.png", "https://x.test/b.gif"},
		},
		{
			// capped at maxURLsPerMsg
			"https://x.test/1.png https://x.test/2.png https://x.test/3.png",
			[]string{"https://x.test/1.png", "https://x.test/2.png"},
		},
	}
	for _, tc := range cases {
		if got := FindURLs(tc.text); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("FindURLs(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}

// 2x2 image, 1:1 scale: each cell's fg is the top pixel, bg the bottom.
func TestRender(t *testing.T) {
	m := image.NewRGBA(image.Rect(0, 0, 2, 2))
	m.Set(0, 0, color.RGBA{255, 0, 0, 255})     // red
	m.Set(1, 0, color.RGBA{0, 255, 0, 255})     // green
	m.Set(0, 1, color.RGBA{0, 0, 255, 255})     // blue
	m.Set(1, 1, color.RGBA{255, 255, 255, 255}) // white
	got := Render(m, 2, 1)
	want := "\x1b[38;2;255;0;0m\x1b[48;2;0;0;255m▀" +
		"\x1b[38;2;0;255;0m\x1b[48;2;255;255;255m▀\x1b[0m"
	if got != want {
		t.Fatalf("Render = %q, want %q", got, want)
	}
}

func TestRenderFitsBounds(t *testing.T) {
	m := image.NewRGBA(image.Rect(0, 0, 200, 100))
	got := Render(m, 40, 10)
	rows := 0
	for _, row := range splitLines(got) {
		rows++
		if cellCount(row) > 40 {
			t.Fatalf("row wider than 40 cells: %q", row)
		}
	}
	if rows > 10 {
		t.Fatalf("taller than 10 rows: %d", rows)
	}
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

// cellCount counts printable cells, skipping ANSI escapes.
func cellCount(s string) int {
	n := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++
			continue
		}
		// "▀" is 3 bytes in UTF-8; count runes.
		_, w := decodeRune(s[i:])
		n++
		i += w
	}
	return n
}

func decodeRune(s string) (rune, int) {
	// minimal UTF-8 width decode, enough for the test
	c := s[0]
	if c < 0x80 {
		return rune(c), 1
	}
	if c>>5 == 0x6 {
		return rune(c&0x1f)<<6 | rune(s[1]&0x3f), 2
	}
	return rune(c&0x0f)<<12 | rune(s[1]&0x3f)<<6 | rune(s[2]&0x3f), 3
}

func TestFetch(t *testing.T) {
	m := image.NewRGBA(image.Rect(0, 0, 2, 2))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing.png" {
			http.NotFound(w, r)
			return
		}
		_ = png.Encode(w, m)
	}))
	defer srv.Close()

	got, err := Fetch(context.Background(), srv.URL+"/tiny.png")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got.Bounds().Dx() != 2 || got.Bounds().Dy() != 2 {
		t.Fatalf("bounds = %v, want 2x2", got.Bounds())
	}
	if _, err := Fetch(context.Background(), srv.URL+"/missing.png"); err == nil {
		t.Fatal("Fetch of 404, want error")
	}
}
