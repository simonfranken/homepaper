package render

import (
	"image"
	"image/color"
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

// Drawing primitives for the panel design (docs/design.md, D-015). They
// work in whole pixels: text sits on whole-pixel baselines, so bitmap glyphs
// keep their hand-placed pixels and the 1-bit threshold eats nothing.

// R is a pixel rectangle.
type R struct{ X, Y, W, H int }

func (r R) right() int  { return r.X + r.W }
func (r R) bottom() int { return r.Y + r.H }

// content is the area a widget draws into, inside its cell.
func content(r R) R { return R{r.X + 12, r.Y + 11, r.W - 24, r.H - 22} }

// ctx is what a widget gets to draw with.
type ctx struct {
	dc     *gg.Context
	states *ha.Store
	now    time.Time
	opts   map[string]any
	clip   image.Rectangle // the widget's area; text and fills are clipped to it
	ink    color.Gray      // black, or white on an inverted widget
	ax     float64         // horizontal alignment: 0 left, 0.5 centre, 1 right
}

// alignX is where content w wide starts in r under the widget's alignment.
func (c *ctx) alignX(r R, w int) int {
	return r.X + int(math.Round(c.ax*float64(r.W-w)))
}

// alignment reads the "align" option.
func alignment(opts map[string]any) float64 {
	switch opts["align"] {
	case "center":
		return 0.5
	case "right":
		return 1
	}
	return 0
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
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return f
		}
	}
	return def
}

// entity looks up the entity bound to an option key. ok is false when it is
// missing, unavailable or unknown; e still carries what is known (the name).
func (c *ctx) entity(key string) (ha.Entity, bool) {
	id := c.str(key, "")
	if id == "" {
		return ha.Entity{}, false
	}
	e, ok := c.states.Get(id)
	if !ok || e.State == "unavailable" || e.State == "unknown" {
		if !ok {
			e.ID = id
		}
		return e, false
	}
	return e, true
}

