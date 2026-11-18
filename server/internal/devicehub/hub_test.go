package devicehub

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"homepaper/internal/frame"
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

// Changing the display area (D-021) wakes the session, and since most of
// the frame changes, the next frame is a full refresh.
func TestViewportChangeSendsFullFrame(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "homepaper.json"))
	if err != nil {
		t.Fatal(err)
	}
	dev, err := st.Create("lr", "Living room", store.ExampleLayout())
	if err != nil {
		t.Fatal(err)
	}
	settings := DefaultSettings()
	settings.Debounce, settings.MinInterval = 0, 0
	h := New(st, ha.NewStore("test"), slog.New(slog.NewTextHandler(io.Discard, nil)), settings)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1 << 20)
	hello := map[string]any{"type": "hello", "device_id": dev.ID, "token": dev.Token, "fw_version": "test",
		"panel": map[string]any{"model": "test", "width": 800, "height": 480, "format": "1bpp"}}
	if err := wsjson.Write(ctx, conn, hello); err != nil {
		t.Fatal(err)
	}
	fb := frame.New(800, 480)
	next := func() frame.Header {
		t.Helper()
		for {
			typ, buf, err := conn.Read(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if typ != websocket.MessageBinary {
				continue // welcome
			}
			hd, data, err := frame.Decode(buf, 1)
			if err != nil {
				t.Fatal(err)
			}
			fb.Apply(hd.Region, data)
			ack := map[string]any{"type": "ack", "seq": hd.Seq, "crc": fmt.Sprintf("%08x", fb.CRC32())}
			if err := wsjson.Write(ctx, conn, ack); err != nil {
				t.Fatal(err)
			}
			return hd
		}
	}
	if hd := next(); hd.Kind != frame.KindFull {
		t.Fatalf("first frame kind %d, want full", hd.Kind)
	}
	if _, err := st.Update(dev.ID, func(d *store.Device) { d.Viewport = store.Viewport{W: 600, H: 400} }); err != nil {
		t.Fatal(err)
	}
	h.Wake(dev.ID)
	if hd := next(); hd.Kind != frame.KindFull || hd.Region != (frame.Region{X: 0, Y: 0, W: 800, H: 480}) {
		t.Fatalf("after a viewport change: kind %d region %+v, want a full frame", hd.Kind, hd.Region)
	}
}
