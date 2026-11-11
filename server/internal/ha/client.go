package ha

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
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

// NormalizeURL checks a Home Assistant base URL as typed by the user and
// returns it in canonical form. A missing scheme defaults to http.
func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("enter the Home Assistant address")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", errors.New("that address is not a valid URL, e.g. http://homeassistant.local:8123")
	}
	switch u.Scheme {
	case "http", "https", "ws", "wss":
	default:
		return "", errors.New("the address must start with http:// or https://")
	}
	u.User, u.RawQuery, u.Fragment = nil, "", ""
	return strings.TrimRight(u.String(), "/"), nil
}

// SameTarget reports whether b points at the same host and port as a, over
// a connection at least as secure. A saved token may be reused for b only
// then: never for another host, and never downgraded from https to http.
func SameTarget(a, b string) bool {
	ua, errA := url.Parse(a)
	ub, errB := url.Parse(b)
	if errA != nil || errB != nil || ua.Host == "" || !strings.EqualFold(ua.Host, ub.Host) {
		return false
	}
	secure := func(s string) bool { return s == "https" || s == "wss" }
	return !secure(ua.Scheme) || secure(ub.Scheme)
}

// dialReason turns a dial error into a short explanation for the UI.
func dialReason(err error) string {
	var dnsErr *net.DNSError
	var opErr *net.OpError
	msg := err.Error()
	switch {
	case errors.As(err, &dnsErr):
		return "the host name is unknown. Check the address."
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded):
		return "no answer. Check the address and that this server can reach it."
	case strings.Contains(msg, "connection refused"):
		return "nothing is listening there. Check the port (Home Assistant uses 8123 by default)."
	case strings.Contains(msg, "certificate"), strings.Contains(msg, "tls:"):
		return "the HTTPS certificate is not trusted (" + msg + ")."
	case strings.Contains(msg, "status code"), strings.Contains(msg, "404"):
		return "something answered, but it is not Home Assistant's WebSocket API. Check the address."
	case errors.As(err, &opErr):
		return opErr.Err.Error() + "."
	}
	return msg
}

// ErrAuth means Home Assistant was reached but refused the token.
var ErrAuth = errors.New("Home Assistant rejected the access token")

// Test connects and authenticates once, without touching any store. It
// tells apart "cannot reach" from "token rejected".
func Test(ctx context.Context, baseURL, token string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, err := (&Client{URL: baseURL, Token: token}).dial(ctx)
	if err != nil {
		return err
	}
	conn.Close(websocket.StatusNormalClosure, "")
	return nil
}

// dial opens the WebSocket and completes authentication.
func (c *Client) dial(ctx context.Context) (*websocket.Conn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(dialCtx, c.wsURL(), nil)
	if err != nil {
		return nil, fmt.Errorf("cannot reach Home Assistant at %s: %s", c.URL, dialReason(err))
	}
	conn.SetReadLimit(haReadLimit)

	var msg haMsg
	if err := wsjson.Read(dialCtx, conn, &msg); err != nil || msg.Type != "auth_required" {
		conn.CloseNow()
		return nil, fmt.Errorf("%s does not answer like Home Assistant", c.URL)
	}
	if err := wsjson.Write(dialCtx, conn, map[string]string{"type": "auth", "access_token": c.Token}); err != nil {
		conn.CloseNow()
		return nil, err
	}
	if err := wsjson.Read(dialCtx, conn, &msg); err != nil {
		conn.CloseNow()
		return nil, err
	}
	if msg.Type != "auth_ok" {
		conn.CloseNow()
		return nil, ErrAuth
	}
	return conn, nil
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
		c.Store.setError(err.Error())
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
	conn, err := c.dial(ctx)
	if err != nil {
		return err
	}
	defer conn.CloseNow()

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
