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
	for _, wg := range layout.Widgets {
		k, ok := kinds[wg.Kind]
		if !ok {
			continue
		}
		// Whole-pixel cell edges, so frames and text sit on the grid even
		// when the panel size isn't a multiple of the cell count.
		x0, y0 := math.Round(float64(wg.X)*cw), math.Round(float64(wg.Y)*ch)
		x1, y1 := math.Round(float64(wg.X+wg.W)*cw), math.Round(float64(wg.Y+wg.H)*ch)
		r := rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
		clip := image.Rect(int(x0), int(y0), int(x1), int(y1)).Inset(frameGap/2 + 2)
		c := &ctx{dc: dc, states: states, now: now, opts: wg.Options, clip: clip}
		if c.boolOpt("border", true) {
			drawFrame(dc, r)
		}
		dc.Push()
		dc.DrawRectangle(r.X, r.Y, r.W, r.H)
		dc.Clip()
		k.render(c, r.inset(framePad))
		dc.ResetClip()
		dc.Pop()
	}

	gray := image.NewGray(image.Rect(0, 0, w, h))
	draw.Draw(gray, gray.Bounds(), dc.Image(), image.Point{}, draw.Src)
	return frame.FromGray(gray, threshold)
}
