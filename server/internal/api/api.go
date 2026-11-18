// Package api serves the editor's REST API, the UI event WebSocket and the
// embedded frontend.
package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"homepaper/internal/devicehub"
	"homepaper/internal/ha"
	"homepaper/internal/render"
	"homepaper/internal/store"
)

type API struct {
	Store    *store.Store
	States   *ha.Store
	Hub      *devicehub.Hub
	Log      *slog.Logger
	Web      fs.FS // built frontend (may lack index.html in dev)
	Location *time.Location
	// HA runs the entity source: the one global Home Assistant connection
	// from HA_URL/HA_TOKEN, or demo entities without HA_URL (D-016).
	HA             *ha.Manager
	HAURL, HAToken string
	// Auth is the login via Home Assistant; nil turns it off
	// (HOMEPAPER_AUTH=off, development and demo only).
	Auth *Auth
	// Version is the server's version (see VERSION), shown in /api/status.
	Version string

	events *broadcaster

	debounceMu sync.Mutex
	pending    map[string]*time.Timer
}

func (a *API) Handler() http.Handler {
	a.events = newBroadcaster()
	a.pending = map[string]*time.Timer{}
	a.Hub.OnStatus = func(id string) { a.events.send(map[string]string{"type": "device", "device": id}) }
	a.States.OnChange(a.onStates)

	mux := http.NewServeMux()
	// The display authenticates with its device token, not a login.
	mux.Handle("GET /ws/device", a.Hub)
	if a.Auth != nil {
		a.Auth.routes(mux)
		mux.HandleFunc("GET /auth/font/{name}", a.font) // for the login pages
	}

	api := http.NewServeMux()
	api.HandleFunc("GET /api/status", a.status)
	api.HandleFunc("GET /api/widgets", a.widgets)
	api.HandleFunc("GET /api/entities", a.entities)
	api.HandleFunc("GET /api/devices", a.listDevices)
	api.HandleFunc("POST /api/devices", a.createDevice)
	api.HandleFunc("GET /api/devices/{id}", a.getDevice)
	api.HandleFunc("PATCH /api/devices/{id}", a.patchDevice)
	api.HandleFunc("DELETE /api/devices/{id}", a.deleteDevice)
	api.HandleFunc("PUT /api/devices/{id}/layout", a.putLayout)
	api.HandleFunc("GET /api/devices/{id}/preview.png", a.preview)
	api.HandleFunc("GET /api/devices/{id}/live", func(w http.ResponseWriter, r *http.Request) {
		a.Hub.ServeViewer(w, r, r.PathValue("id"))
	})
	api.HandleFunc("GET /api/me", a.me)
	api.HandleFunc("GET /api/fonts/{name}", a.font)
	api.HandleFunc("GET /api/events", a.events.serve)
	api.Handle("/", a.frontend())
	if a.Auth != nil {
		mux.Handle("/", a.Auth.require(api))
	} else {
		mux.Handle("/", api)
	}
	return mux
}

// StartHA starts the entity source: Home Assistant from HA_URL/HA_TOKEN,
// or demo entities without them.
func (a *API) StartHA() {
	if a.HAURL == "" {
		a.Log.Info("no HA_URL: showing demo entities")
	} else {
		a.Log.Info("home assistant", "url", a.HAURL)
	}
	a.HA.Use(a.HAURL, a.HAToken)
}

// me tells the UI who is logged in.
func (a *API) me(w http.ResponseWriter, r *http.Request) {
	if a.Auth == nil {
		writeJSON(w, http.StatusOK, map[string]any{"auth": false})
		return
	}
	s, _ := a.Auth.user(r)
	writeJSON(w, http.StatusOK, map[string]any{"auth": true, "name": s.Name, "admin": s.Admin})
}

func (a *API) frontend() http.Handler {
	if _, err := fs.Stat(a.Web, "index.html"); err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "frontend not built: run `make web` (or use the Vite dev server on :5173)", http.StatusNotFound)
		})
	}
	files := http.FileServerFS(a.Web)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Single-page app: unknown paths get index.html.
		if r.URL.Path != "/" {
			if _, err := fs.Stat(a.Web, strings.TrimPrefix(r.URL.Path, "/")); err != nil {
				r.URL.Path = "/"
			}
		}
		files.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, store.ErrNotFound) {
		status = http.StatusNotFound
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func (a *API) status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, struct {
		ha.Status
		Version string `json:"version"`
	}{a.States.Status(), a.Version})
}

func (a *API) font(w http.ResponseWriter, r *http.Request) {
	data, ok := render.FontFile(r.PathValue("name"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "font/ttf")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(data)
}

func (a *API) widgets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, render.Kinds())
}

type entityDTO struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	State  string `json:"state"`
	Unit   string `json:"unit,omitempty"`
	Domain string `json:"domain"`
}

func (a *API) entities(w http.ResponseWriter, r *http.Request) {
	all := a.States.All()
	out := make([]entityDTO, 0, len(all))
	for _, e := range all {
		out = append(out, entityDTO{ID: e.ID, Name: e.Name(), State: e.State, Unit: e.Attr("unit_of_measurement"), Domain: e.Domain()})
	}
	writeJSON(w, http.StatusOK, out)
}

type deviceDTO struct {
	store.Device
	Status devicehub.Status `json:"status"`
}

