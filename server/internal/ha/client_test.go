package ha

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// fakeHA answers get_states with one weather entity and get_forecasts with
// two days. Like the real HA, it rejects message IDs that don't increase.
func fakeHA(t *testing.T, badIDs *atomic.Int32, forecastOK bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx := r.Context()
		_ = wsjson.Write(ctx, conn, map[string]string{"type": "auth_required"})
		var auth map[string]any
		if wsjson.Read(ctx, conn, &auth) != nil {
			return
		}
		_ = wsjson.Write(ctx, conn, map[string]string{"type": "auth_ok"})
		last := 0
		for {
			var m struct {
				ID             int            `json:"id"`
				Type           string         `json:"type"`
				Domain         string         `json:"domain"`
				Service        string         `json:"service"`
				Target         map[string]any `json:"target"`
				ReturnResponse bool           `json:"return_response"`
			}
			if wsjson.Read(ctx, conn, &m) != nil {
				return
			}
			if m.ID <= last {
				badIDs.Add(1)
				_ = wsjson.Write(ctx, conn, map[string]any{"id": m.ID, "type": "result", "success": false,
					"error": map[string]string{"code": "id_reuse", "message": "Identifier values have to increase."}})
				continue
			}
			last = m.ID
			res := map[string]any{"id": m.ID, "type": "result", "success": true}
			switch {
			case m.Type == "get_states":
				res["result"] = []map[string]any{{"entity_id": "weather.home", "state": "rainy",
					"attributes": map[string]any{"temperature": 6.0}}}
			case m.Type == "call_service" && m.Service == "get_forecasts" && m.ReturnResponse:
				if !forecastOK {
					res["success"] = false
					res["error"] = map[string]string{"code": "service_validation_error", "message": "Weather entity does not support 'daily' forecast"}
					break
				}
				res["result"] = map[string]any{"context": map[string]any{}, "response": map[string]any{
					m.Target["entity_id"].(string): map[string]any{"forecast": []map[string]any{
						{"datetime": "2026-11-16T11:00:00+00:00", "condition": "rainy", "temperature": 9.0, "templow": 4.0},
						{"datetime": "2026-11-17T11:00:00+00:00", "condition": "sunny", "temperature": 11.0, "templow": 5.0},
					}},
				}}
			}
			_ = wsjson.Write(ctx, conn, res)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func runClient(t *testing.T, url string) *Store {
	t.Helper()
	s := NewStore(SourceHA)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	c := &Client{URL: url, Token: "t", Store: s, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	go c.Run(ctx)
	return s
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestClientFetchesForecast(t *testing.T) {
	var bad atomic.Int32
	s := runClient(t, fakeHA(t, &bad, true).URL)
	waitUntil(t, "forecast", func() bool { return len(s.Forecast("weather.home")) == 2 })
	f := s.Forecast("weather.home")
	if f[1].Condition != "sunny" || f[1].Temperature == nil || *f[1].Temperature != 11 || *f[1].TempLow != 5 {
		t.Fatalf("unexpected forecast: %+v", f[1])
	}
	if n := bad.Load(); n != 0 {
		t.Fatalf("%d messages had non-increasing IDs", n)
	}
	if st := s.Status(); !st.Connected {
		t.Fatalf("not connected: %+v", st)
	}
}

func TestClientWithoutDailyForecastStaysConnected(t *testing.T) {
	var bad atomic.Int32
	s := runClient(t, fakeHA(t, &bad, false).URL)
	waitUntil(t, "connection", func() bool { return s.Status().Connected })
	time.Sleep(100 * time.Millisecond) // the failed forecast answer arrives
	if st := s.Status(); !st.Connected || st.Error != "" {
		t.Fatalf("a missing forecast must not drop the connection: %+v", st)
	}
	if f := s.Forecast("weather.home"); f != nil {
		t.Fatalf("expected no forecast, got %+v", f)
	}
}
