package api

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"homepaper/internal/frame"
	"homepaper/internal/ha"
)

// The editor's live preview gets frames like a display does (full first,
// then partial diffs, each acked), but without the display's rate limit,
// and without touching the display's status.
func TestLivePreview(t *testing.T) {
	a, h := newTestAPI(t)
	waitFor(t, "demo entities", func() bool { _, ok := a.States.Get("sensor.power_consumption"); return ok })
	if code, _ := call(t, h, "POST", "/api/devices", map[string]string{"id": "lr", "name": "Living room"}); code != 201 && code != 200 {
		t.Fatalf("create device: %d", code)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/api/devices/lr/live", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1 << 20)

	var welcome map[string]any
	if err := wsjson.Read(ctx, conn, &welcome); err != nil || welcome["type"] != "welcome" {
		t.Fatalf("welcome: %v %v", welcome, err)
	}
	var fb *frame.Bitmap
	next := func() frame.Header {
		t.Helper()
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		hd, px, err := frame.Decode(data)
		if err != nil {
			t.Fatal(err)
		}
		if fb == nil {
			fb = frame.New(hd.Region.W, hd.Region.H)
		}
		fb.Apply(hd.Region, px)
		if err := wsjson.Write(ctx, conn, map[string]any{"type": "ack", "seq": hd.Seq, "crc": fmt.Sprintf("%08x", fb.CRC32())}); err != nil {
			t.Fatal(err)
		}
		return hd
	}
	if hd := next(); hd.Kind != frame.KindFull {
		t.Fatalf("first frame is kind %d, want full", hd.Kind)
	}

	e, _ := a.States.Get("sensor.power_consumption")
	e.State = "9999"
	start := time.Now()
	a.States.Set(e.ID, &ha.Entity{ID: e.ID, State: e.State, Attributes: e.Attributes})
	hd := next()
	if hd.Kind != frame.KindPartial {
		t.Errorf("update came as kind %d, want partial", hd.Kind)
	}
	// The display waits 1 s debounce and keeps 3 s between frames; the
	// editor does neither.
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Errorf("update took %v", d)
	}
	if st := a.Hub.Status("lr"); st.Connected || st.FramesSent != 0 {
		t.Errorf("the viewer changed the display's status: %+v", st)
	}
}
