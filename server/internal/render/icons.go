package render

import (
	"math"

	"github.com/fogleman/gg"
)

// Icons are drawn as solid shapes and thick strokes so they survive the
// 1-bit threshold. Each draws centred in a square of side s at (cx, cy).

func iconSun(dc *gg.Context, cx, cy, s float64) {
	dc.SetRGB(0, 0, 0)
	dc.DrawCircle(cx, cy, s*0.2)
	dc.Fill()
	dc.SetLineWidth(math.Max(2, s*0.06))
	dc.SetLineCap(gg.LineCapRound)
	for i := 0; i < 8; i++ {
		a := float64(i) * math.Pi / 4
		dc.DrawLine(cx+math.Cos(a)*s*0.3, cy+math.Sin(a)*s*0.3, cx+math.Cos(a)*s*0.44, cy+math.Sin(a)*s*0.44)
	}
	dc.Stroke()
}

func iconMoon(dc *gg.Context, cx, cy, s float64) {
	dc.SetRGB(0, 0, 0)
	dc.DrawCircle(cx, cy, s*0.32)
	dc.Fill()
	dc.SetRGB(1, 1, 1)
	dc.DrawCircle(cx+s*0.16, cy-s*0.12, s*0.28)
	dc.Fill()
}

// cloud draws a cloud outline (filled when solid) in the upper part of the box.
func iconCloud(dc *gg.Context, cx, cy, s float64, solid bool) {
	lw := math.Max(2, s*0.06)
	shape := func() {
		dc.NewSubPath()
		dc.DrawCircle(cx-s*0.16, cy+s*0.02, s*0.16)
		dc.DrawCircle(cx+s*0.06, cy-s*0.08, s*0.21)
		dc.DrawCircle(cx+s*0.24, cy+s*0.05, s*0.13)
		dc.DrawRoundedRectangle(cx-s*0.32, cy+s*0.02, s*0.69, s*0.16, s*0.08)
	}
	dc.SetRGB(0, 0, 0)
	shape()
	dc.Fill()
	if !solid {
		// Hollow it out by redrawing slightly smaller in white.
		dc.Push()
		dc.Translate(cx, cy+s*0.03)
		dc.Scale(1-2*lw/(s*0.7), 1-2*lw/(s*0.4))
		dc.Translate(-cx, -(cy + s*0.03))
		dc.SetRGB(1, 1, 1)
		shape()
		dc.Fill()
		dc.Pop()
	}
}

func iconRain(dc *gg.Context, cx, cy, s float64, heavy bool) {
	iconCloud(dc, cx, cy-s*0.12, s, true)
	dc.SetRGB(0, 0, 0)
	dc.SetLineWidth(math.Max(2, s*0.05))
	dc.SetLineCap(gg.LineCapRound)
	n := 3
	if heavy {
		n = 4
	}
	for i := 0; i < n; i++ {
		x := cx - s*0.2 + float64(i)*s*0.4/float64(n-1)
		dc.DrawLine(x, cy+s*0.14, x-s*0.06, cy+s*0.32)
	}
	dc.Stroke()
}

func iconSnow(dc *gg.Context, cx, cy, s float64) {
	iconCloud(dc, cx, cy-s*0.12, s, true)
	dc.SetRGB(0, 0, 0)
	for i := 0; i < 3; i++ {
		x := cx - s*0.2 + float64(i)*s*0.2
		dc.DrawCircle(x, cy+s*0.24+float64(i%2)*s*0.08, s*0.045)
	}
	dc.Fill()
}

func iconLightning(dc *gg.Context, cx, cy, s float64) {
	iconCloud(dc, cx, cy-s*0.12, s, true)
	dc.SetRGB(0, 0, 0)
	dc.MoveTo(cx+s*0.04, cy+s*0.06)
	dc.LineTo(cx-s*0.12, cy+s*0.26)
	dc.LineTo(cx, cy+s*0.26)
	dc.LineTo(cx-s*0.08, cy+s*0.44)
	dc.LineTo(cx+s*0.14, cy+s*0.2)
	dc.LineTo(cx+s*0.02, cy+s*0.2)
	dc.ClosePath()
	dc.Fill()
}

func iconFog(dc *gg.Context, cx, cy, s float64) {
	dc.SetRGB(0, 0, 0)
	dc.SetLineWidth(math.Max(2, s*0.06))
	dc.SetLineCap(gg.LineCapRound)
	for i := 0; i < 4; i++ {
		y := cy - s*0.24 + float64(i)*s*0.16
		off := float64(i%2) * s * 0.08
		dc.DrawLine(cx-s*0.34+off, y, cx+s*0.34-off, y)
	}
	dc.Stroke()
}

func iconPartlyCloudy(dc *gg.Context, cx, cy, s float64) {
	iconSun(dc, cx-s*0.14, cy-s*0.14, s*0.7)
	dc.SetRGB(1, 1, 1)
	dc.DrawCircle(cx+s*0.08, cy+s*0.06, s*0.3)
	dc.Fill()
	iconCloud(dc, cx+s*0.08, cy+s*0.1, s*0.8, false)
}

