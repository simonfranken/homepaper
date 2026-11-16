package render

import (
	"fmt"
	"math"
	"strings"
	"time"

	"homepaper/internal/ha"
)

// The widget kinds as designed in docs/design.md (D-015). Each one drops
// secondary information before it shrinks its primary value.

// HasKind reports whether a widget kind exists.
func HasKind(name string) bool { _, ok := kinds[name]; return ok }

// small reports a one-cell-wide widget, which gets the 14 px label.
func small(r R) bool { return r.W < 120 }

// missing draws "–" with "Not available" in words, below the label.
func (c *ctx) missing(r R, label string) {
	if small(r) {
		r = c.smallLabel(r, label)
	} else {
		r = c.label(r, label)
	}
	f := face(fontBold, 34)
	if capH(f) > r.H {
		f = face(fontBold, 20)
	}
	px := 34
	if capH(face(fontBold, 34)) > r.H {
		px = 20
	}
	base := r.Y + (r.H+capH(f))/2
	w := width(f, unavailable)
	note := width(face(fontRegular, 14), "Not available")
	withNote := r.W-w-12 >= note
	total := w
	if withNote {
		total += 12 + note
	}
	x := c.alignX(r, total)
	c.T(unavailable, fontBold, px, x, base, 0)
	if withNote {
		c.T("Not available", fontRegular, 14, x+w+12, base, 0)
	}
}

// ---------- temperature ----------

// tempParts formats a temperature state as "21" and ".4°" (one decimal).
// Units other than degrees are kept after the number.
func tempParts(state, unit string) (string, string) {
	s := formatNumber(state, 1)
	if unit == "" || unit == "°C" || unit == "°F" {
		unit = "°"
	} else {
		unit = " " + unit
	}
	w, f := splitNum(s)
	return w, f + unit
}

func renderTemperature(c *ctx, r R) {
	e, ok := c.entity("entity")
	label := c.str("label", e.Name())
	if !ok {
		c.missing(r, label)
		return
	}
	whole, frac := tempParts(e.State, e.Attr("unit_of_measurement"))
	if small(r) {
		r = c.smallLabel(r, label)
		c.bigNumber(r, whole, frac, 34, c.ax)
		return
	}
	r = c.label(r, label)
	main := r
	if hum, ok := c.entity("humidity_entity"); ok && r.H >= 90 {
		main = R{r.X, r.Y, r.W, r.H - 26}
		f := face(fontRegular, 17)
		y := r.bottom() - 2
		txt := formatNumber(hum.State, 0) + " %"
		x := c.alignX(r, 18+width(f, txt))
		c.droplet(float64(x+6), float64(y-capH(f)/2-1), 14)
		c.T(txt, fontRegular, 17, x+18, y, 0)
	}
	c.bigNumber(main, whole, frac, 80, c.ax)
}

// ---------- value ----------

func renderValue(c *ctx, r R) {
	e, ok := c.entity("entity")
	label := c.str("label", e.Name())
	if !ok {
		c.missing(r, label)
		return
	}
	unit := c.str("unit", e.Attr("unit_of_measurement"))
	val := formatNumber(e.State, c.num("decimals", -1))
	sm := small(r)
	if sm {
		r = c.smallLabel(r, label)
	} else {
		r = c.label(r, label)
	}
	icon := valueIcon(e.Attr("device_class"))
	if sm {
		icon = nil
	}
	iconW := 0
	s := math.Min(float64(r.H), 30)
	if icon != nil {
		iconW = int(s) + 8
	}
	x := r.X + iconW
	up := 25
	if sm {
		up = 17
	}
	uw := 0
	if unit != "" {
		uw = width(face(fontRegular, up), " "+unit)
	}
	px := fit(val, fontBold, []int{64, 56, 48, 44, 40, 34, 25, 20}, r.right()-x-uw, r.H)
	fb := face(fontBold, px)
	base := r.Y + (r.H+capH(fb))/2
	room := r.right() - x - uw
	val = ellip(val, fontBold, px, room)
	x = c.alignX(r, iconW+width(fb, val)+uw)
	if icon != nil {
		icon(c, float64(x)+s/2, float64(r.Y)+float64(r.H)/2, s)
		x += iconW
	}
	x += c.T(val, fontBold, px, x, base, 0)
	if unit != "" {
		c.T(" "+unit, fontRegular, up, x, base, 0)
	}
}

