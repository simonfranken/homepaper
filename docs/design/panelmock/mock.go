// Scratch generator for the homepaper panel redesign. It uses the
// renderer's own fonts (Lucida Sans bitmaps, read unmodified from the
// repo, and the Go fonts) and the same pipeline: anti-aliased shapes,
// whole-pixel text, greyscale, threshold at 128, 1-bit PNG at 800×480.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"strings"

	"github.com/fogleman/gg"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

const (
	PW, PH = 800, 480
	CW, CH = 100, 80
)

type R struct{ X, Y, W, H int }

func cell(x, y, w, h int) R { return R{x * CW, y * CH, w * CW, h * CH} }
func (r R) in(dx, dy int) R { return R{r.X + dx, r.Y + dy, r.W - 2*dx, r.H - 2*dy} }
func (r R) right() int      { return r.X + r.W }
func (r R) bottom() int     { return r.Y + r.H }

type Style int

const (
	Rules Style = iota
	Frames
	Bare
)

type M struct {
	dc    *gg.Context
	img   *image.RGBA
	fg    color.Gray // current ink
	style Style
	// grey mode (2bpp, D-018 draft): tones[1] is dark grey, tones[2] light
	// grey. Each is a coverage layer: whatever is drawn black on it becomes
	// that grey, unless the ink layer is black there. Ink always wins.
	grey  bool
	tones [3]*gg.Context
}

// Grey levels of the 2bpp panel, as shown in the PNGs.
const (
	toneDark  = 1
	toneLight = 2
)

var greyPalette = color.Palette{color.Gray{0}, color.Gray{0x55}, color.Gray{0xAA}, color.Gray{0xFF}}

func newGreyM(style Style) *M {
	m := newM(style)
	m.grey = true
	for _, t := range []int{toneDark, toneLight} {
		dc := gg.NewContext(PW, PH)
		dc.SetColor(color.White)
		dc.Clear()
		m.tones[t] = dc
	}
	return m
}

// tone runs f with drawing redirected to a grey layer. In 1-bit mode, or on
// inverted widgets, f draws in normal ink, so the 1-bit images stay as they
// were. Grey is only for pixels that don't change with entity state or time.
func (m *M) tone(level int, f func()) {
	if !m.grey || m.fg.Y != 0 {
		f()
		return
	}
	dc, img := m.dc, m.img
	m.dc, m.img = m.tones[level], m.tones[level].Image().(*image.RGBA)
	f()
	m.dc, m.img = dc, img
}

// toneOr draws f on a grey layer in grey mode and alt in 1-bit mode.
func (m *M) toneOr(level int, f, alt func()) {
	if m.grey && m.fg.Y == 0 {
		m.tone(level, f)
	} else {
		alt()
	}
}

func newM(style Style) *M {
	dc := gg.NewContext(PW, PH)
	dc.SetColor(color.White)
	dc.Clear()
	return &M{dc: dc, img: dc.Image().(*image.RGBA), fg: color.Gray{0}, style: style}
}

func (m *M) ink() (float64, float64, float64) {
	v := float64(m.fg.Y) / 255
	return v, v, v
}

// ---------- text ----------

func capH(f font.Face) int {
	mt := f.Metrics()
	if mt.CapHeight > 0 {
		return int(math.Round(float64(mt.CapHeight) / 64))
	}
	return int(math.Round(float64(mt.Ascent) / 64 * 0.72))
}

func width(f font.Face, s string) int { return int(math.Ceil(float64(font.MeasureString(f, s)) / 64)) }

// T draws s with its baseline at y; ax = 0 left, 0.5 centre, 1 right of x.
func (m *M) T(s string, id fontID, px, x, y int, ax float64) int {
	f := face(id, px)
	w := width(f, s)
	x0 := x - int(math.Round(ax*float64(w)))
	d := &font.Drawer{Dst: m.img, Src: image.NewUniform(m.fg), Face: f, Dot: fixed.P(x0, y)}
	d.DrawString(s)
	return w
}

// fit returns the largest size from sizes at which s fits maxW and whose
// cap height fits maxCap.
func fit(s string, id fontID, sizes []int, maxW, maxCap int) int {
	for _, px := range sizes {
		f := face(id, px)
		if width(f, s) <= maxW && capH(f) <= maxCap {
			return px
		}
	}
	return sizes[len(sizes)-1]
}

var big = []int{96, 88, 80, 72, 64, 56, 52, 48, 44, 40, 34, 25, 20, 17, 14, 12}

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
func (m *M) label(r R, s string) R {
	px := 17
	if width(face(fontMedium, px), s) > r.W {
		px = 14
	}
	s = ellip(s, fontMedium, px, r.W)
	f := face(fontMedium, px)
	m.tone(toneDark, func() { m.T(s, fontMedium, px, r.X, r.Y+capH(f), 0) })
	used := capH(f) + 9
	return R{r.X, r.Y + used, r.W, r.H - used}
}

