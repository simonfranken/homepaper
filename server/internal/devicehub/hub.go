// Package devicehub serves the device WebSocket (docs/device-protocol.md):
// authenticates devices, renders their layout and pushes full or partial
// frames, gated by acks and rate limits.
package devicehub

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"homepaper/internal/frame"
	"homepaper/internal/ha"
	"homepaper/internal/store"
)

// Settings control refresh behaviour (docs/rendering.md, update strategy).
type Settings struct {
	Debounce         time.Duration // wait this long after a change before rendering
	MinInterval      time.Duration // minimum time between two frames to a device
	FullRefreshEvery int           // partial updates before a forced full refresh
	FullRefreshAfter time.Duration // max time between full refreshes
	AckTimeout       time.Duration
	Location         *time.Location
}

func DefaultSettings() Settings {
	return Settings{
		Debounce:         time.Second,
		MinInterval:      3 * time.Second,
		FullRefreshEvery: 20,
		FullRefreshAfter: time.Hour,
		AckTimeout:       60 * time.Second,
		Location:         time.Local,
	}
}

// Status is what the UI shows about a device connection.
type Status struct {
	Connected   bool      `json:"connected"`
	ConnectedAt time.Time `json:"connected_at,omitempty"`
	Remote      string    `json:"remote,omitempty"`
	Firmware    string    `json:"firmware,omitempty"`
	Panel       string    `json:"panel,omitempty"`
	FramesSent  int       `json:"frames_sent"`
	BytesSent   int       `json:"bytes_sent"`
	LastKind    string    `json:"last_kind,omitempty"`
	LastSeq     uint32    `json:"last_seq"`
	LastAckSeq  uint32    `json:"last_ack_seq"`
	LastAckAt   time.Time `json:"last_ack_at,omitempty"`
	InSync      bool      `json:"in_sync"`
	RSSI        int       `json:"rssi,omitempty"`
	FreeHeap    int       `json:"free_heap,omitempty"`
	Uptime      int       `json:"uptime_s,omitempty"`
	LastError   string    `json:"last_error,omitempty"`
}

type Hub struct {
	Store    *store.Store
	States   *ha.Store
	Log      *slog.Logger
	Settings Settings
	// OnStatus is called whenever a device's status changes.
	OnStatus func(deviceID string)

	mu       sync.Mutex
	sessions map[string]*session
	viewers  map[*session]bool
	status   map[string]*Status
}

func New(st *store.Store, states *ha.Store, log *slog.Logger, settings Settings) *Hub {
	h := &Hub{Store: st, States: states, Log: log, Settings: settings, sessions: map[string]*session{}, viewers: map[*session]bool{}, status: map[string]*Status{}}
	states.OnChange(h.onStates)
	return h
}

// Status returns a copy of a device's connection status.
func (h *Hub) Status(id string) Status {
	h.mu.Lock()
	defer h.mu.Unlock()
	if s, ok := h.status[id]; ok {
		return *s
	}
	return Status{}
}

// updateStatusIf updates a device's status if sess is the device's current
// session. A session that a reconnect replaced (e.g. the half-open
// connection left behind by a device reset) must not touch the status of the
// one that replaced it.
func (h *Hub) updateStatusIf(id string, sess *session, fn func(s *Status)) {
	h.mu.Lock()
	if h.sessions[id] != sess {
		h.mu.Unlock()
		return
	}
	s, ok := h.status[id]
	if !ok {
		s = &Status{}
		h.status[id] = s
	}
	fn(s)
	h.mu.Unlock()
	if h.OnStatus != nil {
		h.OnStatus(id)
	}
}

// Wake asks the session of a device (if connected) and its viewers to
// re-render.
func (h *Hub) Wake(id string) {
	for _, s := range h.sessionsOf(id) {
		s.wake()
	}
}

// sessionsOf returns the device's session (if connected) and its viewers;
// all of them when id is empty.
func (h *Hub) sessionsOf(id string) []*session {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []*session
	for devID, s := range h.sessions {
		if id == "" || devID == id {
			out = append(out, s)
		}
	}
	for s := range h.viewers {
		if id == "" || s.deviceID == id {
			out = append(out, s)
		}
	}
	return out
}

// Disconnect closes a device's session, e.g. after it was deleted.
func (h *Hub) Disconnect(id string) {
	for _, s := range h.sessionsOf(id) {
		s.conn.Close(websocket.StatusPolicyViolation, "device removed")
	}
}

func (h *Hub) onStates(ids []string) {
	for _, s := range h.sessionsOf("") {
		if s.uses(ids) {
			s.wake()
		}
	}
}

// ReferencedEntities lists the entity IDs a layout depends on.
func ReferencedEntities(l store.Layout) map[string]bool {
	refs := map[string]bool{}
	for _, w := range l.Widgets {
		for key, v := range w.Options {
			s, ok := v.(string)
			if !ok || !(key == "entity" || key == "entities" || strings.HasSuffix(key, "_entity")) {
				continue
			}
			for _, id := range strings.Split(s, ",") {
				if id = strings.TrimSpace(id); id != "" {
					refs[id] = true
				}
			}
		}
	}
	return refs
}

