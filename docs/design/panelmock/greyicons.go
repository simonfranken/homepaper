package main

import (
	"math"

	"github.com/fogleman/gg"
)

// Grey variants of the weather icons for the 2bpp mockups. A weather icon
// changes a few times a day at most, so a full 4-grey refresh is cheap
// there. Outlines and small marks stay black so the icon reads at 26 px.

// weatherIcon draws the 1-bit icon, or its grey variant in grey mode.
func (m *M) weatherIcon(cond string, cx, cy, s float64) {
	if !m.grey || m.fg.Y != 0 {
		weatherIcon(m.dc, cond, cx, cy, s)
		return
	}
	switch cond {
	case "sunny":
		m.greySun(cx, cy, s)
	case "clear-night":
		m.tone(toneDark, func() { iconMoon(m.dc, cx, cy, s) })
	case "partlycloudy":
		m.greySun(cx-s*0.14, cy-s*0.14, s*0.7)
		// Knock a gap out of everything behind the cloud.
		for _, dc := range []*gg.Context{m.dc, m.tones[toneDark], m.tones[toneLight]} {
			dc.SetRGB(1, 1, 1)
			dc.DrawCircle(cx+s*0.08, cy+s*0.06, s*0.3)
			dc.Fill()
		}
		m.greyCloud(cx+s*0.08, cy+s*0.1, s*0.8, toneLight)
	case "rainy", "pouring":
		m.greyCloud(cx, cy-s*0.12, s, toneDark)
		m.dc.SetRGB(0, 0, 0)
		m.dc.SetLineWidth(math.Max(2, s*0.05))
		m.dc.SetLineCap(gg.LineCapRound)
		n := 3
		if cond == "pouring" {
			n = 4
		}
		for i := 0; i < n; i++ {
			x := cx - s*0.2 + float64(i)*s*0.4/float64(n-1)
			m.dc.DrawLine(x, cy+s*0.14, x-s*0.06, cy+s*0.32)
		}
		m.dc.Stroke()
	case "snowy", "snowy-rainy", "hail":
		m.greyCloud(cx, cy-s*0.12, s, toneLight)
		m.dc.SetRGB(0, 0, 0)
		for i := 0; i < 3; i++ {
			x := cx - s*0.2 + float64(i)*s*0.2
			m.dc.DrawCircle(x, cy+s*0.24+float64(i%2)*s*0.08, s*0.045)
		}
		m.dc.Fill()
	case "lightning", "lightning-rainy":
		m.greyCloud(cx, cy-s*0.12, s, toneDark)
		m.dc.SetRGB(0, 0, 0)
		m.dc.MoveTo(cx+s*0.04, cy+s*0.06)
		m.dc.LineTo(cx-s*0.12, cy+s*0.26)
		m.dc.LineTo(cx, cy+s*0.26)
		m.dc.LineTo(cx-s*0.08, cy+s*0.44)
		m.dc.LineTo(cx+s*0.14, cy+s*0.2)
		m.dc.LineTo(cx+s*0.02, cy+s*0.2)
		m.dc.ClosePath()
		m.dc.Fill()
	default: // cloudy and the rest
		m.greyCloud(cx, cy, s, toneLight)
	}
}

// greySun: a dark grey disc with black rays.
func (m *M) greySun(cx, cy, s float64) {
	m.tone(toneDark, func() {
		m.dc.SetRGB(0, 0, 0)
		m.dc.DrawCircle(cx, cy, s*0.2)
		m.dc.Fill()
	})
	m.dc.SetRGB(0, 0, 0)
	m.dc.SetLineWidth(math.Max(2, s*0.06))
	m.dc.SetLineCap(gg.LineCapRound)
	for i := 0; i < 8; i++ {
		a := float64(i) * math.Pi / 4
		m.dc.DrawLine(cx+math.Cos(a)*s*0.3, cy+math.Sin(a)*s*0.3, cx+math.Cos(a)*s*0.44, cy+math.Sin(a)*s*0.44)
	}
	m.dc.Stroke()
}

// greyCloud: a black outline filled with the given grey.
func (m *M) greyCloud(cx, cy, s float64, fill int) {
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
	m.dc.SetRGB(0, 0, 0)
	shape(m.dc)
	m.dc.Fill()
	m.dc.SetRGB(1, 1, 1)
	inner(m.dc)
	t := m.tones[fill]
	t.SetRGB(0, 0, 0)
	inner(t)
}
