package render

import (
	"image"
	"image/draw"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/fogleman/gg"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"

	"homepaper/internal/ha"
)

type rect struct{ X, Y, W, H float64 }

func (r rect) inset(d float64) rect { return rect{r.X + d, r.Y + d, r.W - 2*d, r.H - 2*d} }

// splitTop returns the top part of height h and the remainder.
func (r rect) splitTop(h float64) (rect, rect) {
	h = math.Min(h, r.H)
	return rect{r.X, r.Y, r.W, h}, rect{r.X, r.Y + h, r.W, r.H - h}
}

func (r rect) splitLeft(w float64) (rect, rect) {
	w = math.Min(w, r.W)
	return rect{r.X, r.Y, w, r.H}, rect{r.X + w, r.Y, r.W - w, r.H}
}

const (
	framePad  = 10 // space between widget frame and content
	frameGap  = 4  // space between neighbouring widget frames
	labelSize      = 17 // bitmap sizes
	labelSmallSize = 14
)

// ctx is what a widget gets to draw with.
type ctx struct {
	dc     *gg.Context
	states *ha.Store
	now    time.Time
	opts   map[string]any
	clip   image.Rectangle // the widget's area; text is clipped to it
}

func (c *ctx) str(key, def string) string {
	if v, ok := c.opts[key].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func (c *ctx) num(key string, def float64) float64 {
	switch v := c.opts[key].(type) {
	case float64:
		return v
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func (c *ctx) boolOpt(key string, def bool) bool {
	if v, ok := c.opts[key].(bool); ok {
		return v
	}
	return def
}

// entity looks up the entity bound to an option key.
func (c *ctx) entity(key string) (ha.Entity, bool) {
	id := c.str(key, "")
	if id == "" {
		return ha.Entity{}, false
	}
	e, ok := c.states.Get(id)
	if !ok || e.State == "unavailable" || e.State == "unknown" {
		return e, false
	}
	return e, true
}

func drawFrame(dc *gg.Context, r rect) {
	r = r.inset(frameGap / 2)
	dc.SetRGB(0, 0, 0)
	dc.SetLineWidth(2)
	dc.DrawRoundedRectangle(r.X+1, r.Y+1, r.W-2, r.H-2, 8)
	dc.Stroke()
}

// text draws s at the largest size up to maxPx that fits r, aligned by
// (ax, ay) in 0..1 within r. Sizes come from a fixed scale (sizeSteps), so
// small text lands on a bitmap font. Vertical placement uses the cap
// height, so digits and capitals are centred optically; descenders may hang
// below. Text that doesn't fit even at the smallest size is shortened.
func (c *ctx) text(s string, f fontID, maxPx float64, r rect, ax, ay float64) {
	if s == "" || r.W <= 0 || r.H <= 0 {
		return
	}
	px := fitSize(s, f, maxPx, r.W, r.H)
	c.textAt(s, f, px, r, ax, ay)
}

// textAt draws s at exactly px, shortened with "…" to fit r's width.
func (c *ctx) textAt(s string, f fontID, px int, r rect, ax, ay float64) {
	fc := face(f, px)
	s = ellipsize(fc, s, r.W)
	tw := measure(fc, s)
	capH := capHeight(fc)
	// Whole pixels: a fractional origin shifts every glyph off the grid and
	// the 1-bit threshold then eats stems unevenly.
	x := math.Round(r.X + ax*(r.W-tw))
	baseline := math.Round(r.Y + ay*(r.H-capH) + capH)
	d := &font.Drawer{
		Dst:  c.canvas(),
		Src:  image.Black,
		Face: fc,
		Dot:  fixed.P(int(x), int(baseline)),
	}
	d.DrawString(s)
}

// canvas is the frame, clipped to the widget being drawn, so text can
// never spill into a neighbour.
func (c *ctx) canvas() draw.Image {
	im := c.dc.Image().(*image.RGBA)
	return im.SubImage(c.clip).(*image.RGBA)
}

func measure(f font.Face, s string) float64 {
	return float64(font.MeasureString(f, s)) / 64
}

func ellipsize(f font.Face, s string, w float64) string {
	if measure(f, s) <= w {
		return s
	}
	runes := []rune(s)
	for n := len(runes) - 1; n > 0; n-- {
		// Three periods: Lucida's bitmap "…" has only two dots.
		t := strings.TrimRight(string(runes[:n]), " ·,.") + "..."
		if measure(f, t) <= w {
			return t
		}
	}
	return ""
}

func capHeight(f font.Face) float64 {
	m := f.Metrics()
	if m.CapHeight > 0 {
		return float64(m.CapHeight) / 64
	}
	return float64(m.Ascent) / 64 * 0.72
}

// fitSize picks the largest size step <= maxPx at which s fits both the
// width and (by cap height plus descender room) the height. If none fits,
// it returns the smallest step and the caller shortens the text.
func fitSize(s string, f fontID, maxPx, w, h float64) int {
	steps := sizeSteps(maxPx)
	for i := len(steps) - 1; i >= 0; i-- {
		fc := face(f, steps[i])
		if capHeight(fc)*1.25 <= h && measure(fc, s) <= w {
			return steps[i]
		}
	}
	// Nothing fits: the smallest size that fits the height, shortened.
	for _, px := range steps {
		if capHeight(face(f, px))*1.25 > h {
			break
		}
		return px
	}
	return steps[0]
}

// label draws a caption at the top of r and returns the space below.
// Labels share one size on every widget; a long entity name may step down
// to the next bitmap size, and is shortened after that, never shrunk into
// illegibility.
func (c *ctx) label(s string, r rect) rect {
	if s == "" {
		return r
	}
	top, rest := r.splitTop(labelSize + 7)
	px := labelSize
	if measure(face(fontMedium, px), s) > top.W {
		px = labelSmallSize
	}
	c.textAt(s, fontMedium, px, top, 0, 0.5)
	return rest
}

// fits reports whether s fits width w at the size text() would pick.
func fits(s string, f fontID, maxPx, w, h float64) bool {
	return measure(face(f, fitSize(s, f, maxPx, w, h)), s) <= w
}

// formatNumber formats a numeric state with the given decimals; non-numeric
// states pass through unchanged.
func formatNumber(state string, decimals float64) string {
	f, err := strconv.ParseFloat(state, 64)
	if err != nil {
		return state
	}
	if decimals < 0 {
		return state
	}
	return strconv.FormatFloat(f, 'f', int(decimals), 64)
}

const unavailable = "–"
