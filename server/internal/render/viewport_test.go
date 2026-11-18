package render

import (
	"image"
	"testing"

	"homepaper/internal/store"
)

func TestGrid(t *testing.T) {
	l := store.Layout{Cols: 8, Rows: 6}
	cases := []struct {
		w, h       int
		cols, rows int
	}{
		{800, 480, 8, 6},
		{640, 384, 8, 6},
		{639, 384, 7, 6},
		{639, 383, 7, 5},
		{560, 300, 7, 4},
		{80, 64, 1, 1},
		{40, 30, 1, 1},
	}
	for _, c := range cases {
		cols, rows := Grid(l, image.Rect(0, 0, c.w, c.h))
		if cols != c.cols || rows != c.rows {
			t.Errorf("%d×%d: grid %d×%d, want %d×%d", c.w, c.h, cols, rows, c.cols, c.rows)
		}
	}
	if !Fits(store.Widget{X: 6, W: 1, H: 1}, 7, 6) || Fits(store.Widget{X: 7, W: 1, H: 1}, 7, 6) || Fits(store.Widget{X: 5, W: 3, H: 1}, 7, 6) {
		t.Fatal("Fits is wrong at the edge of the grid")
	}
}

// The default area is the full panel and draws exactly what Render draws, so
// the golden images stay valid.
func TestFullAreaUnchanged(t *testing.T) {
	st := demoStates(t)
	l := greyLayout(heroLayout(""))
	a := Render(l, store.DefaultPanel, st, goldenNow).ToGray()
	b := RenderIn(l, store.DefaultPanel, store.Viewport{W: 800, H: 480}, st, goldenNow).ToGray()
	if n := countDiff(a, b); n != 0 {
		t.Fatalf("full-panel viewport differs in %d pixels", n)
	}
}

// A smaller area draws the same as a panel of that size, placed on a white
// panel at the area's position.
func TestSmallerArea(t *testing.T) {
	st := demoStates(t)
	l := greyLayout(heroLayout(""))
	small := store.DefaultPanel
	small.Width, small.Height = 640, 384
	want := Render(l, small, st, goldenNow).ToGray()

	for _, v := range []store.Viewport{
		{W: 640, H: 384},
		{W: 640, H: 384, OffsetX: -30, OffsetY: 10},
		{W: 640, H: 384, OffsetX: 400, OffsetY: -400},
	} {
		got := RenderIn(l, store.DefaultPanel, v, st, goldenNow).ToGray()
		if got.Bounds() != image.Rect(0, 0, 800, 480) {
			t.Fatalf("%+v: frame is %v, want the full panel", v, got.Bounds())
		}
		area := v.Area(store.DefaultPanel)
		for y := 0; y < 480; y++ {
			for x := 0; x < 800; x++ {
				p := image.Pt(x, y)
				g := got.GrayAt(x, y).Y
				if p.In(area) {
					if w := want.GrayAt(x-area.Min.X, y-area.Min.Y).Y; g != w {
						t.Fatalf("%+v: pixel %v is %d, want %d as on a 640×384 panel", v, p, g, w)
					}
				} else if g != 0xFF {
					t.Fatalf("%+v: margin pixel %v is %d, want white", v, p, g)
				}
			}
		}
	}
}

// At 639 px the grid has 7 columns, and a widget in column 8 isn't drawn.
func TestBrokenWidgetNotDrawn(t *testing.T) {
	st := demoStates(t)
	base := store.Layout{Cols: 8, Rows: 6, Widgets: []store.Widget{
		{ID: "a", Kind: "clock", X: 0, Y: 0, W: 3, H: 1, Options: map[string]any{"format": "time"}},
	}}
	with := base
	with.Widgets = append([]store.Widget{}, base.Widgets...)
	with.Widgets = append(with.Widgets, store.Widget{ID: "b", Kind: "clock", X: 7, Y: 0, W: 1, H: 1, Options: map[string]any{"format": "time"}})
	v := store.Viewport{W: 639}
	area := v.Area(store.DefaultPanel)
	if got := Broken(with, area); len(got) != 1 || got[0] != "b" {
		t.Fatalf("broken = %v, want [b]", got)
	}
	a := RenderIn(base, store.DefaultPanel, v, st, goldenNow).ToGray()
	b := RenderIn(with, store.DefaultPanel, v, st, goldenNow).ToGray()
	if n := countDiff(a, b); n != 0 {
		t.Fatalf("a widget outside the effective grid changed %d pixels", n)
	}
	if got := Broken(with, store.Viewport{}.Area(store.DefaultPanel)); len(got) != 0 {
		t.Fatalf("on the full panel nothing is broken, got %v", got)
	}
}

func TestEdges(t *testing.T) {
	st := demoStates(t)
	l := store.Layout{Cols: 8, Rows: 6}
	v := store.Viewport{W: 600, H: 400, Edges: true}
	g := RenderIn(l, store.DefaultPanel, v, st, goldenNow).ToGray()
	area := v.Area(store.DefaultPanel)
	for _, p := range []image.Point{area.Min, {area.Max.X - 1, area.Min.Y}, {area.Min.X, area.Max.Y - 1}, {area.Max.X - 1, area.Max.Y - 1}, {area.Min.X + 300, area.Min.Y}} {
		if g.GrayAt(p.X, p.Y).Y != 0 {
			t.Errorf("edge pixel %v not black", p)
		}
	}
	if g.GrayAt(area.Min.X-1, area.Min.Y).Y != 0xFF || g.GrayAt(area.Min.X+1, area.Min.Y+1).Y != 0xFF {
		t.Error("edge line is more than 1 px or outside the area")
	}
	v.Edges = false
	if n := countDiff(g, RenderIn(l, store.DefaultPanel, v, st, goldenNow).ToGray()); n != 2*600+2*400-4 {
		t.Errorf("edges changed %d pixels, want only the 1 px outline", n)
	}
}