// split value: integer part big, ".4°" smaller and top-aligned, as in print.
func (m *M) bigNumber(r R, whole, frac string, maxPx int, ax float64) {
	var sizes []int
	for _, s := range big {
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
	x += m.T(whole, fontBold, px, x, base, 0)
	m.T(frac, fontBold, sp, x+1, base-ch+capH(fsm), 0)
}

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

// ---------- shapes ----------

func (m *M) fillRect(r R, g uint8) {
	draw.Draw(m.img, image.Rect(r.X, r.Y, r.right(), r.bottom()), image.NewUniform(color.Gray{g}), image.Point{}, draw.Src)
}

func (m *M) hline(x0, x1, y, t int) { m.fillRect(R{x0, y, x1 - x0, t}, m.fg.Y) }
func (m *M) vline(x, y0, y1, t int) { m.fillRect(R{x, y0, t, y1 - y0}, m.fg.Y) }

// dotted 1 px line, one pixel on, two off.
func (m *M) dotted(x0, x1, y int) {
	m.toneOr(toneLight, func() { m.hline(x0, x1, y, 2) }, func() {
		for x := x0; x < x1; x += 3 {
			m.img.Set(x, y, m.fg)
		}
	})
}

// bar is a 2 px outlined meter, filled to frac.
func (m *M) bar(r R, frac float64) {
	m.hline(r.X, r.right(), r.Y, 2)
	m.hline(r.X, r.right(), r.bottom()-2, 2)
	m.vline(r.X, r.Y, r.bottom(), 2)
	m.vline(r.right()-2, r.Y, r.bottom(), 2)
	fw := int(math.Round(float64(r.W-8) * frac))
	m.fillRect(R{r.X + 4, r.Y + 4, fw, r.H - 8}, m.fg.Y)
}

// tri draws a solid triangle pointing up (dir -1), down (1) or right (0).
func (m *M) tri(cx, cy, s float64, dir int) {
	m.dc.SetRGB(m.ink())
	switch dir {
	case -1:
		m.dc.MoveTo(cx-s/2, cy+s*0.3)
		m.dc.LineTo(cx+s/2, cy+s*0.3)
		m.dc.LineTo(cx, cy-s*0.45)
	case 1:
		m.dc.MoveTo(cx-s/2, cy-s*0.3)
		m.dc.LineTo(cx+s/2, cy-s*0.3)
		m.dc.LineTo(cx, cy+s*0.45)
	default:
		m.dc.MoveTo(cx-s*0.3, cy-s/2)
		m.dc.LineTo(cx-s*0.3, cy+s/2)
		m.dc.LineTo(cx+s*0.45, cy)
	}
	m.dc.ClosePath()
	m.dc.Fill()
}

// arrow is a thick right arrow (Lucida has no →).
func (m *M) arrow(x, cy, w float64) {
	m.dc.SetRGB(m.ink())
	m.dc.SetLineWidth(3)
	m.dc.DrawLine(x, cy, x+w-6, cy)
	m.dc.Stroke()
	m.dc.MoveTo(x+w-9, cy-7)
	m.dc.LineTo(x+w, cy)
	m.dc.LineTo(x+w-9, cy+7)
	m.dc.ClosePath()
	m.dc.Fill()
}

func (m *M) droplet(cx, cy, s float64) {
	m.dc.SetRGB(m.ink())
	m.dc.MoveTo(cx, cy-s*0.5)
	m.dc.CubicTo(cx+s*0.1, cy-s*0.25, cx+s*0.36, cy-s*0.02, cx+s*0.36, cy+s*0.16)
	m.dc.CubicTo(cx+s*0.36, cy+s*0.38, cx+s*0.18, cy+s*0.5, cx, cy+s*0.5)
	m.dc.CubicTo(cx-s*0.18, cy+s*0.5, cx-s*0.36, cy+s*0.38, cx-s*0.36, cy+s*0.16)
	m.dc.CubicTo(cx-s*0.36, cy-s*0.02, cx-s*0.1, cy-s*0.25, cx, cy-s*0.5)
	m.dc.Fill()
}

func (m *M) flame(cx, cy, s float64) {
	m.dc.SetRGB(m.ink())
	m.dc.MoveTo(cx, cy-s*0.5)
	m.dc.CubicTo(cx+s*0.42, cy-s*0.1, cx+s*0.4, cy+s*0.2, cx+s*0.32, cy+s*0.32)
	m.dc.CubicTo(cx+s*0.22, cy+s*0.48, cx-s*0.22, cy+s*0.48, cx-s*0.32, cy+s*0.32)
	m.dc.CubicTo(cx-s*0.42, cy+s*0.12, cx-s*0.24, cy-s*0.06, cx-s*0.12, cy-s*0.18)
	m.dc.CubicTo(cx-s*0.1, cy-s*0.02, cx-s*0.02, cy+s*0.02, cx+s*0.02, cy-s*0.06)
	m.dc.CubicTo(cx+s*0.08, cy-s*0.2, cx+s*0.04, cy-s*0.36, cx, cy-s*0.5)
	m.dc.Fill()
}

func (m *M) door(cx, cy, s float64, open bool) {
	m.dc.SetRGB(m.ink())
	m.dc.SetLineWidth(math.Max(2.5, s*0.09))
	w, h := s*0.5, s*0.86
	x0, y0 := cx-w/2, cy-h/2
	if !open {
		m.dc.DrawRectangle(x0, y0, w, h)
		m.dc.Stroke()
		m.dc.DrawCircle(x0+w*0.75, cy+s*0.04, s*0.06)
		m.dc.Fill()
		return
	}
	m.dc.DrawLine(x0, y0, x0+w, y0)
	m.dc.DrawLine(x0+w, y0, x0+w, y0+h)
	m.dc.DrawLine(x0-s*0.06, y0+h, x0+w+s*0.06, y0+h)
	m.dc.Stroke()
	m.dc.MoveTo(x0, y0)
	m.dc.LineTo(x0+w*0.55, y0+h*0.12)
	m.dc.LineTo(x0+w*0.55, y0+h*1.0)
	m.dc.LineTo(x0, y0+h)
	m.dc.ClosePath()
	m.dc.Fill()
}

func (m *M) window(cx, cy, s float64, open bool) {
	m.dc.SetRGB(m.ink())
	lw := math.Max(2.5, s*0.08)
	m.dc.SetLineWidth(lw)
	w, h := s*0.74, s*0.74
	x0, y0 := cx-w/2, cy-h/2
	if !open {
		m.dc.DrawRectangle(x0, y0, w, h)
		m.dc.DrawLine(cx, y0, cx, y0+h)
		m.dc.DrawLine(x0, cy, x0+w, cy)
		m.dc.Stroke()
		return
	}
	// tilted sash
	m.dc.DrawRectangle(x0, y0, w, h)
	m.dc.Stroke()
	m.dc.MoveTo(x0+lw, y0+h-lw)
	m.dc.LineTo(x0+w-lw, y0+h-lw)
	m.dc.LineTo(x0+w*0.82, y0+h*0.18)
	m.dc.LineTo(x0+w*0.18, y0+h*0.18)
	m.dc.ClosePath()
	m.dc.Fill()
}

func (m *M) person(cx, cy, s float64, home bool) {
	m.dc.SetRGB(m.ink())
	m.dc.SetLineWidth(math.Max(2.5, s*0.09))
	m.dc.DrawCircle(cx, cy-s*0.2, s*0.17)
	if home {
		m.dc.Fill()
	} else {
		m.dc.Stroke()
	}
	m.dc.DrawArc(cx, cy+s*0.42, s*0.32, math.Pi, 2*math.Pi)
	m.dc.ClosePath()
	if home {
		m.dc.Fill()
	} else {
		m.dc.Stroke()
	}
}

func (m *M) bolt(cx, cy, s float64) {
	m.dc.SetRGB(m.ink())
	m.dc.MoveTo(cx+s*0.08, cy-s*0.5)
	m.dc.LineTo(cx-s*0.28, cy+s*0.06)
	m.dc.LineTo(cx-s*0.02, cy+s*0.06)
	m.dc.LineTo(cx-s*0.1, cy+s*0.5)
	m.dc.LineTo(cx+s*0.28, cy-s*0.08)
	m.dc.LineTo(cx+s*0.02, cy-s*0.08)
	m.dc.ClosePath()
	m.dc.Fill()
}

func (m *M) lock(cx, cy, s float64, locked bool) {
	m.dc.SetRGB(m.ink())
	m.dc.SetLineWidth(math.Max(2.5, s*0.1))
	if locked {
		m.dc.DrawArc(cx, cy-s*0.08, s*0.2, math.Pi, 2*math.Pi)
	} else {
		m.dc.DrawArc(cx+s*0.26, cy-s*0.16, s*0.2, math.Pi, 2*math.Pi)
	}
	m.dc.Stroke()
	m.dc.DrawRoundedRectangle(cx-s*0.3, cy-s*0.08, s*0.6, s*0.48, s*0.06)
	m.dc.Fill()
}

// ---------- widget chrome ----------

type W struct {
	R     R
	Alert bool
	Draw  func(m *M, r R)
}

// content returns the drawable area of a widget.
func content(r R) R { return R{r.X + 12, r.Y + 11, r.W - 24, r.H - 22} }

func (m *M) layout(ws []W) {
	for _, w := range ws {
		m.fg = color.Gray{0}
		switch m.style {
		case Frames:
			m.dc.SetRGB(0, 0, 0)
			m.dc.SetLineWidth(2)
			m.dc.DrawRoundedRectangle(float64(w.R.X)+3, float64(w.R.Y)+3, float64(w.R.W)-6, float64(w.R.H)-6, 8)
			m.dc.Stroke()
		}
		if w.Alert {
			m.dc.SetRGB(0, 0, 0)
			m.dc.DrawRoundedRectangle(float64(w.R.X)+4, float64(w.R.Y)+4, float64(w.R.W)-8, float64(w.R.H)-8, 8)
			m.dc.Fill()
			m.fg = color.Gray{255}
		}
		w.Draw(m, content(w.R))
		m.fg = color.Gray{0}
	}
	if m.style == Rules {
		// Column rules in the gutters, stopping short of crossings, as in print.
		for _, w := range ws {
			if w.R.right() < PW && !w.Alert {
				m.vline(w.R.right()-1, w.R.Y+10, w.R.bottom()-10, 2)
			}
			if w.R.bottom() < PH && !w.Alert {
				m.hline(w.R.X+10, w.R.right()-10, w.R.bottom()-1, 2)
			}
		}
	}
}

// ---------- widgets ----------

func temperature(label, whole, frac, hum string) func(*M, R) {
	return func(m *M, r R) {
		if r.W < 120 {
			// 1×1: smaller label, value only.
			f := face(fontMedium, 14)
			m.tone(toneDark, func() { m.T(ellip(label, fontMedium, 14, r.W), fontMedium, 14, r.X, r.Y+capH(f), 0) })
			m.bigNumber(R{r.X, r.Y + capH(f) + 6, r.W, r.H - capH(f) - 6}, whole, frac, 34, 0)
			return
		}
		r = m.label(r, label)
		main := r
		if hum != "" && r.H >= 90 {
			main = R{r.X, r.Y, r.W, r.H - 26}
			f := face(fontRegular, 17)
			y := r.bottom() - 2
			m.droplet(float64(r.X+6), float64(y-capH(f)/2-1), 14)
			m.T(hum+" %", fontRegular, 17, r.X+18, y, 0)
		}
		m.bigNumber(main, whole, frac, 80, 0)
	}
}

func value(label, val, unit string, icon func(m *M, cx, cy, s float64)) func(*M, R) {
	return func(m *M, r R) {
		small := r.W < 120
		if small {
			f := face(fontMedium, 14)
			m.tone(toneDark, func() { m.T(ellip(label, fontMedium, 14, r.W), fontMedium, 14, r.X, r.Y+capH(f), 0) })
			r = R{r.X, r.Y + capH(f) + 6, r.W, r.H - capH(f) - 6}
		} else {
			r = m.label(r, label)
		}
		x := r.X
		if icon != nil && !small {
			s := math.Min(float64(r.H), 30)
			icon(m, float64(r.X)+s/2, float64(r.Y)+float64(r.H)/2, s)
			x += int(s) + 8
		}
		up := 25
		if small {
			up = 17
		}
		uw := width(face(fontRegular, up), " "+unit)
		px := fit(val, fontBold, []int{64, 56, 48, 44, 40, 34, 25, 20}, r.right()-x-uw, r.H)
		fb := face(fontBold, px)
		base := r.Y + (r.H+capH(fb))/2
		x += m.T(val, fontBold, px, x, base, 0)
		m.T(" "+unit, fontRegular, up, x, base, 0)
	}
}

func thermostat(label, cur, target, action string) func(*M, R) {
	return func(m *M, r R) {
		r = m.label(r, label)
		heating := action == "Heating"
		if r.H < 80 {
			// one row: 21.4° ▶ 22.0°   ▲ Heating
			f := face(fontBold, 34)
			base := r.Y + (r.H+capH(f))/2
			x := r.X
			x += m.T(cur, fontBold, 34, x, base, 0)
			x += 10
			m.arrow(float64(x), float64(base-capH(f)/2), 22)
			x += 32
			m.T(target, fontBold, 34, x, base, 0)
			if r.W > 300 {
				fa := face(fontBold, 20)
				aw := width(fa, action)
				m.T(action, fontBold, 20, r.right(), base, 1)
				if heating {
					m.flame(float64(r.right()-aw-14), float64(base-capH(fa)/2), 20)
				}
			}
			return
		}
		// tall: current huge on the left, target and action on the right.
		col := r.W * 45 / 100
		left := R{r.X, r.Y, r.W - col, r.H}
		w, fr := splitNum(cur)
		m.bigNumber(left, w, fr, 88, 0)
		rx := r.right() - col + 16
		m.tone(toneDark, func() { m.vline(rx-16, r.Y+4, r.bottom()-4, 2) })
		ft := face(fontMedium, 17)
		y := r.Y + capH(ft) + 6
		m.tone(toneDark, func() { m.T("Set to", fontMedium, 17, rx, y, 0) })
		fb := face(fontBold, 34)
		y += capH(fb) + 12
		m.T(target, fontBold, 34, rx, y, 0)
		fa := face(fontBold, 20)
		y = r.bottom() - 4
		if heating {
			m.flame(float64(rx+9), float64(y-capH(fa)/2), 22)
			m.T(action, fontBold, 20, rx+26, y, 0)
		} else {
			m.T(action, fontRegular, 20, rx, y, 0)
		}
	}
}

func splitNum(s string) (string, string) {
	if i := strings.IndexByte(s, '.'); i >= 0 {
		return s[:i], s[i:]
	}
	if strings.HasSuffix(s, "°") {
		return strings.TrimSuffix(s, "°"), "°"
	}
	return s, ""
}

type day struct {
	name, cond string
	hi, lo     int
}

func weather(cond, condLabel string, temp, hi, lo, hum int, fc []day) func(*M, R) {
	return func(m *M, r R) {
		top := r
		if len(fc) > 0 && r.H >= 120 {
			top = R{r.X, r.Y, r.W, r.H - 64}
		}
		s := math.Min(float64(top.H), float64(r.W)*0.32)
		m.weatherIcon(cond, float64(top.X)+s/2, float64(top.Y)+float64(top.H)/2, s)
		x := top.X + int(s) + 10
		tf := face(fontBold, 64)
		if top.H < 70 {
			tf = face(fontBold, 44)
		}
		px := 64
		if top.H < 70 {
			px = 44
		}
		base := top.Y + (top.H+capH(tf))/2
		x += m.T(fmt.Sprintf("%d", temp), fontBold, px, x, base, 0)
		x += m.T("°", fontBold, smallFor(px), x+1, base-capH(tf)+capH(face(fontBold, smallFor(px))), 0)
		x += 16
		if r.W-(x-r.X) <= 120 && top.H >= 120 {
			// tall and narrow: condition goes underneath.
			fr := face(fontBold, 17)
			m.T(ellip(condLabel, fontBold, 17, r.W), fontBold, 17, r.X, top.bottom()-2, 0)
			_ = fr
		}
		if r.W-(x-r.X) > 120 {
			fb := face(fontBold, 20)
			fr := face(fontRegular, 17)
			lines := 3
			if top.H < 70 {
				lines = 2
			}
			gap := 8
			total := capH(fb) + (lines-1)*(capH(fr)+gap)
			y := top.Y + (top.H-total)/2 + capH(fb)
			m.T(condLabel, fontBold, 20, x, y, 0)
			y += capH(fr) + gap + 2
			m.tone(toneDark, func() { m.tri(float64(x+5), float64(y-capH(fr)/2), 10, -1) })
			xx := x + 14
			xx += m.T(fmt.Sprintf("%d°", hi), fontRegular, 17, xx, y, 0)
			m.tone(toneDark, func() { m.tri(float64(xx+12), float64(y-capH(fr)/2), 10, 1) })
			m.T(fmt.Sprintf("%d°", lo), fontRegular, 17, xx+21, y, 0)
			if lines == 3 {
				y += capH(fr) + gap
				m.tone(toneDark, func() { m.droplet(float64(x+5), float64(y-capH(fr)/2), 12) })
				m.T(fmt.Sprintf("%d %%", hum), fontRegular, 17, x+14, y, 0)
			}
		}
		if top.H < r.H {
			y0 := top.bottom() + 4
			m.dotted(r.X, r.right(), y0)
			cw := r.W / len(fc)
			for i, d := range fc {
				cx := r.X + i*cw + cw/2
				f14 := face(fontBold, 14)
				m.tone(toneDark, func() { m.T(d.name, fontBold, 14, cx, y0+8+capH(f14), 0.5) })
				m.weatherIcon(d.cond, float64(cx), float64(y0+35), 26)
				hs, ls := fmt.Sprintf("%d°", d.hi), fmt.Sprintf(" %d°", d.lo)
				hw, lw := width(face(fontBold, 14), hs), width(face(fontRegular, 14), ls)
				xx := cx - (hw+lw)/2
				xx += m.T(hs, fontBold, 14, xx, r.bottom(), 0)
				m.T(ls, fontRegular, 14, xx, r.bottom(), 0)
			}
		}
	}
}

type light struct {
	name string
	on   bool
	pct  int
}

func lights(heading string, ls []light, bars bool) func(*M, R) {
	return func(m *M, r R) {
		n := 0
		for _, l := range ls {
			if l.on {
				n++
			}
		}
		f17 := face(fontMedium, 17)
		m.tone(toneDark, func() { m.T(heading, fontMedium, 17, r.X, r.Y+capH(f17), 0) })
		summary := fmt.Sprintf("%d of %d on", n, len(ls))
		if n == 0 {
			summary = "All off"
		}
		m.T(summary, fontBold, 17, r.right(), r.Y+capH(f17), 1)
		r = R{r.X, r.Y + capH(f17) + 10, r.W, r.H - capH(f17) - 10}
		if r.H < 40 {
			// Summary row of bulbs only.
			for i, l := range ls {
				iconBulb(m.dc, float64(r.X+14+i*30), float64(r.Y+r.H/2), 26, l.on)
			}
			return
		}
		rowH := r.H / len(ls)
		if rowH > 46 {
			rowH = 46
		}
		for i, l := range ls {
			y := r.Y + i*rowH
			if i > 0 {
				m.dotted(r.X, r.right(), y)
			}
			cy := y + rowH/2
			iconBulb(m.dc, float64(r.X+11), float64(cy+1), 24, l.on)
			id := fontRegular
			if l.on {
				id = fontBold
			}
			fr := face(id, 17)
			base := cy + capH(fr)/2
			stateW := 50
			if bars && r.W > 280 {
				stateW = 140
			}
			m.T(ellip(l.name, id, 17, r.W-30-stateW), id, 17, r.X+30, base, 0)
			if l.on {
				m.T(fmt.Sprintf("%d %%", l.pct), fontBold, 17, r.right(), base, 1)
				if bars && r.W > 280 {
					m.bar(R{r.right() - 136, cy - 6, 70, 12}, float64(l.pct)/100)
				}
			} else {
				m.T("Off", fontRegular, 17, r.right(), base, 1)
			}
		}
	}
}

func state(label, word string, icon func(m *M, cx, cy, s float64)) func(*M, R) {
	return func(m *M, r R) {
		if r.W < 120 {
			f := face(fontMedium, 14)
			m.tone(toneDark, func() { m.T(ellip(label, fontMedium, 14, r.W), fontMedium, 14, r.X, r.Y+capH(f), 0) })
			rest := R{r.X, r.Y + capH(f) + 6, r.W, r.H - capH(f) - 6}
			px := fit(word, fontBold, []int{20, 17, 14}, rest.W-26, rest.H)
			fb := face(fontBold, px)
			base := rest.Y + (rest.H+capH(fb))/2
			if width(fb, word) <= rest.W-26 {
				icon(m, float64(rest.X+10), float64(base-capH(fb)/2), 20)
				m.T(word, fontBold, px, rest.X+26, base, 0)
			} else {
				m.T(word, fontBold, fit(word, fontBold, []int{20, 17, 14}, rest.W, rest.H), rest.X, base, 0)
			}
			return
		}
		r = m.label(r, label)
		fb := face(fontBold, 25)
		base := r.Y + (r.H+capH(fb))/2
		icon(m, float64(r.X+13), float64(base-capH(fb)/2), 28)
		m.T(word, fontBold, 25, r.X+36, base, 0)
	}
}

func clock(timeS, dayS, dateS string, withTime bool) func(*M, R) {
	return func(m *M, r R) {
		x := r.X
		fb := face(fontBold, 20)
		fr := face(fontRegular, 17)
		if withTime {
			tf := face(fontBold, 56)
			base := r.Y + (r.H+capH(tf))/2
			x += m.T(timeS, fontBold, 56, x, base, 0) + 18
			if dayS == "" {
				return
			}
			m.tone(toneDark, func() { m.vline(x-9, r.Y+6, r.bottom()-6, 2) })
		}
		total := capH(fb) + 10 + capH(fr)
		y := r.Y + (r.H-total)/2 + capH(fb)
		m.T(dayS, fontBold, 20, x+4, y, 0)
		m.tone(toneDark, func() { m.T(dateS, fontRegular, 17, x+4, y+10+capH(fr), 0) })
	}
}

func heading(s string) func(*M, R) {
	return func(m *M, r R) {
		f := face(fontBold, 25)
		base := r.Y + (r.H+capH(f))/2
		w := m.T(s, fontBold, 25, r.X, base, 0)
		m.tone(toneDark, func() { m.hline(r.X+w+12, r.right(), base-capH(f)/2, 2) })
	}
}

func note(s string) func(*M, R) {
	return func(m *M, r R) {
		f := face(fontRegular, 17)
		m.T(ellip(s, fontRegular, 17, r.W), fontRegular, 17, r.X, r.Y+(r.H+capH(f))/2, 0)
	}
}

func unavailableW(label string) func(*M, R) {
	return func(m *M, r R) {
		r = m.label(r, label)
		f := face(fontBold, 34)
		base := r.Y + (r.H+capH(f))/2
		if m.grey {
			// A light grey hatch says "no data" without competing with values.
			m.tone(toneLight, func() {
				m.dc.SetRGB(0, 0, 0)
				m.dc.SetLineWidth(2)
				m.dc.Push()
				m.dc.DrawRectangle(float64(r.X), float64(r.Y), float64(r.W), float64(r.H))
				m.dc.Clip()
				for x := r.X - r.H; x < r.right(); x += 10 {
					m.dc.DrawLine(float64(x), float64(r.bottom()), float64(x+r.H), float64(r.Y))
				}
				m.dc.Stroke()
				m.dc.ResetClip()
				m.dc.Pop()
			})
			fs := face(fontRegular, 14)
			tw := width(f, "–") + 12 + width(fs, "Not available")
			// White plate behind the words so the hatch doesn't run through them.
			m.fillRect(R{r.X, base - capH(f) - 8, tw + 12, capH(f) + 16}, 255)
			for _, t := range []int{toneLight} {
				tl := m.tones[t].Image().(*image.RGBA)
				draw.Draw(tl, image.Rect(r.X, base-capH(f)-8, r.X+tw+12, base+8), image.White, image.Point{}, draw.Src)
			}
			m.tone(toneDark, func() {
				w := m.T("–", fontBold, 34, r.X, base, 0)
				m.T("Not available", fontRegular, 14, r.X+w+12, base, 0)
			})
			return
		}
		w := m.T("–", fontBold, 34, r.X, base, 0)
		m.T("Not available", fontRegular, 14, r.X+w+12, base, 0)
	}
}

// ---------- output ----------

func (m *M) save(name string) {
	gray := image.NewGray(image.Rect(0, 0, PW, PH))
	draw.Draw(gray, gray.Bounds(), m.img, image.Point{}, draw.Src)
	pal := color.Palette{color.Black, color.White}
	if m.grey {
		m.saveGrey(name, gray)
		return
	}
	out := image.NewPaletted(gray.Bounds(), pal)
	for y := 0; y < PH; y++ {
		for x := 0; x < PW; x++ {
			if gray.GrayAt(x, y).Y >= 128 {
				out.SetColorIndex(x, y, 1)
			}
		}
	}
	f, err := os.Create(name)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, out); err != nil {
		panic(err)
	}
}

