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

// iconDot is a status dot: filled for active, ring for inactive.
func iconDot(dc *gg.Context, cx, cy, s float64, active bool) {
	dc.SetRGB(0, 0, 0)
	dc.SetLineWidth(math.Max(2, s*0.1))
	dc.DrawCircle(cx, cy, s*0.32)
	if active {
		dc.Fill()
	} else {
		dc.Stroke()
	}
}

func iconThermometer(dc *gg.Context, cx, cy, s float64) {
	dc.SetRGB(0, 0, 0)
	dc.SetLineWidth(math.Max(2, s*0.07))
	dc.DrawRoundedRectangle(cx-s*0.08, cy-s*0.4, s*0.16, s*0.55, s*0.08)
	dc.Stroke()
	dc.DrawCircle(cx, cy+s*0.26, s*0.16)
	dc.Fill()
	dc.DrawRectangle(cx-s*0.03, cy-s*0.1, s*0.06, s*0.3)
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
