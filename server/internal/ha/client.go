package ha

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// Client connects to Home Assistant's WebSocket API, loads all states and
// follows state_changed events. It reconnects with backoff until ctx ends.
type Client struct {
	URL   string // e.g. http://homeassistant.local:8123
	Token string // long-lived access token
	Store *Store
	Log   *slog.Logger
}

// get_states responses for a typical install are hundreds of KB.
const haReadLimit = 32 << 20

func (c *Client) wsURL() string {
	u := strings.TrimRight(c.URL, "/")
	u = strings.Replace(u, "https://", "wss://", 1)
	u = strings.Replace(u, "http://", "ws://", 1)
	if !strings.HasSuffix(u, "/api/websocket") {
		u += "/api/websocket"
	}
	return u
}

func (c *Client) Run(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		start := time.Now()
		err := c.session(ctx)
		c.Store.setConnected(false)
		if ctx.Err() != nil {
			return
		}
		if time.Since(start) > time.Minute {
			backoff = time.Second
		}
		c.Log.Warn("home assistant connection lost", "err", err, "retry_in", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, time.Minute)
	}
}

type haMsg struct {
	ID      int             `json:"id,omitempty"`
	Type    string          `json:"type"`
	Success *bool           `json:"success,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Event   *struct {
		Data struct {
			EntityID string  `json:"entity_id"`
			NewState *Entity `json:"new_state"`
		} `json:"data"`
	} `json:"event,omitempty"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *Client) session(ctx context.Context) error {
	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(dialCtx, c.wsURL(), nil)
	if err != nil {
		return err
	}
	defer conn.CloseNow()
	conn.SetReadLimit(haReadLimit)

	var msg haMsg
	if err := wsjson.Read(ctx, conn, &msg); err != nil || msg.Type != "auth_required" {
		return fmt.Errorf("expected auth_required: %v %s", err, msg.Type)
	}
	if err := wsjson.Write(ctx, conn, map[string]string{"type": "auth", "access_token": c.Token}); err != nil {
		return err
	}
	if err := wsjson.Read(ctx, conn, &msg); err != nil {
		return err
	}
	if msg.Type != "auth_ok" {
		return fmt.Errorf("authentication failed (%s)", msg.Type)
	}

	// HA requires message IDs to increase within a connection. Subscribe
	// first so no state_changed event between snapshot and subscription is lost.
	const idSubscribe, idStates = 1, 2
	if err := wsjson.Write(ctx, conn, map[string]any{"id": idSubscribe, "type": "subscribe_events", "event_type": "state_changed"}); err != nil {
		return err
	}
	if err := wsjson.Write(ctx, conn, map[string]any{"id": idStates, "type": "get_states"}); err != nil {
		return err
	}
	c.Log.Info("connected to home assistant", "url", c.URL)

	for {
		var m haMsg
		if err := wsjson.Read(ctx, conn, &m); err != nil {
			return err
		}
		switch {
		case m.Type == "result" && m.ID == idStates:
			if m.Success != nil && !*m.Success {
				return fmt.Errorf("get_states failed: %v", m.Error)
			}
			var states []Entity
			if err := json.Unmarshal(m.Result, &states); err != nil {
				return fmt.Errorf("decode states: %w", err)
			}
			c.Store.Replace(states)
			c.Store.setConnected(true)
			c.Log.Info("loaded entity states", "count", len(states))
		case m.Type == "result" && m.Success != nil && !*m.Success:
			return fmt.Errorf("request %d failed: %v", m.ID, m.Error)
		case m.Type == "event" && m.Event != nil:
			d := m.Event.Data
			c.Store.Set(d.EntityID, d.NewState)
		}
	}
}