func valueIcon(class string) func(c *ctx, cx, cy, s float64) {
	switch class {
	case "power", "energy", "current", "voltage":
		return func(c *ctx, cx, cy, s float64) { c.bolt(cx, cy, s) }
	case "humidity", "moisture", "precipitation":
		return func(c *ctx, cx, cy, s float64) { c.droplet(cx, cy, s*0.8) }
	}
	return nil
}

// ---------- thermostat ----------

func renderClimate(c *ctx, r R) {
	e, ok := c.entity("entity")
	label := c.str("label", e.Name())
	if !ok {
		c.missing(r, label)
		return
	}
	cur, target := unavailable, unavailable
	if v, ok := e.AttrFloat("current_temperature"); ok {
		cur = fmt.Sprintf("%.1f°", v)
	}
	if v, ok := e.AttrFloat("temperature"); ok {
		target = fmt.Sprintf("%.1f°", v)
	}
	action := e.Attr("hvac_action")
	if action == "" {
		action = e.State
	}
	heating := action == "heating"
	busy := heating || action == "cooling" || action == "drying" || action == "fan"
	action = strings.ToUpper(action[:min(1, len(action))]) + strings.ReplaceAll(action[min(1, len(action)):], "_", " ")

	r = c.label(r, label)
	if r.H < 80 {
		// One row: 21.4° ▸ 22.0°, and the action when there's room. In a
		// narrow widget the numbers step down, and the target goes last.
		px := 0
		for _, s := range []int{34, 25, 20} {
			f := face(fontBold, s)
			if width(f, cur)+42+width(f, target) <= r.W {
				px = s
				break
			}
		}
		if px == 0 {
			px = fit(cur, fontBold, []int{34, 25, 20}, r.W, r.H)
			f := face(fontBold, px)
			c.T(cur, fontBold, px, r.X, r.Y+(r.H+capH(f))/2, 0)
			return
		}
		f := face(fontBold, px)
		base := r.Y + (r.H+capH(f))/2
		x := r.X
		x += c.T(cur, fontBold, px, x, base, 0) + 10
		c.arrow(float64(x), float64(base-capH(f)/2), 22)
		x += 32
		x += c.T(target, fontBold, px, x, base, 0)
		id := fontRegular
		if busy {
			id = fontBold
		}
		fa := face(id, 20)
		aw := width(fa, action)
		iconW := 0
		if heating {
			iconW = 14 + 10
		}
		if r.right()-x >= aw+iconW+16 {
			c.T(action, id, 20, r.right(), base, 1)
			if heating {
				c.flame(float64(r.right()-aw-14), float64(base-capH(fa)/2), 20)
			}
		}
		return
	}
	// Tall: the current temperature large, target and action right of a rule.
	col := r.W * 45 / 100
	whole, frac := splitNum(cur)
	c.bigNumber(R{r.X, r.Y, r.W - col, r.H}, whole, frac, 88, 0)
	rx := r.right() - col + 16
	c.vline(rx-16, r.Y+4, r.bottom()-4, 2)
	ft := face(fontMedium, 17)
	y := r.Y + capH(ft) + 6
	c.T("Set to", fontMedium, 17, rx, y, 0)
	tpx := fit(target, fontBold, []int{34, 25, 20}, col-16, r.H)
	y += capH(face(fontBold, tpx)) + 12
	c.T(target, fontBold, tpx, rx, y, 0)
	id := fontRegular
	if busy {
		id = fontBold
	}
	room := col - 16
	if heating {
		room -= 26
	}
	apx := fit(action, id, []int{20, 17, 14}, room, r.H)
	fa := face(id, apx)
	y = r.bottom() - 4
	fits := width(fa, action) <= room // a shortened action word reads worse than none
	if heating {
		c.flame(float64(rx+9), float64(y-capH(fa)/2), 22)
		if fits {
			c.T(action, id, apx, rx+26, y, 0)
		}
	} else if fits {
		c.T(action, id, apx, rx, y, 0)
	}
}

