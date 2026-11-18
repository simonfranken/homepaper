package render

import (
	"image"
	"math"

	"github.com/fogleman/gg"
)

// Grey mode drawing (D-018, docs/design.md "Panel in 4 greys"). The rule:
// grey only for pixels that don't change with entity state or time, because
// a changed grey pixel costs a 4-grey full refresh. Values, units, state
// words and state icons stay black.

// toneIf is tone when ok, else plain ink drawing.
func (c *ctx) toneIf(ok bool, level int, f func()) {
	if ok {
		c.tone(level, f)
	} else {
		f()
	}
}

// hatch fills r with light grey diagonal lines, leaving keep white, as the
// ground of a widget whose entity is unavailable.
func (c *ctx) hatch(r, keep R) {
	c.tone(toneLight, func() {
		c.dc.Push()
		c.dc.DrawRectangle(float64(r.X), float64(r.Y), float64(r.W), float64(r.H))
		c.dc.Clip()
		c.dc.SetRGB(0, 0, 0)
		c.dc.SetLineWidth(2)
		for x := r.X - r.H; x < r.right(); x += 10 {
			c.dc.DrawLine(float64(x), float64(r.bottom()), float64(x+r.H), float64(r.Y))
		}
		c.dc.Stroke()
		c.dc.ResetClip()
		c.dc.Pop()
		k := image.Rect(keep.X, keep.Y, keep.right(), keep.bottom()).Intersect(c.clip)
		c.dc.SetRGB(1, 1, 1)
		c.dc.DrawRectangle(float64(k.Min.X), float64(k.Min.Y), float64(k.Dx()), float64(k.Dy()))
		c.dc.Fill()
	})
}

// weatherIcon draws the icon for an HA weather condition: the 1-bit icon,
// or in grey mode a variant with grey bodies and black outlines. Weather
// icons change a few times a day at most.
func (c *ctx) weatherIcon(cond string, cx, cy, s float64) {
	if !c.greys() {
		weatherIcon(c.dc, cond, cx, cy, s)
		return
	}
	dc := c.dc
	switch cond {
	case "sunny":
		c.greySun(cx, cy, s)
	case "clear-night":
		c.tone(toneDark, func() { iconMoon(c.dc, cx, cy, s) })
	case "partlycloudy":
		c.greySun(cx-s*0.14, cy-s*0.14, s*0.7)
		// Knock a gap out of everything behind the cloud.
		for _, l := range []*gg.Context{dc, c.tones[toneDark], c.tones[toneLight]} {
			l.SetRGB(1, 1, 1)
			l.DrawCircle(cx+s*0.08, cy+s*0.06, s*0.3)
			l.Fill()
		}
		c.greyCloud(cx+s*0.08, cy+s*0.1, s*0.8, toneLight)
	case "rainy", "pouring":
		c.greyCloud(cx, cy-s*0.12, s, toneDark)
		dc.SetRGB(0, 0, 0)
		dc.SetLineWidth(math.Max(2, s*0.05))
		dc.SetLineCap(gg.LineCapRound)
		n := 3
		if cond == "pouring" {
			n = 4
		}
		for i := 0; i < n; i++ {
			x := cx - s*0.2 + float64(i)*s*0.4/float64(n-1)
			dc.DrawLine(x, cy+s*0.14, x-s*0.06, cy+s*0.32)
		}
		dc.Stroke()
	case "snowy", "snowy-rainy", "hail":
		c.greyCloud(cx, cy-s*0.12, s, toneLight)
		dc.SetRGB(0, 0, 0)
		for i := 0; i < 3; i++ {
			x := cx - s*0.2 + float64(i)*s*0.2
			dc.DrawCircle(x, cy+s*0.24+float64(i%2)*s*0.08, s*0.045)
		}
		dc.Fill()
	case "lightning", "lightning-rainy":
		c.greyCloud(cx, cy-s*0.12, s, toneDark)
		dc.SetRGB(0, 0, 0)
		dc.MoveTo(cx+s*0.04, cy+s*0.06)
		dc.LineTo(cx-s*0.12, cy+s*0.26)
		dc.LineTo(cx, cy+s*0.26)
		dc.LineTo(cx-s*0.08, cy+s*0.44)
		dc.LineTo(cx+s*0.14, cy+s*0.2)
		dc.LineTo(cx+s*0.02, cy+s*0.2)
		dc.ClosePath()
		dc.Fill()
	case "fog", "windy":
		iconFog(dc, cx, cy, s)
	default: // cloudy and the rest
		c.greyCloud(cx, cy, s, toneLight)
	}
}

// greySun is a dark grey disc with black rays.
func (c *ctx) greySun(cx, cy, s float64) {
	c.tone(toneDark, func() {
		c.dc.SetRGB(0, 0, 0)
		c.dc.DrawCircle(cx, cy, s*0.2)
		c.dc.Fill()
	})
	dc := c.dc
	dc.SetRGB(0, 0, 0)
	dc.SetLineWidth(math.Max(2, s*0.06))
	dc.SetLineCap(gg.LineCapRound)
	for i := 0; i < 8; i++ {
		a := float64(i) * math.Pi / 4
		dc.DrawLine(cx+math.Cos(a)*s*0.3, cy+math.Sin(a)*s*0.3, cx+math.Cos(a)*s*0.44, cy+math.Sin(a)*s*0.44)
	}
	dc.Stroke()
}

// greyCloud is a black cloud outline filled with the given grey.
func (c *ctx) greyCloud(cx, cy, s float64, fill int) {
	lw := math.Max(2, s*0.06)
	shape := func(dc *gg.Context) {
		dc.NewSubPath()
		dc.DrawCircle(cx-s*0.16, cy+s*0.02, s*0.16)
		dc.DrawCircle(cx+s*0.06, cy-s*0.08, s*0.21)
		dc.DrawCircle(cx+s*0.24, cy+s*0.05, s*0.13)
		dc.DrawRoundedRectangle(cx-s*0.32, cy+s*0.02, s*0.69, s*0.16, s*0.08)
	}
	inner := func(dc *gg.Context) {
		dc.Push()
		dc.Translate(cx, cy+s*0.03)
		dc.Scale(1-2*lw/(s*0.7), 1-2*lw/(s*0.4))
		dc.Translate(-cx, -(cy + s*0.03))
		shape(dc)
		dc.Fill()
		dc.Pop()
	}
	dc := c.dc
	dc.SetRGB(0, 0, 0)
	shape(dc)
	dc.Fill()
	dc.SetRGB(1, 1, 1)
	inner(dc)
	c.tone(fill, func() {
		c.dc.SetRGB(0, 0, 0)
		inner(c.dc)
	})
}
