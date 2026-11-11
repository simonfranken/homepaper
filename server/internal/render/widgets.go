package render

import (
	"fmt"
	"math"
	"strings"
)

// FieldType tells the editor which input to show for an option.
type FieldType string

const (
	FieldEntity   FieldType = "entity"   // one entity ID, optionally filtered by Domains
	FieldEntities FieldType = "entities" // comma-separated entity IDs
	FieldText     FieldType = "text"
	FieldNumber   FieldType = "number"
	FieldSelect   FieldType = "select"
	FieldBool     FieldType = "bool"
)

// Choice is one option of a select field.
type Choice struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type Field struct {
	Key         string    `json:"key"`
	Label       string    `json:"label"`
	Type        FieldType `json:"type"`
	Help        string    `json:"help,omitempty"`        // one line shown under the input
	Placeholder string    `json:"placeholder,omitempty"` // what an empty input means
	Domains     []string  `json:"domains,omitempty"`
	Choices     []Choice  `json:"choices,omitempty"`
	Default     any       `json:"default,omitempty"`
	Required    bool      `json:"required,omitempty"`
}

// Kind is a widget kind as exposed to the editor.
type Kind struct {
	Name        string  `json:"name"`
	Label       string  `json:"label"`
	Description string  `json:"description"`
	Fields      []Field `json:"fields"`
	MinW        int     `json:"min_w"`
	MinH        int     `json:"min_h"`
	DefaultW    int     `json:"default_w"`
	DefaultH    int     `json:"default_h"`

	render func(c *ctx, r rect)
}

var borderField = Field{Key: "border", Label: "Draw a frame around it", Type: FieldBool, Default: true}
var labelField = Field{Key: "label", Label: "Label", Type: FieldText, Placeholder: "Entity name"}

var kinds = map[string]*Kind{}

// Kinds lists all widget kinds in a stable order.
func Kinds() []*Kind {
	order := []string{"value", "temperature", "climate", "weather", "lights", "binary", "clock", "text"}
	out := make([]*Kind, 0, len(order))
	for _, n := range order {
		out = append(out, kinds[n])
	}
	return out
}

func register(k *Kind) {
	k.Fields = append(k.Fields, borderField)
	kinds[k.Name] = k
}

func init() {
	register(&Kind{
		Name: "value", Label: "Value", Description: "Any sensor reading with a label and unit.",
		MinW: 1, MinH: 1, DefaultW: 2, DefaultH: 1,
		Fields: []Field{
			{Key: "entity", Label: "Entity", Type: FieldEntity, Required: true},
			labelField,
			{Key: "unit", Label: "Unit", Type: FieldText, Placeholder: "From the entity"},
			{Key: "decimals", Label: "Decimals", Type: FieldNumber, Placeholder: "As reported",
				Help: "Leave empty to show the value as Home Assistant reports it."},
		},
		render: renderValue,
	})
	register(&Kind{
		Name: "temperature", Label: "Temperature", Description: "A temperature in large digits, with humidity if you like.",
		MinW: 1, MinH: 1, DefaultW: 2, DefaultH: 2,
		Fields: []Field{
			{Key: "entity", Label: "Temperature sensor", Type: FieldEntity, Domains: []string{"sensor"}, Required: true},
			{Key: "humidity_entity", Label: "Humidity sensor", Type: FieldEntity, Domains: []string{"sensor"},
				Help: "Optional. Shown below the temperature when the widget is tall enough."},
			labelField,
		},
		render: renderTemperature,
	})
	register(&Kind{
		Name: "climate", Label: "Thermostat", Description: "Current and target temperature of a climate entity.",
		MinW: 2, MinH: 1, DefaultW: 4, DefaultH: 1,
		Fields: []Field{
			{Key: "entity", Label: "Climate entity", Type: FieldEntity, Domains: []string{"climate"}, Required: true},
			labelField,
		},
		render: renderClimate,
	})
	register(&Kind{
		Name: "weather", Label: "Weather", Description: "Current condition, temperature and humidity.",
		MinW: 2, MinH: 1, DefaultW: 4, DefaultH: 2,
		Fields: []Field{
			{Key: "entity", Label: "Weather entity", Type: FieldEntity, Domains: []string{"weather"}, Required: true},
			labelField,
		},
		render: renderWeather,
	})
	register(&Kind{
		Name: "lights", Label: "Lights", Description: "Which lights are on, and how bright.",
		MinW: 2, MinH: 1, DefaultW: 3, DefaultH: 2,
		Fields: []Field{
			{Key: "entities", Label: "Lights", Type: FieldEntities, Domains: []string{"light", "switch"}, Required: true},
			{Key: "label", Label: "Heading", Type: FieldText, Placeholder: "None"},
		},
		render: renderLights,
	})
	register(&Kind{
		Name: "binary", Label: "State", Description: "Open or closed, on or off, home or away.",
		MinW: 1, MinH: 1, DefaultW: 2, DefaultH: 1,
		Fields: []Field{
			{Key: "entity", Label: "Entity", Type: FieldEntity, Domains: []string{"binary_sensor", "switch", "input_boolean", "person", "light", "lock", "cover"}, Required: true},
			labelField,
		},
		render: renderBinary,
	})
	register(&Kind{
		Name: "clock", Label: "Date / time", Description: "Today's date, the time, or both. Updates once a minute.",
		MinW: 1, MinH: 1, DefaultW: 4, DefaultH: 1,
		Fields: []Field{
			{Key: "format", Label: "Format", Type: FieldSelect, Default: "date", Choices: []Choice{
				{"date", "Date (Monday, 2 January)"}, {"time", "Time (15:04)"}, {"datetime", "Date and time (Mon 2 Jan, 15:04)"}}},
		},
		render: renderClock,
	})
	register(&Kind{
		Name: "text", Label: "Text", Description: "A static heading or note.",
		MinW: 1, MinH: 1, DefaultW: 2, DefaultH: 1,
		Fields: []Field{
			{Key: "text", Label: "Text", Type: FieldText, Required: true},
			{Key: "size", Label: "Size", Type: FieldSelect, Default: "medium", Choices: []Choice{
				{"small", "Small"}, {"medium", "Medium"}, {"large", "Large"}}},
		},
		render: renderText,
	})
}