// saveGrey composes ink and the two tone layers into a 2bpp PNG: black ink
// wins, then dark grey, then light grey, else white. Every layer is
// thresholded at 50 % coverage, so no anti-aliasing fringe turns grey.
func (m *M) saveGrey(name string, ink *image.Gray) {
	out := image.NewPaletted(ink.Bounds(), greyPalette)
	dark := image.NewGray(ink.Bounds())
	light := image.NewGray(ink.Bounds())
	draw.Draw(dark, dark.Bounds(), m.tones[toneDark].Image(), image.Point{}, draw.Src)
	draw.Draw(light, light.Bounds(), m.tones[toneLight].Image(), image.Point{}, draw.Src)
	for y := 0; y < PH; y++ {
		for x := 0; x < PW; x++ {
			idx := uint8(3)
			switch {
			case ink.GrayAt(x, y).Y < 128:
				idx = 0
			case dark.GrayAt(x, y).Y < 128:
				idx = 1
			case light.GrayAt(x, y).Y < 128:
				idx = 2
			}
			out.SetColorIndex(x, y, idx)
		}
	}
	f, err := os.Create(name)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, out); err != nil {
		panic(err)
	}
}

// ---------- data (from internal/ha/demo.go, plus a forecast) ----------

var forecast = []day{
	{"Tue", "rainy", 9, 4}, {"Wed", "partlycloudy", 11, 5}, {"Thu", "cloudy", 8, 3},
	{"Fri", "snowy", 4, -1}, {"Sat", "sunny", 7, 0},
}

