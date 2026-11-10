package render

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/fogleman/gg"
	"golang.org/x/image/font"

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
	labelSize = 16
)

// ctx is what a widget gets to draw with.
type ctx struct {
	dc     *gg.Context
	states *ha.Store
	now    time.Time
	opts   map[string]any
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
// (ax, ay) in 0..1 within r. Vertical placement uses the cap height, so
// digits and capitals are centred optically; descenders may hang below.
func (c *ctx) text(s string, f fontID, maxPx float64, r rect, ax, ay float64) {
	if s == "" || r.W <= 0 || r.H <= 0 {
		return
	}
	px := fitSize(c.dc, s, f, maxPx, r.W, r.H)
	fc := face(f, px)
	c.dc.SetFontFace(fc)
	c.dc.SetRGB(0, 0, 0)
	tw, _ := c.dc.MeasureString(s)
	capH := capHeight(fc)
	x := r.X + ax*(r.W-tw)
	baseline := r.Y + ay*(r.H-capH) + capH
	c.dc.DrawString(s, x, baseline)
}

func capHeight(f font.Face) float64 {
	m := f.Metrics()
	if m.CapHeight > 0 {
		return float64(m.CapHeight) / 64
	}
	return float64(m.Ascent) / 64 * 0.72
}

// fitSize picks the largest whole pixel size <= maxPx at which s fits both
// the width and (by cap height plus descender room) the height.
func fitSize(dc *gg.Context, s string, f fontID, maxPx, w, h float64) float64 {
	px := math.Floor(maxPx)
	for ; px > 10; px -= 2 {
		fc := face(f, px)
		if capHeight(fc)*1.25 > h {
			continue
		}
		dc.SetFontFace(fc)
		tw, _ := dc.MeasureString(s)
		if tw <= w {
			break
		}
	}
	return math.Max(px, 10)
}

// label draws a small caption at the top of r and returns the space below.
func (c *ctx) label(s string, r rect) rect {
	if s == "" {
		return r
	}
	top, rest := r.splitTop(labelSize + 6)
	c.text(s, fontMedium, labelSize, top, 0, 0.5)
	return rest
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
