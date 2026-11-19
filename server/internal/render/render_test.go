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
	"strings"
	"testing"
	"time"

	"homepaper/internal/frame"
	"homepaper/internal/ha"
	"homepaper/internal/store"
)

var update = flag.Bool("update", false, "rewrite the golden images in docs/design from the renderer")

// Every string the renderer writes itself must be in the bitmap fonts, so
// it never falls back to a thresholded outline at small sizes.
func TestBitmapCoverage(t *testing.T) {
	strs := []string{unavailable, "Not available", "Set to", "All off", "of on", "°", "·", "%", "...",
		"Home", "Away", "Locked", "Unlocked", "Open", "Closed", "Detected", "Clear", "On", "Off",
		"Cancelled · No departures as of",
		"Disarmed Armed home away night vacation custom Arming Disarming Pending Triggered Off Home Away Night Vacation Custom Disarm Alarm",
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

func greyLayout(l store.Layout) store.Layout {
	l.Greys = true
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
		{"grey-hero-rules.png", greyLayout(heroLayout("")), demoStates(t)},
		{"grey-hero-frames.png", greyLayout(heroLayout(store.DividersFrames)), demoStates(t)},
		{"grey-hero-calm.png", greyLayout(heroLayout(store.DividersRules)), calm},
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
			for _, greys := range []bool{false, true} {
				for w := k.MinW; w <= 8; w++ {
					for h := k.MinH; h <= 6; h++ {
						l := store.Layout{Cols: 8, Rows: 6, Greys: greys, Widgets: []store.Widget{{ID: "x", Kind: k.Name, X: 0, Y: 0, W: w, H: h, Options: opts[k.Name]}}}
						g := Render(l, store.DefaultPanel, st.states, goldenNow).ToGray()
						cw, ch := w*100, h*80
						for y := 0; y < 480; y++ {
							for x := 0; x < 800; x++ {
								if g.GrayAt(x, y).Y != 0xFF && (x > cw || y > ch) {
									t.Fatalf("%s %dx%d (%s, greys %v): ink outside the cell at %d,%d", k.Name, w, h, st.name, greys, x, y)
								}
							}
						}
						if outDir != "" && (w <= 4 && h <= 3) {
							name := fmt.Sprintf("%s-%s-%dx%d.png", k.Name, st.name, w, h)
							if greys {
								name = "grey-" + name
							}
							writePNG(t, filepath.Join(outDir, name), g)
						}
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

// Every alarm_control_panel state has its own word and shield, and none is
// drawn inverted (only doors, windows, garages and locks are, D-015).
func TestAlarmStates(t *testing.T) {
	st := demoStates(t)
	states := []string{"disarmed", "armed_home", "armed_away", "armed_night", "armed_vacation",
		"armed_custom_bypass", "arming", "disarming", "pending", "triggered"}
	var ws []store.Widget
	for i, state := range states {
		id := "alarm_control_panel.a" + state
		st.Set(id, &ha.Entity{ID: id, State: state, Attributes: map[string]any{"friendly_name": "Alarm"}})
		ws = append(ws, store.Widget{ID: state, Kind: "binary", X: (i % 4) * 2, Y: i / 4, W: 2, H: 1, Options: map[string]any{"entity": id}})
		ws = append(ws, store.Widget{ID: state + "s", Kind: "binary", X: i % 8, Y: 3 + i/8, W: 1, H: 1, Options: map[string]any{"entity": id}})
		if word, _ := binaryText("alarm_control_panel", "", state); word == "Off" || word == "On" {
			t.Errorf("%s reads %q", state, word)
		}
		if alertState("alarm_control_panel", "", state) {
			t.Errorf("%s is drawn inverted", state)
		}
	}
	g := Render(store.Layout{Cols: 8, Rows: 6, Widgets: ws}, store.DefaultPanel, st, goldenNow).ToGray()
	if dir := os.Getenv("HOMEPAPER_RENDER_OUT"); dir != "" {
		writePNG(t, filepath.Join(dir, "alarm.png"), g)
	}
}

// A State widget with several entities lists them; rows that ask for
// action are inverted, the widget itself is not.
func TestStateList(t *testing.T) {
	st := demoStates(t) // bathroom window open, front door closed
	list := "binary_sensor.front_door, binary_sensor.bathroom_window, person.alex, alarm_control_panel.home, lock.missing"
	ws := []store.Widget{
		{ID: "a", Kind: "binary", X: 0, Y: 0, W: 3, H: 3, Options: map[string]any{"entity": list, "label": "House"}},
		{ID: "b", Kind: "binary", X: 3, Y: 0, W: 2, H: 2, Options: map[string]any{"entity": list}},
		{ID: "c", Kind: "binary", X: 5, Y: 0, W: 3, H: 1, Options: map[string]any{"entity": list}},
		{ID: "d", Kind: "binary", X: 5, Y: 1, W: 1, H: 2, Options: map[string]any{"entity": list}},
		{ID: "e", Kind: "binary", X: 0, Y: 3, W: 4, H: 2, Options: map[string]any{"entity": "binary_sensor.front_door, person.alex", "label": "Front"}},
		{ID: "f", Kind: "binary", X: 4, Y: 3, W: 2, H: 1, Options: map[string]any{"entity": "binary_sensor.front_door"}},
	}
	l := store.Layout{Cols: 8, Rows: 6, Widgets: ws}
	for _, w := range ws {
		c := &ctx{states: st, opts: w.Options}
		if kinds["binary"].alert(c) {
			t.Errorf("widget %s is inverted as a whole", w.ID)
		}
	}
	g := Render(l, store.DefaultPanel, st, goldenNow).ToGray()
	if dir := os.Getenv("HOMEPAPER_RENDER_OUT"); dir != "" {
		writePNG(t, filepath.Join(dir, "state-list.png"), g)
	}
}

// One-row temperature widgets show humidity on the temperature's line when
// both fit, and drop it otherwise.
func TestInlineHumidity(t *testing.T) {
	st := demoStates(t)
	temp := func(id, v, unit string) string {
		st.Set(id, &ha.Entity{ID: id, State: v, Attributes: map[string]any{"friendly_name": "Temperature", "unit_of_measurement": unit}})
		return id
	}
	hot := temp("sensor.t_hot", "104.0", "°F")
	cold := temp("sensor.t_cold", "-12.5", "°C")
	w := func(id string, x, y, ww, h int, entity, align string, hum bool) store.Widget {
		o := map[string]any{"entity": entity, "align": align}
		if hum {
			o["humidity_entity"] = "sensor.living_room_humidity"
		}
		return store.Widget{ID: id, Kind: "temperature", X: x, Y: y, W: ww, H: h, Options: o}
	}
	lr := "sensor.living_room_temperature"
	l := store.Layout{Cols: 8, Rows: 6, Widgets: []store.Widget{
		w("before", 0, 0, 2, 1, lr, "left", false),
		w("left", 2, 0, 2, 1, lr, "left", true),
		w("center", 4, 0, 2, 1, lr, "center", true),
		w("right", 6, 0, 2, 1, lr, "right", true),
		w("w3", 0, 1, 3, 1, lr, "left", true),
		w("w4", 3, 1, 4, 1, lr, "left", true),
		w("one", 7, 1, 1, 1, lr, "left", true),
		w("cold", 0, 2, 2, 1, cold, "left", true),
		w("hot", 2, 2, 2, 1, hot, "left", true),
		w("tall", 4, 2, 2, 2, lr, "left", true),
	}}
	g := Render(l, store.DefaultPanel, st, goldenNow).ToGray()
	if dir := os.Getenv("HOMEPAPER_RENDER_OUT"); dir != "" {
		writePNG(t, filepath.Join(dir, "inline-humidity.png"), g)
	}
	ink := func(x0, y0, x1, y1 int) bool {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				if g.GrayAt(x, y).Y == 0 {
					return true
				}
			}
		}
		return false
	}
	// Left-aligned 2×1 with humidity: ink near the right edge of its cell.
	if !ink(330, 40, 388, 70) {
		t.Error("2×1 shows no humidity at its right edge")
	}
	// Without humidity, the same area stays empty.
	if ink(130, 40, 188, 70) {
		t.Error("2×1 without a humidity sensor draws something at its right edge")
	}
}

// augsburg is the user's db-infoscreen sensor (2026-11-16, 20:56), plus a
// later tram and a cancelled bus.
func augsburg() ha.Entity {
	dep := func(line, typ, dest, plat string, sched, rt float64, cancelled float64) map[string]any {
		return map[string]any{"line": line, "type": typ, "destination": dest, "platform": plat,
			"sched_datetime": sched, "rt_datetime": rt, "datetime": rt, "is_cancelled": cancelled,
			"delay": (rt - sched) / 60, "departure_current": "wrong"}
	}
	list := []any{
		dep("RB86", "R-Bahn", "Dinkelscherben", "1", 1794858720, 1794858900, 0),
		dep("RB87", "R-Bahn", "Donauwörth", "3", 1794859020, 1794859020, 0),
		dep("506e", "Regionalbus", "Zusmarshausen Schulzentrum", "B", 1794858960, 1794859140, 0),
		dep("2", "Straßenbahn", "Stadtbergen, Augsburg West P+R", "D", 1794859140, 1794859140, 0),
		dep("2", "Straßenbahn", "Haunstetten, Nord", "C", 1794859440, 1794859440, 0),
		dep("35", "Bus", "Hammerschmiede", "A", 1794859560, 1794859560, 1),
	}
	return ha.Entity{ID: "sensor.db_infoscreen_augsburg_oberhausen_departures", State: "20:58", Attributes: map[string]any{
		"next_departures": list, "station": "Augsburg-Oberhausen", "last_updated": "2026-11-16T20:56:59.261298+01:00",
		"friendly_name": "DB Infoscreen Augsburg-Oberhausen Departures"}}
}

func TestDepartures(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no tz data")
	}
	now := time.Date(2026, 11, 16, 20, 56, 30, 0, berlin)
	e := augsburg()
	got := selectDepartures(parseDepartures(e), now, 0, nil, nil)
	var order []string
	for _, d := range got {
		order = append(order, d.line+" "+d.rt.In(berlin).Format("15:04"))
	}
	// RB86 left at 20:55 (20:52 +3) and is gone; 506e (20:56 +3) sorts by
	// its real time; times come from rt_datetime, not departure_current.
	want := "RB87 20:57,506e 20:59,2 20:59,2 21:04,35 21:06"
	if strings.Join(order, ",") != want {
		t.Errorf("order %v, want %s", order, want)
	}
	if d := got[1]; d.delay() != 3 {
		t.Errorf("506e delay %d", d.delay())
	}
	if !got[4].cancelled {
		t.Error("35 not cancelled")
	}
	if trams := selectDepartures(parseDepartures(e), now, 0, nil, []string{"straßenbahn"}); len(trams) != 2 {
		t.Errorf("type filter kept %d", len(trams))
	}
	if two := selectDepartures(parseDepartures(e), now, 0, parseLines("2"), nil); len(two) != 2 {
		t.Errorf("line filter kept %d", len(two))
	}
	// One direction per line: tram 2 towards Stadtbergen only, RB87 and
	// 506e towards anything, case and spacing ignored.
	dir := selectDepartures(parseDepartures(e), now, 0, parseLines(" 2 > stadtbergen ; RB87, 506E"), nil)
	var got2 []string
	for _, d := range dir {
		got2 = append(got2, d.line+" "+d.dest)
	}
	if want := "RB87 Donauwörth|506e Zusmarshausen Schulzentrum|2 Stadtbergen, Augsburg West P+R"; strings.Join(got2, "|") != want {
		t.Errorf("per-line direction: %v", got2)
	}
	if none := selectDepartures(parseDepartures(e), now, 0, parseLines("2 > Nowhere"), nil); len(none) != 0 {
		t.Errorf("unknown direction kept %d", len(none))
	}
	if walk := selectDepartures(parseDepartures(e), now, 5*time.Minute, nil, nil); len(walk) != 2 || walk[0].line != "2" {
		t.Errorf("walk filter: %v", walk)
	}

	st := demoStates(t)
	st.Set(e.ID, &e)
	w := func(id string, x, y, ww, h int, opts map[string]any) store.Widget {
		opts["entity"] = e.ID
		return store.Widget{ID: id, Kind: "departures", X: x, Y: y, W: ww, H: h, Options: opts}
	}
	l := store.Layout{Cols: 8, Rows: 6, Widgets: []store.Widget{
		w("a", 0, 0, 4, 4, map[string]any{}),
		w("b", 4, 0, 4, 2, map[string]any{"types": "Straßenbahn"}),
		w("c", 4, 2, 2, 2, map[string]any{}),
		w("d", 6, 2, 2, 2, map[string]any{"walk": 10.0}),
		w("e", 0, 4, 4, 1, map[string]any{}),
		w("f", 4, 4, 4, 2, map[string]any{"lines": "9"}),
		w("g", 0, 5, 4, 1, map[string]any{"entity": "sensor.gone"}),
	}}
	l.Widgets[6].Options["entity"] = "sensor.gone"
	g := Render(l, store.DefaultPanel, st, now).ToGray()
	if dir := os.Getenv("HOMEPAPER_RENDER_OUT"); dir != "" {
		writePNG(t, filepath.Join(dir, "departures.png"), g)
	}
}

// Grey mode keeps grey off everything that changes with entity state or
// time, so routine updates stay black/white partial refreshes (D-018).
func TestGreyRoutineChangesStayBlackWhite(t *testing.T) {
	l := greyLayout(heroLayout(""))
	st := demoStates(t)
	base := Render(l, store.DefaultPanel, st, goldenNow)
	if !base.HasGrey() {
		t.Fatal("grey mode drew no grey")
	}
	changes := []struct {
		name string
		do   func(st *ha.Store) time.Time
	}{
		{"minute tick", func(*ha.Store) time.Time { return goldenNow.Add(time.Minute) }},
		{"ten minutes", func(*ha.Store) time.Time { return goldenNow.Add(17 * time.Minute) }},
		{"power", func(st *ha.Store) time.Time { setState(st, "sensor.power_consumption", "1088"); return goldenNow }},
		{"temperature", func(st *ha.Store) time.Time { setState(st, "sensor.living_room_temperature", "19.9"); return goldenNow }},
		{"light off", func(st *ha.Store) time.Time { setState(st, "light.living_room", "off"); return goldenNow }},
		{"light dimmed", func(st *ha.Store) time.Time {
			e, _ := st.Get("light.hallway")
			e.Attributes = map[string]any{"friendly_name": "Hallway", "brightness": 40.0}
			st.Set(e.ID, &e)
			return goldenNow
		}},
	}
	for _, c := range changes {
		t.Run(c.name, func(t *testing.T) {
			st := demoStates(t)
			now := c.do(st)
			next := Render(l, store.DefaultPanel, st, now)
			regions := frame.Diff(base, next)
			if len(regions) == 0 {
				t.Fatal("nothing changed")
			}
			for _, r := range regions {
				if frame.ChangedGrey(base, next, r) {
					t.Fatalf("region %v changes grey pixels", r)
				}
			}
		})
	}

	// An alert widget turning black or back is the one routine change that
	// touches grey (its label, the rules beside it). It is rare, and the
	// large black plate is better served by a full refresh anyway.
	st = demoStates(t)
	setState(st, "binary_sensor.bathroom_window", "off")
	next := Render(l, store.DefaultPanel, st, goldenNow)
	grey := false
	for _, r := range frame.Diff(base, next) {
		grey = grey || frame.ChangedGrey(base, next, r)
	}
	if !grey {
		t.Fatal("leaving the alert state should count as a grey change (full refresh)")
	}
}

// Without grey mode, and on 1bpp panels, the frame is pure black and white.
func TestGreyModeOffIsBlackWhite(t *testing.T) {
	st := demoStates(t)
	if b := Render(heroLayout(""), store.DefaultPanel, st, goldenNow); b.Depth != 2 || b.HasGrey() {
		t.Fatalf("2bpp with grey mode off: depth %d, grey %v", b.Depth, b.HasGrey())
	}
	p := store.DefaultPanel
	p.Format = frame.Format1bpp
	b1 := Render(greyLayout(heroLayout("")), p, st, goldenNow)
	b2 := Render(heroLayout(""), store.DefaultPanel, st, goldenNow)
	if b1.Depth != 1 || countDiff(b1.ToGray(), b2.ToGray()) != 0 {
		t.Fatal("a 1bpp panel must ignore grey mode and match the 2bpp black/white frame")
	}
}

func TestEntityLabels(t *testing.T) {
	c := &ctx{opts: map[string]any{"labels": "binary_sensor.a=Entrance\n\ncover.b = Garage = big \nbad\nlight.c="}}
	got := c.entityLabels("labels")
	if len(got) != 2 || got["binary_sensor.a"] != "Entrance" || got["cover.b"] != "Garage = big" {
		t.Fatalf("labels = %v", got)
	}
}