// iconBulb is a light bulb, filled when on.
func iconBulb(dc *gg.Context, cx, cy, s float64, on bool) {
	dc.SetRGB(0, 0, 0)
	lw := math.Max(2, s*0.08)
	dc.SetLineWidth(lw)
	dc.DrawCircle(cx, cy-s*0.08, s*0.26)
	if on {
		dc.Fill()
	} else {
		dc.Stroke()
	}
	dc.DrawRectangle(cx-s*0.12, cy+s*0.2, s*0.24, s*0.18)
	dc.Fill()
}

// weatherIcon draws the icon for an HA weather condition.
func weatherIcon(dc *gg.Context, condition string, cx, cy, s float64) {
	switch condition {
	case "sunny":
		iconSun(dc, cx, cy, s)
	case "clear-night":
		iconMoon(dc, cx, cy, s)
	case "partlycloudy":
		iconPartlyCloudy(dc, cx, cy, s)
	case "cloudy", "windy-variant":
		iconCloud(dc, cx, cy, s, false)
	case "rainy":
		iconRain(dc, cx, cy, s, false)
	case "pouring":
		iconRain(dc, cx, cy, s, true)
	case "snowy", "snowy-rainy", "hail":
		iconSnow(dc, cx, cy, s)
	case "lightning", "lightning-rainy":
		iconLightning(dc, cx, cy, s)
	case "fog", "windy":
		iconFog(dc, cx, cy, s)
	default:
		iconCloud(dc, cx, cy, s, false)
	}
}

var weatherLabels = map[string]string{
	"clear-night":     "Clear night",
	"cloudy":          "Cloudy",
	"exceptional":     "Exceptional",
	"fog":             "Fog",
	"hail":            "Hail",
	"lightning":       "Thunderstorm",
	"lightning-rainy": "Thunder & rain",
	"partlycloudy":    "Partly cloudy",
	"pouring":         "Pouring",
	"rainy":           "Rain",
	"snowy":           "Snow",
	"snowy-rainy":     "Sleet",
	"sunny":           "Sunny",
	"windy":           "Windy",
	"windy-variant":   "Windy, cloudy",
}

// State icons draw the state itself, so it reads before the word does.
// They use the widget's ink, because their widgets may be inverted.

func (c *ctx) droplet(cx, cy, s float64) {
	c.setInk()
	dc := c.dc
	dc.MoveTo(cx, cy-s*0.5)
	dc.CubicTo(cx+s*0.1, cy-s*0.25, cx+s*0.36, cy-s*0.02, cx+s*0.36, cy+s*0.16)
	dc.CubicTo(cx+s*0.36, cy+s*0.38, cx+s*0.18, cy+s*0.5, cx, cy+s*0.5)
	dc.CubicTo(cx-s*0.18, cy+s*0.5, cx-s*0.36, cy+s*0.38, cx-s*0.36, cy+s*0.16)
	dc.CubicTo(cx-s*0.36, cy-s*0.02, cx-s*0.1, cy-s*0.25, cx, cy-s*0.5)
	dc.Fill()
}

func (c *ctx) flame(cx, cy, s float64) {
	c.setInk()
	dc := c.dc
	dc.MoveTo(cx, cy-s*0.5)
	dc.CubicTo(cx+s*0.42, cy-s*0.1, cx+s*0.4, cy+s*0.2, cx+s*0.32, cy+s*0.32)
	dc.CubicTo(cx+s*0.22, cy+s*0.48, cx-s*0.22, cy+s*0.48, cx-s*0.32, cy+s*0.32)
	dc.CubicTo(cx-s*0.42, cy+s*0.12, cx-s*0.24, cy-s*0.06, cx-s*0.12, cy-s*0.18)
	dc.CubicTo(cx-s*0.1, cy-s*0.02, cx-s*0.02, cy+s*0.02, cx+s*0.02, cy-s*0.06)
	dc.CubicTo(cx+s*0.08, cy-s*0.2, cx+s*0.04, cy-s*0.36, cx, cy-s*0.5)
	dc.Fill()
}

// door is a closed door with a knob, or an open one swung inwards.
func (c *ctx) door(cx, cy, s float64, open bool) {
	c.setInk()
	dc := c.dc
	dc.SetLineWidth(math.Max(2.5, s*0.09))
	w, h := s*0.5, s*0.86
	x0, y0 := cx-w/2, cy-h/2
	if !open {
		dc.DrawRectangle(x0, y0, w, h)
		dc.Stroke()
		dc.DrawCircle(x0+w*0.75, cy+s*0.04, s*0.06)
		dc.Fill()
		return
	}
	dc.DrawLine(x0, y0, x0+w, y0)
	dc.DrawLine(x0+w, y0, x0+w, y0+h)
	dc.DrawLine(x0-s*0.06, y0+h, x0+w+s*0.06, y0+h)
	dc.Stroke()
	dc.MoveTo(x0, y0)
	dc.LineTo(x0+w*0.55, y0+h*0.12)
	dc.LineTo(x0+w*0.55, y0+h*1.0)
	dc.LineTo(x0, y0+h)
	dc.ClosePath()
	dc.Fill()
}

