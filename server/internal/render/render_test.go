package render

import (
	"context"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"homepaper/internal/ha"
	"homepaper/internal/store"
)

var update = flag.Bool("update", false, "rewrite the golden images in docs/design from the renderer")

// Every string the renderer writes itself must be in the bitmap fonts, so
// it never falls back to a thresholded outline at small sizes.
func TestBitmapCoverage(t *testing.T) {
	strs := []string{unavailable, "Not available", "Set to", "All off", "of on", "°", "·", "%", "...",
		"Home", "Away", "Locked", "Unlocked", "Open", "Closed", "Detected", "Clear", "On", "Off",
		"0123456789.,-+ ", "Heating Cooling Idle Off Heat Cool Auto Drying Fan",
		"Mon Tue Wed Thu Fri Sat Sun Monday Tuesday Wednesday Thursday Friday Saturday Sunday",
		"January February March April May June July August September October November December",
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

// goldenNow is the moment the mockups in docs/design show.
var goldenNow = time.Date(2026, 11, 16, 18, 42, 0, 0, time.UTC)

func demoStates(t *testing.T) *ha.Store {
	t.Helper()
	st := ha.NewStore(ha.SourceDemo)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ha.RunDemo(ctx, st) // loads the entities, then returns
	st.SetForecast("weather.home", ha.DemoForecast(goldenNow))
	return st
}

func setState(st *ha.Store, id, state string) {
	e, _ := st.Get(id)
	e.State = state
	st.Set(id, &e)
}

// heroLayout is the main layout of docs/design.md, on demo entities.
func heroLayout(dividers string) store.Layout {
	l := store.ExampleLayout() // a new display starts with the main design
	l.Dividers = dividers
	return l
}

// The renderer draws the approved mockups pixel for pixel.
func TestGolden(t *testing.T) {
	calm := demoStates(t)
	setState(calm, "binary_sensor.bathroom_window", "off")
	cases := []struct {
		file   string
		layout store.Layout
		states *ha.Store
	}{
		{"hero-rules.png", heroLayout(""), demoStates(t)},
		{"hero-frames.png", heroLayout(store.DividersFrames), demoStates(t)},
		{"hero-calm.png", heroLayout(store.DividersRules), calm},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			got := Render(tc.layout, store.DefaultPanel, tc.states, goldenNow).ToGray()
			path := filepath.Join("..", "..", "..", "docs", "design", tc.file)
			if *update {
				writePNG(t, path, got)
				return
			}
			want := readGray(t, path)
			if diff := countDiff(got, want); diff > 0 {
				out := filepath.Join(t.TempDir(), tc.file)
				writePNG(t, out, got)
				t.Fatalf("%d pixels differ from %s; got %s", diff, path, out)
			}
		})
	}
}

func readGray(t *testing.T, path string) *image.Gray {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	g := image.NewGray(img.Bounds())
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			g.Set(x, y, img.At(x, y))
		}
	}
	return g
}

func writePNG(t *testing.T, path string, g *image.Gray) {
	t.Helper()
	pal := color.Palette{color.Black, color.White}
	out := image.NewPaletted(g.Bounds(), pal)
	for y := g.Bounds().Min.Y; y < g.Bounds().Max.Y; y++ {
		for x := g.Bounds().Min.X; x < g.Bounds().Max.X; x++ {
			if g.GrayAt(x, y).Y >= 128 {
				out.SetColorIndex(x, y, 1)
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, out); err != nil {
		t.Fatal(err)
	}
}

func countDiff(a, b *image.Gray) int {
	if a.Bounds() != b.Bounds() {
		return a.Bounds().Dx() * a.Bounds().Dy()
	}
	n := 0
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			if (a.GrayAt(x, y).Y >= 128) != (b.GrayAt(x, y).Y >= 128) {
				n++
			}
		}
	}
	return n
}