// ---------- weather ----------

// forecastDays splits a daily forecast into today's entry (if any) and the
// days after it.
func forecastDays(f []ha.Forecast, now time.Time) (*ha.Forecast, []ha.Forecast) {
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	var cur *ha.Forecast
	var next []ha.Forecast
	for i := range f {
		t := f[i].Datetime.In(now.Location())
		day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location())
		switch {
		case day.Equal(today):
			if cur == nil {
				cur = &f[i]
			}
		case day.After(today):
			next = append(next, f[i])
		}
	}
	return cur, next
}

func deg(v *float64) string {
	if v == nil {
		return unavailable
	}
	return fmt.Sprintf("%.0f°", math.Round(*v)+0) // +0 turns -0 into 0
}

func renderWeather(c *ctx, r R) {
	e, ok := c.entity("entity")
	label := c.str("label", "")
	if !ok {
		c.missing(r, c.str("label", e.Name()))
		return
	}
	if label != "" {
		r = c.label(r, label)
	}
	today, next := forecastDays(c.states.Forecast(e.ID), c.now)
	cond := weatherLabels[e.State]
	if cond == "" {
		cond = e.State
	}
	temp := unavailable
	if v, ok := e.AttrFloat("temperature"); ok {
		temp = fmt.Sprintf("%.0f", math.Round(v)+0)
	}

	top := r
	days := 0
	if r.H >= 120 && len(next) > 0 {
		days = min(len(next), 5, r.W/64)
		if days > 0 {
			top = R{r.X, r.Y, r.W, r.H - 64}
		}
	}
	s := math.Min(float64(top.H), float64(r.W)*0.32)
	weatherIcon(c.dc, e.State, float64(top.X)+s/2, float64(top.Y)+float64(top.H)/2, s)
	x := top.X + int(s) + 10
	px := 64
	if top.H < 70 {
		px = 44
	}
	tf := face(fontBold, px)
	base := top.Y + (top.H+capH(tf))/2
	x += c.T(temp, fontBold, px, x, base, 0)
	sp := smallFor(px)
	x += c.T("°", fontBold, sp, x+1, base-capH(tf)+capH(face(fontBold, sp)), 0)
	x += 16

	if r.W-(x-r.X) <= 120 && top.H >= 120 {
		// Tall and narrow: the condition goes underneath.
		c.T(ellip(cond, fontBold, 17, r.W), fontBold, 17, r.X, top.bottom()-2, 0)
	}
	if r.W-(x-r.X) > 120 {
		fb := face(fontBold, 20)
		fr := face(fontRegular, 17)
		hum, hasHum := e.AttrFloat("humidity")
		var lines []func(y int)
		gap := 8
		avail := r.right() - x
		lines = append(lines, func(y int) { c.T(ellip(cond, fontBold, 20, avail), fontBold, 20, x, y, 0) })
		if today != nil {
			lines = append(lines, func(y int) {
				c.tri(float64(x+5), float64(y-capH(fr)/2), 10, -1)
				xx := x + 14
				xx += c.T(deg(today.Temperature), fontRegular, 17, xx, y, 0)
				c.tri(float64(xx+12), float64(y-capH(fr)/2), 10, 1)
				c.T(deg(today.TempLow), fontRegular, 17, xx+21, y, 0)
			})
		}
		if hasHum && top.H >= 70 {
			lines = append(lines, func(y int) {
				c.droplet(float64(x+5), float64(y-capH(fr)/2), 12)
				c.T(fmt.Sprintf("%.0f %%", hum), fontRegular, 17, x+14, y, 0)
			})
		}
		maxLines := 3
		if top.H < 70 {
			maxLines = 2
		}
		lines = lines[:min(len(lines), maxLines)]
		total := capH(fb) + (len(lines)-1)*(capH(fr)+gap)
		y := top.Y + (top.H-total)/2 + capH(fb)
		for i, draw := range lines {
			if i == 1 {
				y += 2
			}
			draw(y)
			y += capH(fr) + gap
		}
	}
	if days > 0 {
		y0 := top.bottom() + 4
		c.dotted(r.X, r.right(), y0)
		cw := r.W / days
		f14 := face(fontBold, 14)
		for i, d := range next[:days] {
			cx := r.X + i*cw + cw/2
			c.T(d.Datetime.In(c.now.Location()).Format("Mon"), fontBold, 14, cx, y0+8+capH(f14), 0.5)
			weatherIcon(c.dc, d.Condition, float64(cx), float64(y0+35), 26)
			hs, ls := deg(d.Temperature), " "+deg(d.TempLow)
			hw, lw := width(face(fontBold, 14), hs), width(face(fontRegular, 14), ls)
			xx := cx - (hw+lw)/2
			xx += c.T(hs, fontBold, 14, xx, r.bottom(), 0)
			c.T(ls, fontRegular, 14, xx, r.bottom(), 0)
		}
	}
}