// entityList returns the comma-separated entity IDs of an option.
func (c *ctx) entityList(key string) []string {
	var ids []string
	for _, id := range strings.Split(c.str(key, ""), ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// canvas is the frame, clipped to the widget being drawn, so nothing can
// spill into a neighbour.
func (c *ctx) canvas() draw.Image {
	return c.dc.Image().(*image.RGBA).SubImage(c.clip).(*image.RGBA)
}

func (c *ctx) setInk() {
	v := float64(c.ink.Y) / 255
	c.dc.SetRGB(v, v, v)
}

// ---------- text ----------

func capH(f font.Face) int {
	m := f.Metrics()
	if m.CapHeight > 0 {
		return int(math.Round(float64(m.CapHeight) / 64))
	}
	return int(math.Round(float64(m.Ascent) / 64 * 0.72))
}

func width(f font.Face, s string) int { return int(math.Ceil(float64(font.MeasureString(f, s)) / 64)) }

// T draws s with its baseline at y; ax = 0 left, 0.5 centre, 1 right of x.
// It returns the width drawn.
func (c *ctx) T(s string, id fontID, px, x, y int, ax float64) int {
	f := face(id, px)
	w := width(f, s)
	x0 := x - int(math.Round(ax*float64(w)))
	d := &font.Drawer{Dst: c.canvas(), Src: image.NewUniform(c.ink), Face: f, Dot: fixed.P(x0, y)}
	d.DrawString(s)
	return w
}

// fit returns the first size from sizes (largest first) at which s fits
// maxW wide with a cap height of at most maxCap, else the last size.
func fit(s string, id fontID, sizes []int, maxW, maxCap int) int {
	for _, px := range sizes {
		f := face(id, px)
		if width(f, s) <= maxW && capH(f) <= maxCap {
			return px
		}
	}
	return sizes[len(sizes)-1]
}

// bigSizes is the scale for primary values: Go Bold, then bitmap sizes.
var bigSizes = []int{96, 88, 80, 72, 64, 56, 52, 48, 44, 40, 34, 25, 20, 17, 14, 12}

// ellip shortens s with "..." to fit w. Three periods: Lucida's bitmap "…"
// has only two dots.
func ellip(s string, id fontID, px, w int) string {
	f := face(id, px)
	if width(f, s) <= w {
		return s
	}
	r := []rune(s)
	for n := len(r) - 1; n > 0; n-- {
		t := strings.TrimRight(string(r[:n]), " ·,.") + "..."
		if width(f, t) <= w {
			return t
		}
	}
	return ""
}

// label draws a widget caption at the top of r and returns what is left.
// Labels are 17 px, stepping down to 14 px before they are shortened.
func (c *ctx) label(r R, s string) R {
	px := 17
	if width(face(fontMedium, px), s) > r.W {
		px = 14
	}
	f := face(fontMedium, px)
	s = ellip(s, fontMedium, px, r.W)
	c.T(s, fontMedium, px, c.alignX(r, width(f, s)), r.Y+capH(f), 0)
	used := capH(f) + 9
	return R{r.X, r.Y + used, r.W, r.H - used}
}

// smallLabel is the 14 px caption of one-cell widgets.
func (c *ctx) smallLabel(r R, s string) R {
	f := face(fontMedium, 14)
	s = ellip(s, fontMedium, 14, r.W)
	c.T(s, fontMedium, 14, c.alignX(r, width(f, s)), r.Y+capH(f), 0)
	used := capH(f) + 6
	return R{r.X, r.Y + used, r.W, r.H - used}
}

// smallFor is the size of the raised part next to a big number.
func smallFor(px int) int {
	switch {
	case px >= 72:
		return 34
	case px >= 52:
		return 25
	case px >= 40:
		return 20
	case px >= 34:
		return 17
	default:
		return 14
	}
}

// bigNumber sets whole large and frac (".4°") small and top-aligned, as in
// print, at the largest size up to maxPx that fits r.
func (c *ctx) bigNumber(r R, whole, frac string, maxPx int, ax float64) {
	var sizes []int
	for _, s := range bigSizes {
		if s <= maxPx {
			sizes = append(sizes, s)
		}
	}
	px := sizes[len(sizes)-1]
	sp := 0
	for _, s := range sizes {
		fs := smallFor(s)
		w := width(face(fontBold, s), whole) + width(face(fontBold, fs), frac)
		if w <= r.W && capH(face(fontBold, s)) <= r.H {
			px, sp = s, fs
			break
		}
	}
	if sp == 0 {
		sp = smallFor(px)
	}
	fb, fsm := face(fontBold, px), face(fontBold, sp)
	tw := width(fb, whole) + width(fsm, frac)
	ch := capH(fb)
	base := r.Y + (r.H+ch)/2
	x := r.X + int(math.Round(ax*float64(r.W-tw)))
	x += c.T(whole, fontBold, px, x, base, 0)
	c.T(frac, fontBold, sp, x+1, base-ch+capH(fsm), 0)
}

// splitNum splits "21.4°" into "21" and ".4°".
func splitNum(s string) (string, string) {
	if i := strings.IndexByte(s, '.'); i >= 0 {
		return s[:i], s[i:]
	}
	if strings.HasSuffix(s, "°") {
		return strings.TrimSuffix(s, "°"), "°"
	}
	return s, ""
}

// ---------- shapes ----------

func (c *ctx) fill(r R) {
	draw.Draw(c.canvas(), image.Rect(r.X, r.Y, r.right(), r.bottom()), image.NewUniform(c.ink), image.Point{}, draw.Src)
}

func (c *ctx) hline(x0, x1, y, t int) { c.fill(R{x0, y, x1 - x0, t}) }
func (c *ctx) vline(x, y0, y1, t int) { c.fill(R{x, y0, t, y1 - y0}) }

// dotted is a 1 px line, one pixel on, two off: it groups rows without
// boxing them.
func (c *ctx) dotted(x0, x1, y int) {
	img := c.canvas()
	for x := x0; x < x1; x += 3 {
		img.Set(x, y, c.ink)
	}
}

// bar is a 2 px outlined meter, filled to frac.
func (c *ctx) bar(r R, frac float64) {
	frac = math.Max(0, math.Min(1, frac))
	c.hline(r.X, r.right(), r.Y, 2)
	c.hline(r.X, r.right(), r.bottom()-2, 2)
	c.vline(r.X, r.Y, r.bottom(), 2)
	c.vline(r.right()-2, r.Y, r.bottom(), 2)
	c.fill(R{r.X + 4, r.Y + 4, int(math.Round(float64(r.W-8) * frac)), r.H - 8})
}

// tri is a solid triangle pointing up (dir -1) or down (1). Lucida has no
// ▲▼, so they are drawn.
func (c *ctx) tri(cx, cy, s float64, dir int) {
	c.setInk()
	if dir < 0 {
		c.dc.MoveTo(cx-s/2, cy+s*0.3)
		c.dc.LineTo(cx+s/2, cy+s*0.3)
		c.dc.LineTo(cx, cy-s*0.45)
	} else {
		c.dc.MoveTo(cx-s/2, cy-s*0.3)
		c.dc.LineTo(cx+s/2, cy-s*0.3)
		c.dc.LineTo(cx, cy+s*0.45)
	}
	c.dc.ClosePath()
	c.dc.Fill()
}

// arrow is a thick right arrow (Lucida has no →).
func (c *ctx) arrow(x, cy, w float64) {
	c.setInk()
	c.dc.SetLineWidth(3)
	c.dc.DrawLine(x, cy, x+w-6, cy)
	c.dc.Stroke()
	c.dc.MoveTo(x+w-9, cy-7)
	c.dc.LineTo(x+w, cy)
	c.dc.LineTo(x+w-9, cy+7)
	c.dc.ClosePath()
	c.dc.Fill()
}

// drawFrame is the "frames" divider: a 2 px rounded frame inside the cell.
func drawFrame(dc *gg.Context, r R) {
	dc.SetRGB(0, 0, 0)
	dc.SetLineWidth(2)
	dc.DrawRoundedRectangle(float64(r.X)+3, float64(r.Y)+3, float64(r.W)-6, float64(r.H)-6, 8)
	dc.Stroke()
}

// formatNumber formats a numeric state with the given decimals; a negative
// decimals keeps the state as reported. Non-numeric states pass through.
func formatNumber(state string, decimals float64) string {
	f, err := strconv.ParseFloat(state, 64)
	if err != nil || decimals < 0 {
		return state
	}
	return strconv.FormatFloat(f, 'f', int(decimals), 64)
}

const unavailable = "–"

// frameGap is the gutter between neighbouring widgets.
const frameGap = 4