func renderValue(c *ctx, r rect) {
	e, ok := c.entity("entity")
	r = c.label(c.str("label", e.Name()), r)
	if !ok {
		c.text(unavailable, fontBold, r.H, r, 0.5, 0.5)
		return
	}
	unit := c.str("unit", e.Attr("unit_of_measurement"))
	val := formatNumber(e.State, c.num("decimals", -1))
	main, unitR := r, rect{}
	if unit != "" {
		main, unitR = r.splitLeft(r.W * 0.72)
	}
	c.text(val, fontBold, r.H*0.9, main, 1, 0.55)
	if unit != "" {
		c.text(unit, fontMedium, math.Min(r.H*0.4, 28), unitR.inset(4), 0, 0.7)
	}
}

func renderTemperature(c *ctx, r rect) {
	e, ok := c.entity("entity")
	r = c.label(c.str("label", e.Name()), r)
	hum, hasHum := c.entity("humidity_entity")
	main := r
	var bottom rect
	if hasHum && r.H > 70 {
		main, bottom = r.splitTop(r.H * 0.72)
	}
	if !ok {
		c.text(unavailable, fontBold, main.H, main, 0.5, 0.5)
	} else {
		unit := e.Attr("unit_of_measurement")
		if unit == "" || unit == "°C" || unit == "°F" {
			unit = "°"
		}
		c.text(formatNumber(e.State, 1)+unit, fontBold, main.H*0.85, main, 0.5, 0.5)
	}
	if hasHum && bottom.H > 0 {
		c.text(formatNumber(hum.State, 0)+" % humidity", fontRegular, math.Min(bottom.H, 20), bottom, 0.5, 0.5)
	}
}

func renderClimate(c *ctx, r rect) {
	e, ok := c.entity("entity")
	r = c.label(c.str("label", e.Name()), r)
	if !ok {
		c.text(unavailable, fontBold, r.H, r, 0.5, 0.5)
		return
	}
	cur := "–"
	if v, ok := e.AttrFloat("current_temperature"); ok {
		cur = fmt.Sprintf("%.1f°", v)
	}
	target := "–"
	if v, ok := e.AttrFloat("temperature"); ok {
		target = fmt.Sprintf("%.1f°", v)
	}
	left, right := r.splitLeft(r.W * 0.6)
	c.text(cur+" → "+target, fontBold, left.H*0.8, left, 0, 0.5)
	action := e.Attr("hvac_action")
	if action == "" {
		action = e.State
	}
	c.text(strings.ToUpper(action[:min(1, len(action))])+action[min(1, len(action)):], fontRegular, math.Min(right.H*0.6, 24), right, 1, 0.5)
}

