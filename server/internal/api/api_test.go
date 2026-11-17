package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"homepaper/internal/devicehub"
	"homepaper/internal/ha"
	"homepaper/internal/store"
)

// fakeHA speaks just enough of the Home Assistant WebSocket API: it accepts
// one token and answers get_states with a single entity.
func fakeHA(t *testing.T, token string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/websocket" {
			http.NotFound(w, r)
			return
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx := r.Context()
		_ = wsjson.Write(ctx, conn, map[string]string{"type": "auth_required"})
		var auth struct {
			Token string `json:"access_token"`
		}
		if wsjson.Read(ctx, conn, &auth) != nil {
			return
		}
		if auth.Token != token {
			_ = wsjson.Write(ctx, conn, map[string]string{"type": "auth_invalid"})
			return
		}
		_ = wsjson.Write(ctx, conn, map[string]string{"type": "auth_ok"})
		for {
			var m struct {
				ID   int    `json:"id"`
				Type string `json:"type"`
			}
			if wsjson.Read(ctx, conn, &m) != nil {
				return
			}
			res := map[string]any{"id": m.ID, "type": "result", "success": true}
			if m.Type == "get_states" {
				res["result"] = []map[string]any{{"entity_id": "sensor.real", "state": "1"}}
			}
			_ = wsjson.Write(ctx, conn, res)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTestAPI(t *testing.T) (*API, http.Handler) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(filepath.Join(t.TempDir(), "homepaper.json"))
	if err != nil {
		t.Fatal(err)
	}
	m := ha.NewManager(ctx, log)
	a := &API{Store: st, States: m.Store, Hub: devicehub.New(st, m.Store, log, devicehub.DefaultSettings()),
		Log: log, Web: fstest.MapFS{}, Location: time.UTC, HA: m}
	h := a.Handler()
	a.StartHA()
	return a, h
}

func call(t *testing.T, h http.Handler, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, &buf))
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestStarterLayoutKeepsOnlyBoundWidgets(t *testing.T) {
	a, h := newTestAPI(t)
	waitFor(t, "demo entities", func() bool { _, ok := a.States.Get("weather.home"); return ok })

	code, out := call(t, h, "POST", "/api/devices", map[string]string{"id": "demo", "name": "Demo"})
	if code != 201 {
		t.Fatalf("create: %d %v", code, out)
	}
	full := len(store.ExampleLayout().Widgets)
	if n := len(out["layout"].(map[string]any)["widgets"].([]any)); n != full {
		t.Fatalf("demo layout has %d widgets, want %d", n, full)
	}

	good := fakeHA(t, "secret")
	a.HAURL, a.HAToken = good.URL, "secret"
	a.StartHA()
	waitFor(t, "entities from fake HA", func() bool { _, ok := a.States.Get("sensor.real"); return ok })
	_, out = call(t, h, "POST", "/api/devices", map[string]string{"id": "real", "name": "Real"})
	for _, w := range out["layout"].(map[string]any)["widgets"].([]any) {
		kind := w.(map[string]any)["kind"]
		if kind != "clock" && kind != "text" {
			t.Fatalf("starter layout kept a %v widget bound to a demo entity", kind)
		}
	}
}

func TestValidateLayout(t *testing.T) {
	ok := store.Layout{Cols: 8, Rows: 6, Widgets: []store.Widget{{ID: "a", Kind: "clock", W: 2, H: 1}}}
	if err := validateLayout(&ok); err != nil {
		t.Fatal(err)
	}
	if ok.Widgets[0].Options == nil {
		t.Fatal("nil options not normalised")
	}
	// The old per-widget frame option is dropped; layout dividers replace it.
	old := store.Layout{Cols: 8, Rows: 6, Dividers: store.DividersFrames, Widgets: []store.Widget{
		{ID: "a", Kind: "clock", W: 2, H: 1, Options: map[string]any{"border": false, "format": "time"}}}}
	if err := validateLayout(&old); err != nil {
		t.Fatal(err)
	}
	if _, has := old.Widgets[0].Options["border"]; has || old.Widgets[0].Options["format"] != "time" {
		t.Fatalf("options not cleaned: %v", old.Widgets[0].Options)
	}
	bad := []store.Layout{
		{Cols: 0, Rows: 6},
		{Cols: 8, Rows: 6, Dividers: "boxes"},
		{Cols: 8, Rows: 6, Widgets: []store.Widget{{ID: "a", Kind: "nope", W: 1, H: 1}}},
		{Cols: 8, Rows: 6, Widgets: []store.Widget{{ID: "a", Kind: "clock", X: 7, W: 2, H: 1}}},
		{Cols: 8, Rows: 6, Widgets: []store.Widget{{ID: "a", Kind: "clock", W: 1, H: 1}, {ID: "a", Kind: "clock", X: 1, W: 1, H: 1}}},
	}
	for i, l := range bad {
		if validateLayout(&l) == nil {
			t.Errorf("layout %d accepted", i)
		}
	}
}
