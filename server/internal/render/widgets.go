package render

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

	render func(c *ctx, r R)
	// alert reports whether the widget is in a state that asks for action;
	// it is then drawn inverted (D-015). Nil for kinds that never alert.
	alert func(c *ctx) bool
}

var labelField = Field{Key: "label", Label: "Label", Type: FieldText, Placeholder: "Entity name"}

// alignField places a widget's content left, centred or right in its cell.
var alignField = Field{Key: "align", Label: "Alignment", Type: FieldSelect, Default: "left", Choices: []Choice{
	{"left", "Left"}, {"center", "Centre"}, {"right", "Right"}}}

var kinds = map[string]*Kind{}

// Kinds lists all widget kinds in a stable order.
func Kinds() []*Kind {
	order := []string{"value", "temperature", "climate", "weather", "lights", "binary", "departures", "clock", "text"}
	out := make([]*Kind, 0, len(order))
	for _, n := range order {
		out = append(out, kinds[n])
	}
	return out
}

func register(k *Kind) {
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
			alignField,
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
			alignField,
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
		Name: "weather", Label: "Weather", Description: "Current condition, today's high and low, and the next days.",
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
		Name: "binary", Label: "State", Description: "Open or closed, on or off, home or away, armed or disarmed.",
		MinW: 1, MinH: 1, DefaultW: 2, DefaultH: 1,
		Fields: []Field{
			// Stored under "entity" as a comma-separated list, so widgets
			// saved with a single entity keep working.
			{Key: "entity", Label: "Entities", Type: FieldEntities, Domains: []string{"binary_sensor", "switch", "input_boolean", "person", "light", "lock", "cover", "alarm_control_panel"}, Required: true,
				Help: "One entity shows large. Two or more show as a list, one row each."},
			{Key: "label", Label: "Label", Type: FieldText, Placeholder: "Entity name; none for a list",
				Help: "With several entities, a heading above the list."},
			alignField,
		},
		render: renderBinary,
		alert:  binaryAlert,
	})
	register(&Kind{
		Name: "clock", Label: "Date / time", Description: "Today's date, the time, or both. Updates once a minute.",
		MinW: 1, MinH: 1, DefaultW: 4, DefaultH: 1,
		Fields: []Field{
			{Key: "format", Label: "Format", Type: FieldSelect, Default: "date", Choices: []Choice{
				{"date", "Date (Monday, 16 November)"}, {"time", "Time (15:04)"}, {"datetime", "Time and date (15:04 | Monday, 16 November)"}}},
			alignField,
		},
		render: renderClock,
	})
	register(&Kind{
		Name: "departures", Label: "Departures", Description: "Next buses, trams and trains from a departures sensor (db-infoscreen).",
		MinW: 2, MinH: 1, DefaultW: 4, DefaultH: 2,
		Fields: []Field{
			{Key: "entity", Label: "Departures sensor", Type: FieldEntity, Domains: []string{"sensor"}, Required: true,
				Help: "A db-infoscreen sensor; its next_departures attribute is shown."},
			{Key: "label", Label: "Heading", Type: FieldText, Placeholder: "Station name"},
			{Key: "lines", Label: "Lines", Type: FieldText, Placeholder: "All lines",
				Help: "Comma-separated, e.g. 2, 506e."},
			{Key: "types", Label: "Types", Type: FieldText, Placeholder: "All types",
				Help: "Comma-separated parts of the type, e.g. Straßenbahn, Bus."},
			{Key: "walk", Label: "Hide departures leaving within", Type: FieldNumber, Placeholder: "0 minutes",
				Help: "Minutes you need to get to the stop."},
			{Key: "platform", Label: "Show the platform", Type: FieldBool, Default: true},
		},
		render: renderDepartures,
	})
	register(&Kind{
		Name: "text", Label: "Text", Description: "A static heading or note.",
		MinW: 1, MinH: 1, DefaultW: 2, DefaultH: 1,
		Fields: []Field{
			{Key: "text", Label: "Text", Type: FieldText, Required: true},
			{Key: "size", Label: "Size", Type: FieldSelect, Default: "medium", Choices: []Choice{
				{"small", "Note (one line)"}, {"medium", "Heading with a rule"}, {"large", "Large"}}},
			alignField,
		},
		render: renderText,
	})
}
