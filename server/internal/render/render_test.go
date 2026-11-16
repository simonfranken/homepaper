package render

import (
	"context"
	"testing"
	"time"

	"homepaper/internal/ha"
	"homepaper/internal/store"
)

// Every string the renderer writes itself must be in the bitmap fonts, so
// it never falls back to a thresholded outline at small sizes. ("→" is the
// known exception: no Lucida bitmap has it, it comes from the Go font.)
func TestBitmapCoverage(t *testing.T) {
	strs := []string{unavailable, " % humidity", "on", "off", "°", "·", "%", "...",
		"Home", "Away", "Locked", "Unlocked", "Open", "Closed", "Detected", "Clear", "On", "Off",
		"0123456789.,-+ ", "Heating Cooling Idle Off Heat Cool Auto",
		"ÄÖÜäöüß ÀÉÈÊÇàéèêçñ €"}
	for _, l := range weatherLabels {
		strs = append(strs, l)
	}
	for weight, sizes := range bitmaps {
		for px, f := range sizes {
			for _, s := range strs {
				if missing := f.covers(s); len(missing) > 0 {
					t.Errorf("%s %d px lacks %q (in %q)", weight, px, string(missing), s)
				}
			}
		}
	}
	if len(bitmapSizes) != 6 {
		t.Errorf("bitmap sizes %v, want 6", bitmapSizes)
	}
}

func demoStates(t *testing.T) *ha.Store {
	t.Helper()
	st := ha.NewStore(ha.SourceDemo)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ha.RunDemo(ctx, st) // loads the entities, then returns
	return st
}

// Text never leaves its widget, however long it is.
func TestTextStaysInsideWidget(t *testing.T) {
	long := "A label far too long for a single cell, with more words than fit anywhere"
	l := store.Layout{Cols: 8, Rows: 6, Widgets: []store.Widget{
		{ID: "a", Kind: "text", X: 3, Y: 2, W: 1, H: 1, Options: map[string]any{"text": long, "size": "large", "border": false}},
		{ID: "b", Kind: "value", X: 5, Y: 2, W: 1, H: 1, Options: map[string]any{"entity": "sensor.power_consumption", "label": long, "border": false}},
	}}
	g := Render(l, store.DefaultPanel, demoStates(t), time.Now()).ToGray()
	inside := func(x, y int) bool {
		return (x >= 300 && x < 400 || x >= 500 && x < 600) && y >= 160 && y < 240
	}
	ink := 0
	b := g.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if g.GrayAt(x, y).Y == 0 {
				if !inside(x, y) {
					t.Fatalf("black pixel outside the widgets at %d,%d", x, y)
				}
				ink++
			}
		}
	}
	if ink == 0 {
		t.Fatal("nothing drawn")
	}
}

// Small text lands on a size step that has a bitmap font.
func TestFitSizeUsesSteps(t *testing.T) {
	px := fitSize("Closed", fontBold, 40, 60, 40)
	found := false
	for _, s := range sizeSteps(40) {
		found = found || s == px
	}
	if !found {
		t.Fatalf("fitSize returned %d, not a size step", px)
	}
	if px > 25 {
		t.Fatalf("\"Closed\" in 60 px chose %d px", px)
	}
	if _, ok := face(fontBold, px).(*bitmapFace); !ok {
		t.Fatalf("%d px is not a bitmap face", px)
	}
}

