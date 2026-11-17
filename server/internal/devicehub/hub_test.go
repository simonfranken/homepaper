package devicehub

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"homepaper/internal/ha"
	"homepaper/internal/store"
)

// After a device reset the old connection is still open on the server when
// the device reconnects. Closing the replaced session must not mark the
// device offline.
func TestReconnectKeepsDeviceOnline(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "homepaper.json"))
	if err != nil {
		t.Fatal(err)
	}
	dev, err := st.Create("lr", "Living room", store.Layout{})
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, ha.NewStore("test"), slog.New(slog.NewTextHandler(io.Discard, nil)), DefaultSettings())
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	connect := func() *websocket.Conn {
		t.Helper()
		conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.CloseNow() })
		conn.SetReadLimit(1 << 20)
		hello := map[string]any{"type": "hello", "device_id": dev.ID, "token": dev.Token, "fw_version": "test",
			"panel": map[string]any{"model": "test", "width": 800, "height": 480, "format": "1bpp"}}
		if err := wsjson.Write(ctx, conn, hello); err != nil {
			t.Fatal(err)
		}
		var welcome map[string]any
		if err := wsjson.Read(ctx, conn, &welcome); err != nil || welcome["type"] != "welcome" {
			t.Fatalf("welcome: %v %v", welcome, err)
		}
		return conn
	}

	old := connect()
	connect()
	// The server closes the replaced connection; wait until it is gone.
	for {
		if _, _, err := old.Read(ctx); err != nil {
			break
		}
	}
	time.Sleep(200 * time.Millisecond) // let the old session's cleanup run
	if st := h.Status(dev.ID); !st.Connected {
		t.Fatalf("device shown offline after reconnect: %+v", st)
	}
}
