// Package render turns a layout plus current entity states into a panel
// bitmap. Widgets exist only here; the web UI shows the rendered result.
package render

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"sync"
	"time"

	"github.com/fogleman/gg"

	"homepaper/internal/frame"
	"homepaper/internal/ha"
	"homepaper/internal/store"
)

// threshold separates black from white after anti-aliased drawing.
const threshold = 128

var mu sync.Mutex

// Render draws a layout for a panel. It is safe for concurrent callers.
func Render(layout store.Layout, panel store.Panel, states *ha.Store, now time.Time) *frame.Bitmap {
	mu.Lock()
	defer mu.Unlock()

	w, h := panel.Width, panel.Height
	if w <= 0 || h <= 0 {
		w, h = store.DefaultPanel.Width, store.DefaultPanel.Height
	}
	dc := gg.NewContext(w, h)
	dc.SetColor(color.White)
	dc.Clear()

	cols, rows := max(layout.Cols, 1), max(layout.Rows, 1)
	cw, ch := float64(w)/float64(cols), float64(h)/float64(rows)
	frames := layout.Dividers == store.DividersFrames
	type placed struct {
		r     R
		alert bool
	}
	var drawn []placed
	for _, wg := range layout.Widgets {
		k, ok := kinds[wg.Kind]
		if !ok {
			continue
		}
		// Whole-pixel cell edges, so frames and text sit on the grid even
		// when the panel size isn't a multiple of the cell count.
		x0, y0 := int(math.Round(float64(wg.X)*cw)), int(math.Round(float64(wg.Y)*ch))
		x1, y1 := int(math.Round(float64(wg.X+wg.W)*cw)), int(math.Round(float64(wg.Y+wg.H)*ch))
		r := R{x0, y0, x1 - x0, y1 - y0}
		c := &ctx{dc: dc, states: states, now: now, opts: wg.Options,
			clip: image.Rect(x0, y0, x1, y1).Inset(frameGap/2 + 2), ink: color.Gray{0}, ax: alignment(wg.Options)}
		alert := k.alert != nil && k.alert(c)
		if frames {
			drawFrame(dc, r)
		}
		if alert {
			// Inverted: the widget asks for action (D-015).
			dc.SetRGB(0, 0, 0)
			dc.DrawRoundedRectangle(float64(r.X)+4, float64(r.Y)+4, float64(r.W)-8, float64(r.H)-8, 8)
			dc.Fill()
			c.ink = color.Gray{255}
		}
		dc.Push()
		dc.DrawRectangle(float64(r.X), float64(r.Y), float64(r.W), float64(r.H))
		dc.Clip()
		k.render(c, content(r))
		dc.ResetClip()
		dc.Pop()
		drawn = append(drawn, placed{r, alert})
	}
	if !frames {
		// Column rules in the gutters, stopping short of crossings, as in print.
		ink := &ctx{dc: dc, clip: image.Rect(0, 0, w, h), ink: color.Gray{0}}
		for _, p := range drawn {
			if p.alert {
				continue
			}
			if p.r.right() < w {
				ink.vline(p.r.right()-1, p.r.Y+10, p.r.bottom()-10, 2)
			}
			if p.r.bottom() < h {
				ink.hline(p.r.X+10, p.r.right()-10, p.r.bottom()-1, 2)
			}
		}
	}

	gray := image.NewGray(image.Rect(0, 0, w, h))
	draw.Draw(gray, gray.Bounds(), dc.Image(), image.Point{}, draw.Src)
	return frame.FromGray(gray, threshold)
}