func renderWeather(c *ctx, r rect) {
	e, ok := c.entity("entity")
	if lbl := c.str("label", ""); lbl != "" {
		r = c.label(lbl, r)
	}
	if !ok {
		c.text(unavailable, fontBold, r.H, r, 0.5, 0.5)
		return
	}
	iconSize := math.Min(r.H, r.W*0.4)
	iconR, rest := r.splitLeft(iconSize)
	weatherIcon(c.dc, e.State, iconR.X+iconR.W/2, iconR.Y+iconR.H/2, iconSize*0.95)
	rest = rest.inset(4)
	temp := "–"
	if v, ok := e.AttrFloat("temperature"); ok {
		temp = fmt.Sprintf("%.0f°", v)
	}
	top, bottom := rest.splitTop(rest.H * 0.62)
	c.text(temp, fontBold, top.H, top, 0, 0.6)
	desc := weatherLabels[e.State]
	if desc == "" {
		desc = e.State
	}
	if v, ok := e.AttrFloat("humidity"); ok {
		desc += fmt.Sprintf(" · %.0f %%", v)
	}
	c.text(desc, fontRegular, math.Min(bottom.H, 22), bottom, 0, 0.3)
}

func renderLights(c *ctx, r rect) {
	r = c.label(c.str("label", ""), r)
	var ids []string
	for _, id := range strings.Split(c.str("entities", ""), ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return
	}
	rowH := math.Min(r.H/float64(len(ids)), 40)
	for i, id := range ids {
		row := rect{r.X, r.Y + float64(i)*rowH, r.W, rowH}
		if row.Y+row.H > r.Y+r.H+0.5 {
			break
		}
		e, ok := c.states.Get(id)
		on := ok && e.State == "on"
		iconR, rest := row.splitLeft(rowH)
		iconBulb(c.dc, iconR.X+iconR.W/2, iconR.Y+iconR.H/2, rowH*0.8, on)
		name, state := rest.splitLeft(rest.W * 0.7)
		c.text(e.Name(), fontRegular, rowH*0.6, name.inset(2), 0, 0.5)
		status := "off"
		switch {
		case !ok || e.State == "unavailable":
			status = unavailable
		case on:
			status = "on"
			if b, ok := e.AttrFloat("brightness"); ok {
				status = fmt.Sprintf("%.0f %%", b/255*100)
			}
		}
		c.text(status, fontBold, rowH*0.55, state.inset(2), 1, 0.5)
	}
}

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
	case "door", "window", "opening", "garage_door":
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

func renderBinary(c *ctx, r rect) {
	e, ok := c.entity("entity")
	r = c.label(c.str("label", e.Name()), r)
	if !ok {
		c.text(unavailable, fontBold, r.H, r, 0.5, 0.5)
		return
	}
	txt, active := binaryText(e.Domain(), e.Attr("device_class"), e.State)
	s := math.Min(r.H, 36)
	iconR, rest := r.splitLeft(s + 6)
	iconDot(c.dc, iconR.X+s/2, iconR.Y+iconR.H/2, s, active)
	c.text(txt, fontBold, math.Min(r.H*0.8, 40), rest, 0, 0.5)
}

func renderClock(c *ctx, r rect) {
	var s string
	switch c.str("format", "date") {
	case "time":
		s = c.now.Format("15:04")
	case "datetime":
		s = c.now.Format("Mon 2 Jan, 15:04")
	default:
		s = c.now.Format("Monday, 2 January")
	}
	c.text(s, fontBold, r.H*0.7, r, 0, 0.5)
}

func renderText(c *ctx, r rect) {
	sizes := map[string]float64{"small": 18, "medium": 28, "large": 56}
	px := sizes[c.str("size", "medium")]
	if px == 0 {
		px = 28
	}
	c.text(c.str("text", ""), fontBold, px, r, 0.5, 0.5)
}

// HasKind reports whether a widget kind exists.
func HasKind(name string) bool { _, ok := kinds[name]; return ok }