func (a *API) dto(d store.Device) deviceDTO { return deviceDTO{Device: d, Status: a.Hub.Status(d.ID)} }

func (a *API) listDevices(w http.ResponseWriter, r *http.Request) {
	out := []deviceDTO{}
	for _, d := range a.Store.List() {
		out = append(out, a.dto(d))
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) getDevice(w http.ResponseWriter, r *http.Request) {
	d, err := a.Store.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a.dto(d))
}

func (a *API) createDevice(w http.ResponseWriter, r *http.Request) {
	var body struct{ ID, Name string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, err)
		return
	}
	d, err := a.Store.Create(strings.TrimSpace(body.ID), strings.TrimSpace(body.Name), a.starterLayout())
	if err != nil {
		writeErr(w, err)
		return
	}
	a.events.send(map[string]string{"type": "devices"})
	writeJSON(w, http.StatusCreated, a.dto(d))
}

// starterLayout is the example layout without the widgets whose entities
// the current source does not have. With demo data that is the full example;
// with a real Home Assistant it is usually the date and a heading, which
// beats a panel full of dashes.
func (a *API) starterLayout() store.Layout {
	l := store.ExampleLayout()
	kept := l.Widgets[:0]
	for _, wg := range l.Widgets {
		ok := true
		for id := range devicehub.ReferencedEntities(store.Layout{Widgets: []store.Widget{wg}}) {
			if _, found := a.States.Get(id); !found {
				ok = false
			}
		}
		if ok {
			kept = append(kept, wg)
		}
	}
	l.Widgets = kept
	return l
}

func (a *API) patchDevice(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name       *string `json:"name"`
		ResetToken bool    `json:"reset_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, err)
		return
	}
	id := r.PathValue("id")
	d, err := a.Store.Update(id, func(d *store.Device) {
		if body.Name != nil && strings.TrimSpace(*body.Name) != "" {
			d.Name = strings.TrimSpace(*body.Name)
		}
	})
	if err == nil && body.ResetToken {
		d, err = a.Store.ResetToken(id)
		a.Hub.Disconnect(id)
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	a.events.send(map[string]string{"type": "devices"})
	writeJSON(w, http.StatusOK, a.dto(d))
}

func (a *API) deleteDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := a.Store.Delete(id); err != nil {
		writeErr(w, err)
		return
	}
	a.Hub.Disconnect(id)
	a.events.send(map[string]string{"type": "devices"})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) putLayout(w http.ResponseWriter, r *http.Request) {
	var l store.Layout
	if err := json.NewDecoder(r.Body).Decode(&l); err != nil {
		writeErr(w, err)
		return
	}
	if err := validateLayout(&l); err != nil {
		writeErr(w, err)
		return
	}
	id := r.PathValue("id")
	d, err := a.Store.Update(id, func(d *store.Device) { d.Layout = l })
	if err != nil {
		writeErr(w, err)
		return
	}
	a.Hub.Wake(id)
	a.events.send(map[string]string{"type": "render", "device": id})
	writeJSON(w, http.StatusOK, a.dto(d))
}

func validateLayout(l *store.Layout) error {
	if l.Cols < 1 || l.Cols > 24 || l.Rows < 1 || l.Rows > 24 {
		return errors.New("grid must be between 1x1 and 24x24")
	}
	switch l.Dividers {
	case "", store.DividersRules, store.DividersFrames:
	default:
		return fmt.Errorf("dividers must be %q or %q", store.DividersRules, store.DividersFrames)
	}
	seen := map[string]bool{}
	for i := range l.Widgets {
		wg := &l.Widgets[i]
		if wg.ID == "" || seen[wg.ID] {
			return fmt.Errorf("widget %d: missing or duplicate id", i)
		}
		seen[wg.ID] = true
		if !render.HasKind(wg.Kind) {
			return fmt.Errorf("widget %s: unknown kind %q", wg.ID, wg.Kind)
		}
		if wg.W < 1 || wg.H < 1 || wg.X < 0 || wg.Y < 0 || wg.X+wg.W > l.Cols || wg.Y+wg.H > l.Rows {
			return fmt.Errorf("widget %s: outside the grid", wg.ID)
		}
		if wg.Options == nil {
			wg.Options = map[string]any{}
		}
		// The per-widget frame option was replaced by layout dividers (D-015).
		delete(wg.Options, "border")
	}
	return nil
}

func (a *API) preview(w http.ResponseWriter, r *http.Request) {
	d, err := a.Store.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	bmp := render.Render(d.Layout, d.Panel, a.States, time.Now().In(a.Location))
	var buf bytes.Buffer
	if err := png.Encode(&buf, bmp.ToGray()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(buf.Bytes())
}

// onStates tells editors to reload previews of devices using changed
// entities, debounced per device.
func (a *API) onStates(ids []string) {
	for _, d := range a.Store.List() {
		refs := devicehub.ReferencedEntities(d.Layout)
		hit := false
		for _, id := range ids {
			if refs[id] {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		id := d.ID
		a.debounceMu.Lock()
		if t, ok := a.pending[id]; ok {
			t.Stop()
		}
		a.pending[id] = time.AfterFunc(time.Second, func() {
			a.events.send(map[string]string{"type": "render", "device": id})
		})
		a.debounceMu.Unlock()
	}
}
