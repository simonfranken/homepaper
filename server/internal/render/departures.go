package render

import (
	"fmt"
	"image/color"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"homepaper/internal/ha"
)

// The Departures widget (docs/design.md): the next buses, trams and trains
// from a departures sensor's attribute list, as written by db-infoscreen.

// departure is one entry of a departures sensor.
type departure struct {
	line, dest, platform, typ string
	sched, rt                 time.Time
	cancelled                 bool
}

func (d departure) delay() int { return int(math.Round(d.rt.Sub(d.sched).Minutes())) }

// parseDepartures reads a sensor's departure list: next_departures (or
// departures), each with sched_datetime and rt_datetime as Unix seconds.
// It uses those two rather than departure_current, which db-infoscreen
// appears to compute with the delay counted twice.
func parseDepartures(e ha.Entity) []departure {
	raw, _ := e.Attributes["next_departures"].([]any)
	if raw == nil {
		raw, _ = e.Attributes["departures"].([]any)
	}
	var out []departure
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		sched, ok := unixAttr(m["sched_datetime"])
		if !ok {
			if sched, ok = unixAttr(m["datetime"]); !ok {
				continue
			}
		}
		rt, ok := unixAttr(m["rt_datetime"])
		if !ok {
			rt = sched.Add(time.Duration(numAttr(m["delay"])) * time.Minute)
		}
		out = append(out, departure{
			line:      strAttr(m["line"]),
			dest:      strAttr(m["destination"]),
			platform:  strAttr(m["platform"]),
			typ:       strAttr(m["type"]),
			sched:     sched,
			rt:        rt,
			cancelled: truthy(m["is_cancelled"]),
		})
	}
	return out
}

func strAttr(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return ""
}

func numAttr(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	}
	return 0
}

func unixAttr(v any) (time.Time, bool) {
	n := numAttr(v)
	if n <= 0 {
		return time.Time{}, false
	}
	return time.Unix(int64(n), 0), true
}

func truthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case float64:
		return x != 0
	case string:
		return x == "1" || strings.EqualFold(x, "true")
	}
	return false
}

// selectDepartures keeps what can still be caught: not yet gone, not
// leaving within walk, matching the line and type filters. Sorted by when
// each trip really leaves.
func selectDepartures(ds []departure, now time.Time, walk time.Duration, lines, types []string) []departure {
	from := now.Truncate(time.Minute).Add(walk)
	var out []departure
	for _, d := range ds {
		if d.rt.Before(from) {
			continue
		}
		if len(lines) > 0 && !matchAny(d.line, lines, true) {
			continue
		}
		if len(types) > 0 && !matchAny(d.typ, types, false) {
			continue
		}
		out = append(out, d)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].rt.Before(out[j].rt) })
	return out
}

// matchAny matches s against a filter list, whole (lines) or as a part
// (types), ignoring case.
func matchAny(s string, list []string, whole bool) bool {
	for _, f := range list {
		if whole && strings.EqualFold(s, f) || !whole && strings.Contains(strings.ToLower(s), strings.ToLower(f)) {
			return true
		}
	}
	return false
}