// ---------- lights ----------

func renderLights(c *ctx, r R) {
	ids := c.entityList("entities")
	type light struct {
		name     string
		on, gone bool
		pct      int
		hasPct   bool
	}
	ls := make([]light, 0, len(ids))
	n := 0
	for _, id := range ids {
		e, ok := c.states.Get(id)
		l := light{name: e.Name()}
		if !ok {
			l.name = id
		}
		switch {
		case !ok || e.State == "unavailable" || e.State == "unknown":
			l.gone = true
		case e.State == "on":
			l.on = true
			n++
			if b, ok := e.AttrFloat("brightness"); ok {
				l.pct, l.hasPct = int(math.Round(b/255*100)), true
			}
		}
		ls = append(ls, l)
	}
	f17 := face(fontMedium, 17)
	gone := 0
	for _, l := range ls {
		if l.gone {
			gone++
		}
	}
	summary := fmt.Sprintf("%d of %d on", n, len(ls))
	switch {
	case len(ls) > 0 && gone == len(ls):
		summary = unavailable
	case n == 0 && gone == 0:
		summary = "All off"
	}
	sw := width(face(fontBold, 17), summary)
	c.T(ellip(c.str("label", "Lights"), fontMedium, 17, r.W-sw-12), fontMedium, 17, r.X, r.Y+capH(f17), 0)
	c.T(summary, fontBold, 17, r.right(), r.Y+capH(f17), 1)
	r = R{r.X, r.Y + capH(f17) + 10, r.W, r.H - capH(f17) - 10}
	if len(ls) == 0 {
		return
	}
	if r.H < 40 {
		// One row tall: a bulb per light, filled when on.
		for i, l := range ls {
			if 14+i*30+13 > r.W {
				break
			}
			iconBulb(c.dc, float64(r.X+14+i*30), float64(r.Y+r.H/2), 26, l.on)
		}
		return
	}
	rowH := min(r.H/len(ls), 46)
	if rowH < 24 {
		rowH = 24 // too many lights: show those that fit, never squeeze
	}
	bars := r.W > 280
	for i, l := range ls {
		y := r.Y + i*rowH
		if y+rowH > r.bottom()+2 {
			break
		}
		if i > 0 {
			c.dotted(r.X, r.right(), y)
		}
		cy := y + rowH/2
		iconBulb(c.dc, float64(r.X+11), float64(cy+1), math.Min(24, float64(rowH)-6), l.on)
		id := fontRegular
		if l.on {
			id = fontBold
		}
		fr := face(id, 17)
		base := cy + capH(fr)/2
		stateW := 50
		if bars {
			stateW = 140
		}
		c.T(ellip(l.name, id, 17, r.W-30-stateW), id, 17, r.X+30, base, 0)
		switch {
		case l.gone:
			c.T(unavailable, fontRegular, 17, r.right(), base, 1)
		case l.on && l.hasPct:
			c.T(fmt.Sprintf("%d %%", l.pct), fontBold, 17, r.right(), base, 1)
			if bars {
				c.bar(R{r.right() - 136, cy - 6, 70, 12}, float64(l.pct)/100)
			}
		case l.on:
			c.T("On", fontBold, 17, r.right(), base, 1)
		default:
			c.T("Off", fontRegular, 17, r.right(), base, 1)
		}
	}
}