// Text never leaves its widget, however long it is.
func TestTextStaysInsideWidget(t *testing.T) {
	long := "A label far too long for a single cell, with more words than fit anywhere"
	l := store.Layout{Cols: 8, Rows: 6, Dividers: store.DividersFrames, Widgets: []store.Widget{
		{ID: "a", Kind: "text", X: 3, Y: 2, W: 1, H: 1, Options: map[string]any{"text": long, "size": "large"}},
		{ID: "b", Kind: "value", X: 5, Y: 2, W: 1, H: 1, Options: map[string]any{"entity": "sensor.power_consumption", "label": long}},
	}}
	g := Render(l, store.DefaultPanel, demoStates(t), goldenNow).ToGray()
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

// Every kind renders at every size from its minimum, with its entities
// present or unavailable, without panicking and without drawing outside its
// cell. The images go to $HOMEPAPER_RENDER_OUT when set, for a look.
func TestEveryKindEverySize(t *testing.T) {
	present := demoStates(t)
	gone := demoStates(t)
	for _, e := range gone.All() {
		setState(gone, e.ID, "unavailable")
	}
	opts := map[string]map[string]any{
		"value":       {"entity": "sensor.power_consumption"},
		"temperature": {"entity": "sensor.living_room_temperature", "humidity_entity": "sensor.living_room_humidity"},
		"climate":     {"entity": "climate.living_room"},
		"weather":     {"entity": "weather.home"},
		"lights":      {"entities": "light.living_room, light.kitchen, light.hallway, light.a, light.b, light.c, light.d"},
		"binary":      {"entity": "binary_sensor.bathroom_window"},
		"clock":       {"format": "datetime"},
		"text":        {"text": "Downstairs and a much longer heading than fits"},
	}
	outDir := os.Getenv("HOMEPAPER_RENDER_OUT")
	for _, k := range Kinds() {
		for _, st := range []struct {
			name   string
			states *ha.Store
		}{{"present", present}, {"gone", gone}} {
			for w := k.MinW; w <= 8; w++ {
				for h := k.MinH; h <= 6; h++ {
					l := store.Layout{Cols: 8, Rows: 6, Widgets: []store.Widget{{ID: "x", Kind: k.Name, X: 0, Y: 0, W: w, H: h, Options: opts[k.Name]}}}
					g := Render(l, store.DefaultPanel, st.states, goldenNow).ToGray()
					cw, ch := w*100, h*80
					for y := 0; y < 480; y++ {
						for x := 0; x < 800; x++ {
							if g.GrayAt(x, y).Y == 0 && (x > cw || y > ch) {
								t.Fatalf("%s %dx%d (%s): ink outside the cell at %d,%d", k.Name, w, h, st.name, x, y)
							}
						}
					}
					if outDir != "" && (w <= 4 && h <= 3) {
						writePNG(t, filepath.Join(outDir, fmt.Sprintf("%s-%s-%dx%d.png", k.Name, st.name, w, h)), g)
					}
				}
			}
		}
	}
}

// Aligned widgets stay in their cells; right alignment ends near the right
// edge. The sheets go to $HOMEPAPER_RENDER_OUT when set, for a look.
func TestAlignment(t *testing.T) {
	st := demoStates(t)
	setState(st, "binary_sensor.bathroom_window", "off")
	for _, align := range []string{"left", "center", "right"} {
		o := func(m map[string]any) map[string]any { m["align"] = align; return m }
		l := store.Layout{Cols: 8, Rows: 6, Widgets: []store.Widget{
			{ID: "c1", Kind: "clock", X: 0, Y: 0, W: 4, H: 1, Options: o(map[string]any{"format": "datetime"})},
			{ID: "c2", Kind: "clock", X: 4, Y: 0, W: 4, H: 1, Options: o(map[string]any{"format": "date"})},
			{ID: "c3", Kind: "clock", X: 0, Y: 1, W: 2, H: 1, Options: o(map[string]any{"format": "time"})},
			{ID: "v1", Kind: "value", X: 2, Y: 1, W: 2, H: 1, Options: o(map[string]any{"entity": "sensor.power_consumption"})},
			{ID: "v2", Kind: "value", X: 4, Y: 1, W: 1, H: 1, Options: o(map[string]any{"entity": "sensor.power_consumption"})},
			{ID: "s1", Kind: "binary", X: 5, Y: 1, W: 3, H: 1, Options: o(map[string]any{"entity": "binary_sensor.front_door"})},
			{ID: "t1", Kind: "temperature", X: 0, Y: 2, W: 3, H: 2, Options: o(map[string]any{"entity": "sensor.living_room_temperature", "humidity_entity": "sensor.living_room_humidity"})},
			{ID: "t2", Kind: "temperature", X: 3, Y: 2, W: 1, H: 1, Options: o(map[string]any{"entity": "sensor.outdoor_temperature"})},
			{ID: "s2", Kind: "binary", X: 3, Y: 3, W: 1, H: 1, Options: o(map[string]any{"entity": "person.alex"})},
			{ID: "x1", Kind: "text", X: 4, Y: 2, W: 4, H: 1, Options: o(map[string]any{"text": "Downstairs", "size": "medium"})},
			{ID: "x2", Kind: "text", X: 4, Y: 3, W: 4, H: 1, Options: o(map[string]any{"text": "Bins go out tonight.", "size": "small"})},
			{ID: "x3", Kind: "text", X: 0, Y: 4, W: 8, H: 1, Options: o(map[string]any{"text": "Good evening", "size": "large"})},
			{ID: "m1", Kind: "value", X: 0, Y: 5, W: 4, H: 1, Options: o(map[string]any{"entity": "sensor.missing"})},
		}}
		g := Render(l, store.DefaultPanel, st, goldenNow).ToGray()
		if dir := os.Getenv("HOMEPAPER_RENDER_OUT"); dir != "" {
			writePNG(t, filepath.Join(dir, "align-"+align+".png"), g)
		}
		// The 8-wide large text: right-aligned ink reaches the right content edge.
		rightmost := 0
		for y := 330; y < 390; y++ {
			for x := 0; x < 800; x++ {
				if g.GrayAt(x, y).Y == 0 && x > rightmost {
					rightmost = x
				}
			}
		}
		if align == "right" && rightmost < 780 {
			t.Errorf("right-aligned text ends at x=%d", rightmost)
		}
		if align == "left" && rightmost > 600 {
			t.Errorf("left-aligned text reaches x=%d", rightmost)
		}
	}
}
