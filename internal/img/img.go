// Package img fetches image URLs posted in chat and renders them as
// half-block terminal art (like chafa): each "▀" cell carries two image
// rows, the top pixel as foreground and the bottom as background. It works
// in any terminal and keeps every art row a real text row, so viewport
// scrolling math stays correct.
package img

import (
	"context"
	"fmt"
	"image"
	_ "image/gif" // first frame of animated GIFs
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	_ "golang.org/x/image/webp"
)

const (
	maxFetchBytes = 8 << 20 // 8 MiB per image
	fetchTimeout  = 15 * time.Second
	maxURLsPerMsg = 2
)

var urlRe = regexp.MustCompile(`https?://[^\s<>"']+`)

var imgExts = []string{".png", ".jpg", ".jpeg", ".gif", ".webp"}

// FindURLs returns up to maxURLsPerMsg image URLs found in text.
func FindURLs(text string) []string {
	var out []string
	for _, raw := range urlRe.FindAllString(text, -1) {
		u := strings.TrimRight(raw, ".,;:!?)")
		if !isImageURL(u) {
			continue
		}
		out = append(out, u)
		if len(out) >= maxURLsPerMsg {
			break
		}
	}
	return out
}

func isImageURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	p := strings.ToLower(u.Path)
	for _, ext := range imgExts {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}

// Fetch downloads and decodes an image, capped by size and time.
func Fetch(ctx context.Context, rawURL string) (image.Image, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ircgo/image-preview")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("img: %s -> %s", rawURL, resp.Status)
	}
	m, _, err := image.Decode(io.LimitReader(resp.Body, maxFetchBytes))
	if err != nil {
		return nil, fmt.Errorf("img: decode %s: %w", rawURL, err)
	}
	return m, nil
}

// Render converts m to half-block art fitting within maxWidth cells and
// maxRows text rows, preserving aspect ratio. Raw 24-bit escapes are
// emitted so the output doesn't depend on terminal color detection.
func Render(m image.Image, maxWidth, maxRows int) string {
	if maxWidth < 1 {
		maxWidth = 1
	}
	if maxRows < 1 {
		maxRows = 1
	}
	b := m.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return ""
	}

	// Fit into maxWidth columns and maxRows*2 pixel rows (two pixels per
	// text row), keeping square-ish pixels.
	scale := min(float64(maxWidth)/float64(sw), float64(maxRows*2)/float64(sh))
	tw := max(int(float64(sw)*scale+0.5), 1)
	th := max(int(float64(sh)*scale+0.5), 1)
	th = th / 2 * 2 // pairs of rows
	if th < 2 {
		th = 2
	}

	px := func(x, y int) (r, g, bl uint8) {
		// Box-average the source pixels covered by target pixel (x, y).
		x0 := float64(x) * float64(sw) / float64(tw)
		x1 := float64(x+1) * float64(sw) / float64(tw)
		y0 := float64(y) * float64(sh) / float64(th)
		y1 := float64(y+1) * float64(sh) / float64(th)
		var sr, sg, sb, n float64
		for sy := int(y0); float64(sy) < y1 && sy < sh; sy++ {
			for sx := int(x0); float64(sx) < x1 && sx < sw; sx++ {
				cr, cg, cb, ca := m.At(b.Min.X+sx, b.Min.Y+sy).RGBA()
				a := float64(ca) / 0xffff
				// Composite onto black.
				sr += float64(cr) / 0xffff * a
				sg += float64(cg) / 0xffff * a
				sb += float64(cb) / 0xffff * a
				n++
			}
		}
		if n == 0 {
			return 0, 0, 0
		}
		return uint8(sr / n * 255), uint8(sg / n * 255), uint8(sb / n * 255)
	}

	var sb strings.Builder
	for y := 0; y < th; y += 2 {
		for x := 0; x < tw; x++ {
			tr, tg, tb := px(x, y)
			br, bg, bb := px(x, y+1)
			fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀",
				tr, tg, tb, br, bg, bb)
		}
		sb.WriteString("\x1b[0m\n")
	}
	return strings.TrimSuffix(sb.String(), "\n")
}