type hello struct {
	Type      string      `json:"type"`
	DeviceID  string      `json:"device_id"`
	Token     string      `json:"token"`
	Firmware  string      `json:"fw_version"`
	Panel     store.Panel `json:"panel"`
	Abilities []string    `json:"capabilities"`
}

type inbound struct {
	Type     string `json:"type"`
	Seq      uint32 `json:"seq"`
	CRC      string `json:"crc"`
	RSSI     int    `json:"rssi"`
	FreeHeap int    `json:"free_heap"`
	Uptime   int    `json:"uptime_s"`
	Message  string `json:"message"`
}

// ServeHTTP handles GET /ws/device.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	ctx := r.Context()

	hctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	_, data, err := conn.Read(hctx)
	cancel()
	if err != nil {
		return
	}
	var hl hello
	if json.Unmarshal(data, &hl) != nil || hl.Type != "hello" {
		h.reject(ctx, conn, "expected hello")
		return
	}
	dev, err := h.Store.Get(hl.DeviceID)
	if err != nil || subtle.ConstantTimeCompare([]byte(dev.Token), []byte(hl.Token)) != 1 {
		h.Log.Warn("device rejected", "device", hl.DeviceID, "remote", r.RemoteAddr)
		h.reject(ctx, conn, "unknown device or wrong token")
		return
	}
	if hl.Panel.Width%8 != 0 || hl.Panel.Width <= 0 || hl.Panel.Height <= 0 || frame.DepthOf(hl.Panel.Format) == 0 {
		h.reject(ctx, conn, "unsupported panel")
		return
	}
	dev, _ = h.Store.Update(dev.ID, func(d *store.Device) { d.Panel = hl.Panel; d.LastSeen = time.Now() })

	s := &session{hub: h, conn: conn, deviceID: dev.ID, wakeCh: make(chan struct{}, 1), acks: make(chan inbound, 4)}
	s.refs.Store(ReferencedEntities(dev.Layout))

	h.mu.Lock()
	old := h.sessions[dev.ID]
	h.sessions[dev.ID] = s
	h.mu.Unlock()
	if old != nil {
		// After a device reset the old connection is usually half-open, and a
		// close handshake would wait for a peer that is gone.
		h.Log.Info("device reconnected, closing its old connection", "device", dev.ID)
		old.conn.CloseNow()
	}
	defer func() {
		s.status(func(st *Status) { st.Connected = false })
		h.mu.Lock()
		current := h.sessions[dev.ID] == s
		if current {
			delete(h.sessions, dev.ID)
		}
		h.mu.Unlock()
		if current {
			h.Log.Info("device disconnected", "device", dev.ID)
		}
	}()

	s.status(func(st *Status) {
		*st = Status{Connected: true, ConnectedAt: time.Now(), Remote: r.RemoteAddr, Firmware: hl.Firmware,
			Panel: fmt.Sprintf("%s %dx%d %s", hl.Panel.Model, hl.Panel.Width, hl.Panel.Height, hl.Panel.Format)}
	})
	h.Log.Info("device connected", "device", dev.ID, "remote", r.RemoteAddr, "fw", hl.Firmware)

	if err := writeJSON(ctx, conn, map[string]any{"type": "welcome", "full_refresh_every": h.Settings.FullRefreshEvery}); err != nil {
		return
	}
	s.run(ctx)
}

func (h *Hub) reject(ctx context.Context, conn *websocket.Conn, msg string) {
	_ = writeJSON(ctx, conn, map[string]string{"type": "error", "message": msg})
	conn.Close(websocket.StatusPolicyViolation, msg)
}

func writeJSON(ctx context.Context, conn *websocket.Conn, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return conn.Write(wctx, websocket.MessageText, data)
}

// ServeViewer handles GET /api/devices/{id}/live: an editor tab becomes a
// virtual copy of the display. It speaks the device protocol from the
// welcome on (binary frames in, acks out) and gets frames the way the
// display does: pushed on every change, diffed into partial and full frames,
// each waiting for its ack. Unlike the display it is not rate limited (no
// debounce, no minimum interval). It does not replace the display's session
// or change its status. The UI's auth protects it.
func (h *Hub) ServeViewer(w http.ResponseWriter, r *http.Request, id string) {
	dev, err := h.Store.Get(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	ctx := r.Context()

	s := &session{hub: h, conn: conn, deviceID: dev.ID, viewer: true, wakeCh: make(chan struct{}, 1), acks: make(chan inbound, 4)}
	s.refs.Store(ReferencedEntities(dev.Layout))
	h.mu.Lock()
	h.viewers[s] = true
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.viewers, s)
		h.mu.Unlock()
	}()

	if err := writeJSON(ctx, conn, map[string]any{"type": "welcome", "full_refresh_every": h.Settings.FullRefreshEvery,
		"panel": dev.Panel}); err != nil {
		return
	}
	s.run(ctx)
}