var demoLights = []light{{"Living room light", true, 71}, {"Kitchen light", false, 0}, {"Hallway", true, 100}}

func iDoor(open bool) func(*M, float64, float64, float64) {
	return func(m *M, cx, cy, s float64) { m.door(cx, cy, s, open) }
}
func iWin(open bool) func(*M, float64, float64, float64) {
	return func(m *M, cx, cy, s float64) { m.window(cx, cy, s, open) }
}
func iPerson(home bool) func(*M, float64, float64, float64) {
	return func(m *M, cx, cy, s float64) { m.person(cx, cy, s, home) }
}
func iBolt(m *M, cx, cy, s float64) { m.bolt(cx, cy, s) }

func hero(style Style, alert bool) []W {
	win := W{R: cell(2, 5, 2, 1), Draw: state("Bathroom window", "Closed", iWin(false))}
	if alert {
		win = W{R: cell(2, 5, 2, 1), Alert: true, Draw: state("Bathroom window", "Open", iWin(true))}
	}
	return []W{
		{R: cell(0, 0, 4, 1), Draw: clock("18:42", "Monday", "16 November", true)},
		{R: cell(4, 0, 4, 2), Draw: weather("partlycloudy", "Partly cloudy", 8, 9, 3, 81, forecast)},
		{R: cell(0, 1, 2, 2), Draw: temperature("Living room", "21", ".4°", "48")},
		{R: cell(2, 1, 2, 2), Draw: temperature("Outdoor", "7", ".8°", "")},
		{R: cell(0, 3, 4, 2), Draw: thermostat("Living room thermostat", "21.4°", "22.0°", "Heating")},
		{R: cell(4, 2, 4, 3), Draw: lights("Lights", demoLights, true)},
		{R: cell(0, 5, 2, 1), Draw: state("Front door", "Closed", iDoor(false))},
		win,
		{R: cell(4, 5, 2, 1), Draw: state("Alex", "Home", iPerson(true))},
		{R: cell(6, 5, 2, 1), Draw: value("Power", "412", "W", iBolt)},
	}
}