// ---------- state ----------

// binaryText maps a state to display text based on domain and device class.
func binaryText(domain, class, state string) (string, bool) {
	active := state == "on" || state == "home" || state == "open" || state == "unlocked"
	switch domain {
	case "person":
		if state == "home" {
			return "Home", true
		}
		if state == "not_home" {
			return "Away", false
		}
		return strings.ToUpper(state[:min(1, len(state))]) + state[min(1, len(state)):], false
	case "lock":
		if state == "locked" {
			return "Locked", false
		}
		return "Unlocked", true
	case "cover":
		return strings.ToUpper(state[:min(1, len(state))]) + state[min(1, len(state)):], state == "open"
	}
	switch class {
	case "door", "window", "opening", "garage_door", "garage":
		if active {
			return "Open", true
		}
		return "Closed", false
	case "motion", "occupancy", "presence":
		if active {
			return "Detected", true
		}
		return "Clear", false
	}
	if active {
		return "On", true
	}
	return "Off", false
}

// alertState reports states that ask for action (D-015): a door, window
// or garage open, a lock unlocked. Nothing else is drawn inverted.
func alertState(domain, class, state string) bool {
	switch domain {
	case "lock":
		return state == "unlocked" || state == "open"
	case "cover":
		return (class == "garage" || class == "door" || class == "window") && (state == "open" || state == "opening")
	case "binary_sensor":
		switch class {
		case "door", "window", "opening", "garage_door":
			return state == "on"
		}
	}
	return false
}

func binaryAlert(c *ctx) bool {
	e, ok := c.entity("entity")
	return ok && alertState(e.Domain(), e.Attr("device_class"), e.State)
}

func stateIcon(e ha.Entity, active bool) func(c *ctx, cx, cy, s float64) {
	class := e.Attr("device_class")
	switch {
	case e.Domain() == "lock":
		return func(c *ctx, cx, cy, s float64) { c.lock(cx, cy, s, !active) }
	case e.Domain() == "person":
		return func(c *ctx, cx, cy, s float64) { c.person(cx, cy, s, active) }
	case class == "window":
		return func(c *ctx, cx, cy, s float64) { c.window(cx, cy, s, active) }
	case class == "door" || class == "garage_door" || class == "garage" || class == "opening":
		return func(c *ctx, cx, cy, s float64) { c.door(cx, cy, s, active) }
	}
	return func(c *ctx, cx, cy, s float64) { c.dot(cx, cy, s, active) }
}

func renderBinary(c *ctx, r R) {
	e, ok := c.entity("entity")
	label := c.str("label", e.Name())
	if !ok {
		c.missing(r, label)
		return
	}
	word, active := binaryText(e.Domain(), e.Attr("device_class"), e.State)
	icon := stateIcon(e, active)
	if small(r) {
		rest := c.smallLabel(r, label)
		px := fit(word, fontBold, []int{20, 17, 14}, rest.W-26, rest.H)
		fb := face(fontBold, px)
		base := rest.Y + (rest.H+capH(fb))/2
		if width(fb, word) <= rest.W-26 {
			x := c.alignX(rest, 26+width(fb, word))
			icon(c, float64(x+10), float64(base-capH(fb)/2), 20)
			c.T(word, fontBold, px, x+26, base, 0)
		} else {
			px = fit(word, fontBold, []int{20, 17, 14}, rest.W, rest.H)
			word = ellip(word, fontBold, px, rest.W)
			c.T(word, fontBold, px, c.alignX(rest, width(face(fontBold, px), word)), base, 0)
		}
		return
	}
	r = c.label(r, label)
	px := fit(word, fontBold, []int{25, 20, 17}, r.W-36, r.H)
	fb := face(fontBold, px)
	base := r.Y + (r.H+capH(fb))/2
	word = ellip(word, fontBold, px, r.W-36)
	x := c.alignX(r, 36+width(fb, word))
	icon(c, float64(x+13), float64(base-capH(fb)/2), 28)
	c.T(word, fontBold, px, x+36, base, 0)
}

