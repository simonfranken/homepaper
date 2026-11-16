package ha

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Forecast is one entry of a weather entity's daily forecast, as returned
// by the weather.get_forecasts service.
type Forecast struct {
	Datetime    time.Time `json:"datetime"`
	Condition   string    `json:"condition"`
	Temperature *float64  `json:"temperature"` // the day's high
	TempLow     *float64  `json:"templow"`
}

const (
	// forecastEvery is how often daily forecasts are fetched again.
	forecastEvery = 30 * time.Minute
	// forecastMinGap limits refetches triggered by state changes.
	forecastMinGap = 10 * time.Minute
)

// forecastRequest is the call_service message asking for one entity's
// daily forecast.
func forecastRequest(id int, entityID string) map[string]any {
	return map[string]any{
		"id":              id,
		"type":            "call_service",
		"domain":          "weather",
		"service":         "get_forecasts",
		"service_data":    map[string]any{"type": "daily"},
		"target":          map[string]any{"entity_id": entityID},
		"return_response": true,
	}
}

// parseForecastResult reads a get_forecasts result:
// {"context": …, "response": {"weather.home": {"forecast": [ … ]}}}.
func parseForecastResult(raw json.RawMessage, entityID string) ([]Forecast, error) {
	var res struct {
		Response map[string]struct {
			Forecast []Forecast `json:"forecast"`
		} `json:"response"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("decode forecast: %w", err)
	}
	r, ok := res.Response[entityID]
	if !ok {
		return nil, fmt.Errorf("no forecast for %s in the response", entityID)
	}
	return r.Forecast, nil
}

func isWeather(id string) bool { return strings.HasPrefix(id, "weather.") }
