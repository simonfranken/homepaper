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

// Minimum cell size (D-021). When the display area is too small for the
// layout's grid at this size, the effective grid drops whole columns or
// rows instead of shrinking cells further.
const MinCellW, MinCellH = 80, 64

// Grid returns the effective grid of a layout drawn into area: the layout's
// columns and rows, reduced to what fits at the minimum cell size. Grid and
// Fits are the only place this rule lives; the API hands the result to the
// editor.
func Grid(layout store.Layout, area image.Rectangle) (cols, rows int) {
	cols = min(max(area.Dx()/MinCellW, 1), max(layout.Cols, 1))
	rows = min(max(area.Dy()/MinCellH, 1), max(layout.Rows, 1))
	return cols, rows
}

// Fits reports whether a widget lies inside an effective grid. Widgets that
// don't fit aren't drawn, but stay stored (D-021).
func Fits(wg store.Widget, cols, rows int) bool {
	return wg.X >= 0 && wg.Y >= 0 && wg.X+wg.W <= cols && wg.Y+wg.H <= rows
}

// Broken returns the IDs of the widgets that don't fit the effective grid.
func Broken(layout store.Layout, area image.Rectangle) []string {
	cols, rows := Grid(layout, area)
	out := []string{}
	for _, wg := range layout.Widgets {
		if !Fits(wg, cols, rows) {
			out = append(out, wg.ID)
		}
	}
	return out
}

// Render draws a layout for the full panel. It is safe for concurrent callers.
func Render(layout store.Layout, panel store.Panel, states *ha.Store, now time.Time) *frame.Bitmap {
	return RenderIn(layout, panel, store.Viewport{}, states, now)
}

// RenderIn draws a layout into the panel's display area (D-021). The layout
// is drawn exactly as on a panel the size of the area, then placed on a
// white full-panel image, so frames always cover the whole panel and the
// margin never shows up in a diff. It is safe for concurrent callers.
func RenderIn(layout store.Layout, panel store.Panel, vp store.Viewport, states *ha.Store, now time.Time) *frame.Bitmap {
	mu.Lock()
	defer mu.Unlock()

	pw, ph := panel.Width, panel.Height
	if pw <= 0 || ph <= 0 {
		pw, ph = store.DefaultPanel.Width, store.DefaultPanel.Height
	}
	area := vp.Area(panel)
	w, h := area.Dx(), area.Dy()
	dc := gg.NewContext(w, h)
	dc.SetColor(color.White)
	dc.Clear()
	depth := frame.DepthOf(panel.Format)
	if depth == 0 {
		depth = 1
	}
	// Grey mode (D-018): grey is drawn on two coverage layers, composed
	// under the black ink at the end.
	var tones *[3]*gg.Context
	if depth == 2 && layout.Greys {
		tones = &[3]*gg.Context{}
		for _, t := range []int{toneDark, toneLight} {
			tones[t] = gg.NewContext(w, h)
			tones[t].SetColor(color.White)
			tones[t].Clear()
		}
	}

	cols, rows := Grid(layout, area)
	cw, ch := float64(w)/float64(cols), float64(h)/float64(rows)
	frames := layout.Dividers == store.DividersFrames
	type placed struct {
		r     R
		alert bool
	}
	var drawn []placed
	for _, wg := range layout.Widgets {
		k, ok := kinds[wg.Kind]
		if !ok || !Fits(wg, cols, rows) {
			continue
		}
		// Whole-pixel cell edges, so frames and text sit on the grid even
		// when the area size isn't a multiple of the cell count.
		x0, y0 := int(math.Round(float64(wg.X)*cw)), int(math.Round(float64(wg.Y)*ch))
		x1, y1 := int(math.Round(float64(wg.X+wg.W)*cw)), int(math.Round(float64(wg.Y+wg.H)*ch))
		r := R{x0, y0, x1 - x0, y1 - y0}
		c := &ctx{dc: dc, states: states, now: now, opts: wg.Options,
			clip: image.Rect(x0, y0, x1, y1).Inset(frameGap/2 + 2), ink: color.Gray{0}, ax: alignment(wg.Options), tones: tones}
		alert := k.alert != nil && k.alert(c)
		if frames {
			if tones != nil {
				drawFrame(tones[toneDark], r)
			} else {
				drawFrame(dc, r)
			}
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
		ink := &ctx{dc: dc, clip: image.Rect(0, 0, w, h), ink: color.Gray{0}, tones: tones}
		ink.tone(toneDark, func() {
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
		})
	}

	// Place the area on a white full-panel image.
	onPanel := func(src image.Image) *image.Gray {
		g := image.NewGray(image.Rect(0, 0, pw, ph))
		for i := range g.Pix {
			g.Pix[i] = 0xFF
		}
		draw.Draw(g, area, src, image.Point{}, draw.Src)
		return g
	}
	gray := onPanel(dc.Image())
	if vp.Edges {
		edges(gray, area)
	}
	var layers [3]*image.Gray
	if tones != nil {
		for _, t := range []int{toneDark, toneLight} {
			layers[t] = onPanel(tones[t].Image())
		}
	}
	if depth == 1 {
		return frame.FromGray(gray, threshold)
	}
	return frame.FromLevels(compose(gray, layers))
}

// edges draws a 1 px black line on the area's outermost pixels, for lining
// the area up with the frame's opening.
func edges(g *image.Gray, r image.Rectangle) {
	for x := r.Min.X; x < r.Max.X; x++ {
		g.SetGray(x, r.Min.Y, color.Gray{0})
		g.SetGray(x, r.Max.Y-1, color.Gray{0})
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		g.SetGray(r.Min.X, y, color.Gray{0})
		g.SetGray(r.Max.X-1, y, color.Gray{0})
	}
}

// compose turns the ink canvas and the grey layers into exact levels: black
// ink wins, then dark grey, then light grey, else white. Each layer is
// thresholded on its own, so anti-aliased edges never turn grey.
func compose(ink *image.Gray, layers [3]*image.Gray) *image.Gray {
	out := image.NewGray(ink.Bounds())
	for i, v := range ink.Pix {
		switch {
		case v < threshold:
			out.Pix[i] = 0
		case layers[toneDark] != nil && layers[toneDark].Pix[i] < threshold:
			out.Pix[i] = 0x55
		case layers[toneLight] != nil && layers[toneLight].Pix[i] < threshold:
			out.Pix[i] = 0xAA
		default:
			out.Pix[i] = 0xFF
		}
	}
	return out
}
