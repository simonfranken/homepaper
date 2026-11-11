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
	status   map[string]*Status
}

func New(st *store.Store, states *ha.Store, log *slog.Logger, settings Settings) *Hub {
	h := &Hub{Store: st, States: states, Log: log, Settings: settings, sessions: map[string]*session{}, status: map[string]*Status{}}
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

func (h *Hub) updateStatus(id string, fn func(s *Status)) {
	h.mu.Lock()
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

// Wake asks the session of a device (if connected) to re-render.
func (h *Hub) Wake(id string) {
	h.mu.Lock()
	s := h.sessions[id]
	h.mu.Unlock()
	if s != nil {
		s.wake()
	}
}

// Disconnect closes a device's session, e.g. after it was deleted.
func (h *Hub) Disconnect(id string) {
	h.mu.Lock()
	s := h.sessions[id]
	h.mu.Unlock()
	if s != nil {
		s.conn.Close(websocket.StatusPolicyViolation, "device removed")
	}
}

func (h *Hub) onStates(ids []string) {
	h.mu.Lock()
	sessions := make([]*session, 0, len(h.sessions))
	for _, s := range h.sessions {
		sessions = append(sessions, s)
	}
	h.mu.Unlock()
	for _, s := range sessions {
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
	if hl.Panel.Width%8 != 0 || hl.Panel.Width <= 0 || hl.Panel.Height <= 0 || hl.Panel.Format != "1bpp" {
		h.reject(ctx, conn, "unsupported panel")
		return
	}
	dev, _ = h.Store.Update(dev.ID, func(d *store.Device) { d.Panel = hl.Panel; d.LastSeen = time.Now() })

	s := &session{hub: h, conn: conn, deviceID: dev.ID, wakeCh: make(chan struct{}, 1), acks: make(chan inbound, 4)}
	s.refs.Store(ReferencedEntities(dev.Layout))

	h.mu.Lock()
	if old := h.sessions[dev.ID]; old != nil {
		old.conn.Close(websocket.StatusGoingAway, "replaced by new connection")
	}
	h.sessions[dev.ID] = s
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		if h.sessions[dev.ID] == s {
			delete(h.sessions, dev.ID)
		}
		h.mu.Unlock()
		h.updateStatus(dev.ID, func(st *Status) { st.Connected = false })
		h.Log.Info("device disconnected", "device", dev.ID)
	}()

	h.updateStatus(dev.ID, func(st *Status) {
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