// ---------- clock ----------

func renderClock(c *ctx, r R) {
	format := c.str("format", "date")
	timeS := c.now.Format("15:04")
	fb := face(fontBold, 20)
	fr := face(fontRegular, 17)

	// The pieces first, then the whole group is placed by the alignment.
	tpx, tw := 0, 0
	if format == "time" || format == "datetime" {
		tpx = fit(timeS, fontBold, []int{56, 48, 40, 34, 25}, r.W, r.H)
		if format == "time" {
			tpx = fit(timeS, fontBold, bigSizes, r.W, r.H)
		}
		tw = width(face(fontBold, tpx), timeS)
	}
	withDate := format != "time"
	if format == "datetime" && r.W-tw-18 < width(fr, "16 November") {
		withDate = false // the date only when it fits beside the time
	}
	day, date := c.now.Format("Monday"), c.now.Format("2 January")
	if format == "date" && r.W >= 340 {
		date = c.now.Format("2 January 2006")
	}
	dateX := 4 // where the date block starts in the group
	if tw > 0 {
		dateX = tw + 22
	}
	dw := 0
	if withDate {
		avail := r.W - dateX
		day, date = ellip(day, fontBold, 20, avail), ellip(date, fontRegular, 17, avail)
		dw = max(width(fb, day), width(fr, date))
	}
	total := tw
	if withDate {
		total = dateX + dw
	}
	x := c.alignX(r, total)

	if tw > 0 {
		tf := face(fontBold, tpx)
		c.T(timeS, fontBold, tpx, x, r.Y+(r.H+capH(tf))/2, 0)
		if withDate {
			c.vline(x+tw+9, r.Y+6, r.bottom()-6, 2)
		}
	}
	if withDate {
		blockH := capH(fb) + 10 + capH(fr)
		y := r.Y + (r.H-blockH)/2 + capH(fb)
		bx := R{x + dateX, r.Y, dw, r.H}
		c.T(day, fontBold, 20, c.alignX(bx, width(fb, day)), y, 0)
		c.T(date, fontRegular, 17, c.alignX(bx, width(fr, date)), y+10+capH(fr), 0)
	}
}

// ---------- text ----------

func renderText(c *ctx, r R) {
	s := c.str("text", "")
	if s == "" {
		return
	}
	switch c.str("size", "medium") {
	case "small":
		f := face(fontRegular, 17)
		s = ellip(s, fontRegular, 17, r.W)
		c.T(s, fontRegular, 17, c.alignX(r, width(f, s)), r.Y+(r.H+capH(f))/2, 0)
	case "large":
		px := fit(s, fontBold, bigSizes[4:], r.W, r.H) // 64 px and below
		f := face(fontBold, px)
		s = ellip(s, fontBold, px, r.W)
		c.T(s, fontBold, px, c.alignX(r, width(f, s)), r.Y+(r.H+capH(f))/2, 0)
	default:
		// A heading and its rule: the rule fills the space the text leaves,
		// on both sides when centred.
		f := face(fontBold, 25)
		base := r.Y + (r.H+capH(f))/2
		mid := base - capH(f)/2
		s = ellip(s, fontBold, 25, r.W)
		w := width(f, s)
		x := c.alignX(r, w)
		c.T(s, fontBold, 25, x, base, 0)
		if x+w+12+24 <= r.right() {
			c.hline(x+w+12, r.right(), mid, 2)
		}
		if x-12-24 >= r.X {
			c.hline(r.X, x-12, mid, 2)
		}
	}
}