func main() {
	dir := ".."
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	m := newM(Rules)
	m.layout(hero(Rules, true))
	m.save(dir + "/hero-rules.png")

	m = newM(Frames)
	m.layout(hero(Frames, true))
	m.save(dir + "/hero-frames.png")

	m = newM(Rules)
	m.layout(hero(Rules, false))
	m.save(dir + "/hero-calm.png")

	// Exceptions: things unavailable, all lights off, door open, away.
	m = newM(Rules)
	m.layout(exceptionsLayout())
	m.save(dir + "/exceptions.png")

	// Specimen A: temperature, value, thermostat at their sizes.
	m = newM(Rules)
	m.layout([]W{
		{R: cell(0, 0, 1, 1), Draw: temperature("Living room", "21", ".4°", "48")},
		{R: cell(1, 0, 2, 1), Draw: temperature("Living room", "21", ".4°", "48")},
		{R: cell(3, 0, 1, 1), Draw: value("Power", "412", "W", iBolt)},
		{R: cell(4, 0, 2, 1), Draw: value("Power", "412", "W", iBolt)},
		{R: cell(6, 0, 2, 1), Draw: value("Humidity", "48", "%", func(m *M, cx, cy, s float64) { m.droplet(cx, cy, s*0.8) })},
		{R: cell(0, 1, 2, 2), Draw: temperature("Living room", "21", ".4°", "48")},
		{R: cell(2, 1, 3, 3), Draw: temperature("Living room", "21", ".4°", "48")},
		{R: cell(5, 1, 3, 1), Draw: thermostat("Thermostat", "21.4°", "22.0°", "Heating")},
		{R: cell(5, 2, 3, 2), Draw: thermostat("Thermostat", "21.4°", "22.0°", "Heating")},
		{R: cell(0, 3, 2, 1), Draw: value("Outdoor", "7.8", "°C", nil)},
		{R: cell(0, 4, 4, 1), Draw: thermostat("Living room thermostat", "21.4°", "22.0°", "Heating")},
		{R: cell(4, 4, 4, 2), Draw: thermostat("Bedroom thermostat", "17.9°", "18.0°", "Idle")},
		{R: cell(0, 5, 4, 1), Draw: heading("Downstairs")},
	})
	m.save(dir + "/specimen-a.png")

	// Specimen B: weather, lights, state, clock, text.
	m = newM(Rules)
	m.layout(specimenB())
	m.save(dir + "/specimen-b.png")

	// Type sheet: every size the panel has, both weights.
	m = newM(Bare)
	y := 8
	for _, px := range []int{12, 14, 17, 20, 25, 34} {
		f := face(fontRegular, px)
		y += capH(f) + px/2 + 8
		m.T(fmt.Sprintf("%d", px), fontBold, 12, 8, y, 0)
		x := 48
		x += m.T("Living room 21.4° Hallway 71 %", fontRegular, px, x, y, 0) + 24
		m.T("Cloudy 412 W", fontBold, px, x, y, 0)
	}
	y += 16
	m.hline(8, PW-8, y, 2)
	y += 14
	x := 48
	for _, px := range []int{44, 56, 64, 80, 96} {
		fb := face(fontBold, px)
		m.T(fmt.Sprintf("%d", px), fontBold, 12, x, y+12, 0)
		w := m.T("21", fontBold, px, x, y+20+capH(fb), 0)
		x += w + 26
	}
	m.save(dir + "/type.png")

	// 2bpp variants (D-018). The renderer owns grey-hero-*.png (TestGolden
	// with -update writes them); these are the mockup-only ones.
	m = newGreyM(Rules)
	m.layout(exceptionsLayout())
	m.save(dir + "/grey-exceptions.png")

	m = newGreyM(Rules)
	m.layout(specimenB())
	m.save(dir + "/grey-specimen-b.png")

	m = newGreyM(Bare)
	greySheet(m)
	m.save(dir + "/grey-rules.png")
}