func splitList(s string) []string {
	var out []string
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

func renderDepartures(c *ctx, r R) {
	e, ok := c.entity("entity")
	heading := c.str("label", strAttr(e.Attributes["station"]))
	if heading == "" {
		heading = e.Name()
	}
	if !ok {
		c.missing(r, heading)
		return
	}
	walk := time.Duration(math.Max(0, c.num("walk", 0))) * time.Minute
	ds := selectDepartures(parseDepartures(e), c.now, walk, splitList(c.str("lines", "")), splitList(c.str("types", "")))

	f17 := face(fontMedium, 17)
	if r.H >= 90 {
		asOf := ""
		// "as of" only where it leaves the heading room (3 cells and up).
		if t, err := time.Parse(time.RFC3339Nano, strAttr(e.Attributes["last_updated"])); err == nil && r.W >= 250 {
			asOf = "as of " + t.In(c.now.Location()).Format("15:04")
		}
		aw := 0
		if asOf != "" {
			aw = width(face(fontRegular, 14), asOf) + 12
			c.T(asOf, fontRegular, 14, r.right(), r.Y+capH(f17), 1)
		}
		c.T(ellip(heading, fontMedium, 17, r.W-aw), fontMedium, 17, r.X, r.Y+capH(f17), 0)
		r = R{r.X, r.Y + capH(f17) + 10, r.W, r.H - capH(f17) - 10}
	}
	if len(ds) == 0 {
		f := face(fontRegular, 17)
		c.T("No departures", fontRegular, 17, r.X, r.Y+(min(r.H, 34)+capH(f))/2, 0)
		return
	}
	rowH := 34
	if r.H < 70 {
		rowH = r.H / 2
	}
	n := min(len(ds), r.H/rowH)
	if n == 0 {
		return
	}
	ds = ds[:n]
	px := 17
	if rowH < 30 {
		px = 14
	}
	fb := face(fontBold, px)
	f14 := face(fontBold, 14)

	// Columns: line badge | destination | platform | time | delay tag.
	badgeW, delayW, platW := 30, 0, 0
	for _, d := range ds {
		badgeW = max(badgeW, width(fb, d.line)+12)
		if d.delay() > 0 && !d.cancelled {
			delayW = max(delayW, width(f14, fmt.Sprintf("+%d", d.delay()))+8+6)
		}
	}
	if c.opts["platform"] != false && r.W >= 300 {
		for _, d := range ds {
			if d.platform != "" {
				platW = max(platW, width(face(fontRegular, 14), d.platform)+14)
			}
		}
	}
	timeW := width(fb, "00:00")
	ink := c.ink
	for i, d := range ds {
		y := r.Y + i*rowH
		if i > 0 {
			c.dotted(r.X, r.right(), y)
		}
		cy := y + rowH/2
		base := cy + capH(fb)/2

		// The line as a sign: solid when running, hollow when cancelled.
		bh := float64(min(rowH-10, 26))
		bx, by := float64(r.X), float64(cy)-bh/2
		c.setInk()
		if d.cancelled {
			c.dc.SetLineWidth(2)
			c.dc.DrawRoundedRectangle(bx+1, by+1, float64(badgeW)-2, bh-2, 4)
			c.dc.Stroke()
		} else {
			c.dc.DrawRoundedRectangle(bx, by, float64(badgeW), bh, 4)
			c.dc.Fill()
			c.ink = color.Gray{255 - ink.Y}
		}
		c.T(ellip(d.line, fontBold, px, badgeW-6), fontBold, px, r.X+badgeW/2, base, 0.5)
		c.ink = ink

		// The actual time, and when late a small inverted "+3".
		dx := r.right() - delayW
		if delayW > 0 && d.delay() > 0 && !d.cancelled {
			txt := fmt.Sprintf("+%d", d.delay())
			tw := width(f14, txt)
			c.setInk()
			c.dc.DrawRoundedRectangle(float64(r.right()-tw-8), float64(cy-9), float64(tw+8), 18, 3)
			c.dc.Fill()
			c.ink = color.Gray{255 - ink.Y}
			c.T(txt, fontBold, 14, r.right()-4, cy+capH(f14)/2, 1)
			c.ink = ink
		}
		tx := dx - timeW
		c.T(d.rt.In(c.now.Location()).Format("15:04"), fontBold, px, dx, base, 1)
		if d.cancelled {
			c.hline(tx-2, dx+2, cy, 2)
		}
		if platW > 0 && d.platform != "" {
			c.T(d.platform, fontRegular, 14, tx-8, base, 1)
		}
		nx := r.X + badgeW + 10
		dest := d.dest
		if d.cancelled {
			dest = "Cancelled · " + dest
		}
		c.T(ellip(dest, fontRegular, px, tx-10-platW-nx), fontRegular, px, nx, base, 0)
	}
}
