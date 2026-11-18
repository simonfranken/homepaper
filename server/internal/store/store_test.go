package store

import (
	"image"
	"testing"
)

func TestViewportArea(t *testing.T) {
	p := DefaultPanel
	cases := []struct {
		v    Viewport
		want image.Rectangle
	}{
		{Viewport{}, image.Rect(0, 0, 800, 480)},
		{Viewport{W: 600, H: 432}, image.Rect(100, 24, 700, 456)},
		{Viewport{W: 600, H: 432, OffsetX: -8, OffsetY: 4}, image.Rect(92, 28, 692, 460)},
		{Viewport{W: 600, H: 432, OffsetX: 500}, image.Rect(200, 24, 800, 456)},
		{Viewport{W: 600, H: 432, OffsetX: -500, OffsetY: -500}, image.Rect(0, 0, 600, 432)},
		// Stored for a larger panel: clamped, not reset.
		{Viewport{W: 900, H: 432}, image.Rect(0, 24, 800, 456)},
	}
	for _, c := range cases {
		if got := c.v.Area(p); got != c.want {
			t.Errorf("%+v: area %v, want %v", c.v, got, c.want)
		}
	}
	small := Panel{Model: "x", Width: 640, Height: 384, Format: "1bpp"}
	if got := (Viewport{W: 720, H: 432, OffsetX: 20}).Area(small); got != image.Rect(0, 0, 640, 384) {
		t.Errorf("viewport from a larger panel on a smaller one: %v", got)
	}
}