// greySheet is the legend: the four levels and where each may be used.
func greySheet(m *M) {
	m.T("Four levels", fontBold, 25, 24, 44, 0)
	names := []string{"Black", "Dark grey", "Light grey", "White"}
	uses := []string{"values, units, state words, state icons, meters", "labels, rules, static icons", "row lines, cloud fills, hatch", "paper"}
	for i := 0; i < 4; i++ {
		x := 24 + i*190
		r := R{x, 64, 170, 70}
		switch i {
		case 0:
			m.fillRect(r, 0)
		case 1, 2:
			m.tone(i, func() { m.fillRect(r, 0) })
		case 3:
			m.hline(r.X, r.right(), r.Y, 2)
			m.hline(r.X, r.right(), r.bottom()-2, 2)
			m.vline(r.X, r.Y, r.bottom(), 2)
			m.vline(r.right()-2, r.Y, r.bottom(), 2)
		}
		m.T(names[i], fontBold, 17, x, 160, 0)
		for j, line := range strings.SplitAfter(uses[i], ", ") {
			m.tone(toneDark, func() { m.T(strings.TrimRight(line, ", "), fontRegular, 14, x, 182+j*18, 0) })
		}
	}
	m.tone(toneDark, func() { m.hline(24, PW-24, 252, 2) })
	m.T("Black and white: changes often", fontBold, 20, 24, 290, 0)
	m.T("Grey: changes rarely", fontBold, 20, 420, 290, 0)
	left := []string{
		"Fast partial refresh (0.4 s), no flash.",
		"Clock, temperatures, power, light levels,",
		"door and presence states, and anything",
		"whose position follows a value (units, %).",
	}
	right := []string{
		"Any change to a grey pixel costs a 4-grey",
		"full refresh (2.1 s, flashes). Labels,",
		"rules, frames, weather icons, date,",
		"forecast days. The hourly full refresh",
		"absorbs changes that come about that rarely.",
	}
	for i, l := range left {
		m.T(l, fontRegular, 17, 24, 322+i*24, 0)
	}
	for i, l := range right {
		m.tone(toneDark, func() { m.T(l, fontRegular, 17, 420, 322+i*24, 0) })
	}
	m.tone(toneDark, func() { m.vline(400, 270, 450, 2) })
}