// window is a four-pane window, or one with its sash tilted open.
func (c *ctx) window(cx, cy, s float64, open bool) {
	c.setInk()
	dc := c.dc
	lw := math.Max(2.5, s*0.08)
	dc.SetLineWidth(lw)
	w, h := s*0.74, s*0.74
	x0, y0 := cx-w/2, cy-h/2
	dc.DrawRectangle(x0, y0, w, h)
	if !open {
		dc.DrawLine(cx, y0, cx, y0+h)
		dc.DrawLine(x0, cy, x0+w, cy)
		dc.Stroke()
		return
	}
	dc.Stroke()
	dc.MoveTo(x0+lw, y0+h-lw)
	dc.LineTo(x0+w-lw, y0+h-lw)
	dc.LineTo(x0+w*0.82, y0+h*0.18)
	dc.LineTo(x0+w*0.18, y0+h*0.18)
	dc.ClosePath()
	dc.Fill()
}

// person is solid when home, hollow when away.
func (c *ctx) person(cx, cy, s float64, home bool) {
	c.setInk()
	dc := c.dc
	dc.SetLineWidth(math.Max(2.5, s*0.09))
	paint := dc.Stroke
	if home {
		paint = dc.Fill
	}
	dc.DrawCircle(cx, cy-s*0.2, s*0.17)
	paint()
	dc.DrawArc(cx, cy+s*0.42, s*0.32, math.Pi, 2*math.Pi)
	dc.ClosePath()
	paint()
}

func (c *ctx) bolt(cx, cy, s float64) {
	c.setInk()
	dc := c.dc
	dc.MoveTo(cx+s*0.08, cy-s*0.5)
	dc.LineTo(cx-s*0.28, cy+s*0.06)
	dc.LineTo(cx-s*0.02, cy+s*0.06)
	dc.LineTo(cx-s*0.1, cy+s*0.5)
	dc.LineTo(cx+s*0.28, cy-s*0.08)
	dc.LineTo(cx+s*0.02, cy-s*0.08)
	dc.ClosePath()
	dc.Fill()
}

// lock has its shackle closed, or lifted and swung aside when unlocked.
func (c *ctx) lock(cx, cy, s float64, locked bool) {
	c.setInk()
	dc := c.dc
	dc.SetLineWidth(math.Max(2.5, s*0.1))
	if locked {
		dc.DrawArc(cx, cy-s*0.08, s*0.2, math.Pi, 2*math.Pi)
	} else {
		dc.DrawArc(cx+s*0.26, cy-s*0.16, s*0.2, math.Pi, 2*math.Pi)
	}
	dc.Stroke()
	dc.DrawRoundedRectangle(cx-s*0.3, cy-s*0.08, s*0.6, s*0.48, s*0.06)
	dc.Fill()
}

// dot is the generic state icon: filled when active, a ring otherwise.
func (c *ctx) dot(cx, cy, s float64, active bool) {
	c.setInk()
	c.dc.SetLineWidth(math.Max(2.5, s*0.1))
	c.dc.DrawCircle(cx, cy, s*0.3)
	if active {
		c.dc.Fill()
	} else {
		c.dc.Stroke()
	}
}

type shieldFill int

const (
	shieldHollow shieldFill = iota
	shieldHalf
	shieldSolid
)

// shield is the alarm icon. Its fill tells the state, like the status dots.
func (c *ctx) shield(cx, cy, s float64, fill shieldFill) {
	c.setInk()
	dc := c.dc
	path := func() {
		dc.MoveTo(cx, cy-s*0.45)
		dc.LineTo(cx+s*0.34, cy-s*0.3)
		dc.LineTo(cx+s*0.34, cy+s*0.02)
		dc.CubicTo(cx+s*0.34, cy+s*0.26, cx+s*0.16, cy+s*0.4, cx, cy+s*0.48)
		dc.CubicTo(cx-s*0.16, cy+s*0.4, cx-s*0.34, cy+s*0.26, cx-s*0.34, cy+s*0.02)
		dc.LineTo(cx-s*0.34, cy-s*0.3)
		dc.ClosePath()
	}
	if fill == shieldSolid {
		path()
		dc.Fill()
		return
	}
	if fill == shieldHalf {
		// Push/Pop keep the widget's clip; Clip narrows it to the left half.
		dc.Push()
		dc.DrawRectangle(cx-s, cy-s, s, 2*s)
		dc.Clip()
		path()
		dc.Fill()
		dc.Pop()
	}
	dc.SetLineWidth(math.Max(2.5, s*0.09))
	path()
	dc.Stroke()
}
