package ha

import (
	"context"
	"math"
	"math/rand/v2"
	"time"
)

// RunDemo fills the store with fake entities and changes some of them every
// few seconds, so the server works without a Home Assistant instance.
func RunDemo(ctx context.Context, s *Store) {
	now := time.Now()
	e := func(id, state string, attrs map[string]any) Entity {
		return Entity{ID: id, State: state, Attributes: attrs, LastChanged: now}
	}
	s.Replace([]Entity{
		// One of each kind the widgets can show.
		e("sensor.living_room_temperature", "21.4", map[string]any{"friendly_name": "Living room", "unit_of_measurement": "°C", "device_class": "temperature"}),
		e("sensor.living_room_humidity", "48", map[string]any{"friendly_name": "Living room humidity", "unit_of_measurement": "%", "device_class": "humidity"}),
		e("sensor.outdoor_temperature", "7.8", map[string]any{"friendly_name": "Outdoor", "unit_of_measurement": "°C", "device_class": "temperature"}),
		e("sensor.power_consumption", "412", map[string]any{"friendly_name": "Power", "unit_of_measurement": "W", "device_class": "power"}),
		e("climate.living_room", "heat", map[string]any{"friendly_name": "Living room thermostat", "current_temperature": 21.4, "temperature": 22.0, "hvac_action": "heating"}),
		e("weather.home", "partlycloudy", map[string]any{"friendly_name": "Home", "temperature": 8.0, "humidity": 81.0, "temperature_unit": "°C"}),
		e("light.living_room", "on", map[string]any{"friendly_name": "Living room light", "brightness": 180.0}),
		e("light.kitchen", "off", map[string]any{"friendly_name": "Kitchen light"}),
		e("light.hallway", "on", map[string]any{"friendly_name": "Hallway", "brightness": 255.0}),
		e("binary_sensor.front_door", "off", map[string]any{"friendly_name": "Front door", "device_class": "door"}),
		e("binary_sensor.bathroom_window", "on", map[string]any{"friendly_name": "Bathroom window", "device_class": "window"}),
		e("person.alex", "home", map[string]any{"friendly_name": "Alex"}),
	})
	s.SetForecast("weather.home", DemoForecast(now))
	s.setConnected(true)

	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	start := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		phase := time.Since(start).Seconds() / 60
		update := func(id string, fn func(*Entity)) {
			if cur, ok := s.Get(id); ok {
				cur.LastChanged = time.Now()
				fn(&cur)
				s.Set(id, &cur)
			}
		}
		update("sensor.living_room_temperature", func(e *Entity) {
			e.State = formatValue(math.Round((21.4+0.6*math.Sin(phase))*10) / 10)
		})
		update("sensor.power_consumption", func(e *Entity) {
			e.State = formatValue(float64(300 + rand.IntN(400)))
		})
		if rand.IntN(4) == 0 {
			update("light.kitchen", func(e *Entity) {
				if e.State == "on" {
					e.State = "off"
					delete(e.Attributes, "brightness")
				} else {
					e.State = "on"
					e.Attributes["brightness"] = 120.0
				}
			})
		}
		if rand.IntN(6) == 0 {
			update("binary_sensor.front_door", func(e *Entity) {
				e.State = map[string]string{"on": "off", "off": "on"}[e.State]
			})
		}
	}
}

// DemoForecast is today and the next five days, matching the panel
// mockups in docs/design.
func DemoForecast(now time.Time) []Forecast {
	days := []struct {
		cond   string
		hi, lo float64
	}{
		{"partlycloudy", 9, 3}, {"rainy", 9, 4}, {"partlycloudy", 11, 5},
		{"cloudy", 8, 3}, {"snowy", 4, -1}, {"sunny", 7, 0},
	}
	y, m, d := now.Date()
	out := make([]Forecast, len(days))
	for i, x := range days {
		hi, lo := x.hi, x.lo
		out[i] = Forecast{Datetime: time.Date(y, m, d+i, 12, 0, 0, 0, now.Location()), Condition: x.cond, Temperature: &hi, TempLow: &lo}
	}
	return out
}
