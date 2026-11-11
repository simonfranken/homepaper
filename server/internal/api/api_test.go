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
	"strings"
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

func TestHASettings(t *testing.T) {
	a, h := newTestAPI(t)
	good := fakeHA(t, "secret")

	code, out := call(t, h, "GET", "/api/settings/ha", nil)
	if code != 200 || out["origin"] != "none" || out["has_token"] != false {
		t.Fatalf("initial settings: %d %v", code, out)
	}

	// A wrong token is reported and nothing is saved.
	code, out = call(t, h, "PUT", "/api/settings/ha", map[string]string{"url": good.URL, "token": "wrong"})
	if code != 400 || !strings.Contains(out["error"].(string), "rejected") {
		t.Fatalf("wrong token: %d %v", code, out)
	}
	if a.Store.HA().URL != "" {
		t.Fatal("settings saved despite failed test")
	}

	// An unreachable address is reported as such.
	code, out = call(t, h, "PUT", "/api/settings/ha", map[string]string{"url": "http://127.0.0.1:1", "token": "secret"})
	if code != 400 || !strings.Contains(out["error"].(string), "cannot reach") {
		t.Fatalf("unreachable: %d %v", code, out)
	}

	// The right token saves, never echoes the token, and switches the source.
	code, out = call(t, h, "PUT", "/api/settings/ha", map[string]string{"url": good.URL + "/", "token": "secret"})
	if code != 200 || out["origin"] != "ui" || out["has_token"] != true || out["url"] != good.URL {
		t.Fatalf("save: %d %v", code, out)
	}
	if _, leaked := out["token"]; leaked {
		t.Fatal("token returned to the browser")
	}
	waitFor(t, "entities from fake HA", func() bool { _, ok := a.States.Get("sensor.real"); return ok })
	if st := a.States.Status(); st.Source != ha.SourceHA || !st.Connected {
		t.Fatalf("status after save: %+v", st)
	}

	// An empty token keeps the saved one for the same host...
	if code, out = call(t, h, "PUT", "/api/settings/ha", map[string]string{"url": good.URL}); code != 200 {
		t.Fatalf("keep token: %d %v", code, out)
	}
	// ...but is never sent over a less secure connection...
	httpsURL := strings.Replace(good.URL, "http://", "https://", 1)
	if err := a.Store.SetHA(store.HASettings{URL: httpsURL, Token: "secret"}); err != nil {
		t.Fatal(err)
	}
	code, out = call(t, h, "PUT", "/api/settings/ha", map[string]string{"url": good.URL})
	if code != 400 || !strings.Contains(out["error"].(string), "again") {
		t.Fatalf("token downgrade to http: %d %v", code, out)
	}
	if err := a.Store.SetHA(store.HASettings{URL: good.URL, Token: "secret"}); err != nil {
		t.Fatal(err)
	}
	// ...or to another host.
	other := fakeHA(t, "secret")
	code, out = call(t, h, "PUT", "/api/settings/ha", map[string]string{"url": other.URL})
	if code != 400 || !strings.Contains(out["error"].(string), "again") {
		t.Fatalf("token reuse on other host: %d %v", code, out)
	}

	// Clearing the address goes back to demo data.
	if code, out = call(t, h, "PUT", "/api/settings/ha", map[string]string{"url": ""}); code != 200 || out["origin"] != "none" {
		t.Fatalf("clear: %d %v", code, out)
	}
	waitFor(t, "demo entities", func() bool { _, ok := a.States.Get("weather.home"); return ok })
	if _, ok := a.States.Get("sensor.real"); ok {
		t.Fatal("HA entity survived the switch to demo data")
	}
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
	if code, out = call(t, h, "PUT", "/api/settings/ha", map[string]string{"url": good.URL, "token": "secret"}); code != 200 {
		t.Fatalf("save: %d %v", code, out)
	}
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
	bad := []store.Layout{
		{Cols: 0, Rows: 6},
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