func exceptionsLayout() []W {
	return []W{
		{R: cell(0, 0, 4, 1), Draw: clock("07:15", "Tuesday", "17 November", true)},
		{R: cell(4, 0, 4, 2), Draw: weather("rainy", "Rain", 6, 9, 4, 94, forecast[1:])},
		{R: cell(0, 1, 2, 2), Draw: temperature("Living room", "19", ".6°", "52")},
		{R: cell(2, 1, 2, 2), Draw: unavailableW("Outdoor")},
		{R: cell(0, 3, 4, 2), Draw: thermostat("Living room thermostat", "19.6°", "18.0°", "Idle")},
		{R: cell(4, 2, 4, 3), Draw: lights("Lights", []light{{"Living room light", false, 0}, {"Kitchen light", false, 0}, {"Hallway", false, 0}}, true)},
		{R: cell(0, 5, 2, 1), Alert: true, Draw: state("Front door", "Open", iDoor(true))},
		{R: cell(2, 5, 2, 1), Draw: state("Bathroom window", "Closed", iWin(false))},
		{R: cell(4, 5, 2, 1), Draw: state("Alex", "Away", iPerson(false))},
		{R: cell(6, 5, 2, 1), Draw: value("Power", "88", "W", iBolt)},
	}
}

func specimenB() []W {
	return []W{
		{R: cell(0, 0, 2, 1), Draw: weather("partlycloudy", "Partly cloudy", 8, 9, 3, 81, nil)},
		{R: cell(2, 0, 4, 1), Draw: weather("partlycloudy", "Partly cloudy", 8, 9, 3, 81, nil)},
		{R: cell(6, 0, 2, 1), Draw: clock("18:42", "", "", true)},
		{R: cell(0, 1, 2, 2), Draw: weather("rainy", "Rain", 6, 9, 4, 94, nil)},
		{R: cell(2, 1, 4, 2), Draw: weather("partlycloudy", "Partly cloudy", 8, 9, 3, 81, forecast)},
		{R: cell(6, 1, 2, 1), Draw: lights("Lights", demoLights, false)},
		{R: cell(6, 2, 2, 2), Draw: lights("Lights", demoLights, false)},
		{R: cell(0, 3, 1, 1), Draw: state("Front door", "Closed", iDoor(false))},
		{R: cell(1, 3, 1, 1), Alert: true, Draw: state("Window", "Open", iWin(true))},
		{R: cell(2, 3, 2, 1), Draw: state("Front door", "Locked", func(m *M, cx, cy, s float64) { m.lock(cx, cy, s, true) })},
		{R: cell(4, 3, 2, 1), Alert: true, Draw: state("Back door", "Unlocked", func(m *M, cx, cy, s float64) { m.lock(cx, cy, s, false) })},
		{R: cell(0, 4, 4, 1), Draw: clock("18:42", "Monday", "16 November", true)},
		{R: cell(4, 4, 4, 1), Draw: clock("", "Monday", "16 November 2026", false)},
		{R: cell(0, 5, 2, 1), Draw: state("Alex", "Away", iPerson(false))},
		{R: cell(2, 5, 6, 1), Draw: note("Bins go out tonight. Recycling and paper.")},
	}
}
