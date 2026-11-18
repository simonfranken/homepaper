// Command fakedevice speaks the device protocol like the ESP32 firmware does,
// for testing the server without hardware. It keeps a local framebuffer,
// applies received frames, acks them with the framebuffer's CRC32 and can
// write the framebuffer to a PNG after every frame.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image/png"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/coder/websocket"

	"homepaper/internal/frame"
)

func main() {
	url := flag.String("url", "ws://localhost:8080/ws/device", "server device WebSocket URL")
	id := flag.String("id", "", "device id")
	token := flag.String("token", "", "device token")
	out := flag.String("png", "", "write the framebuffer to this PNG after each frame")
	corrupt := flag.Bool("corrupt", false, "flip a bit after the first frame to test checksum mismatch handling")
	badCRC := flag.Bool("badcrc", false, "report a wrong checksum for every frame (systematic mismatch)")
	format := flag.String("format", frame.Format2bpp, "pixel format to report: 1bpp or 2bpp")
	flag.Parse()
	if *id == "" || *token == "" {
		log.Fatal("-id and -token are required")
	}
	depth := frame.DepthOf(*format)
	if depth == 0 {
		log.Fatalf("unknown format %q", *format)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	conn, _, err := websocket.Dial(ctx, *url, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1 << 20)

	send := func(v any) {
		data, _ := json.Marshal(v)
		if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
			log.Fatal(err)
		}
	}
	send(map[string]any{
		"type": "hello", "device_id": *id, "token": *token, "fw_version": "fake-0.1.0",
		"panel":        map[string]any{"model": "waveshare-7in5-v2", "width": 800, "height": 480, "format": *format},
		"capabilities": []string{"partial"},
	})
	fb := frame.NewDepth(800, 480, depth)
	start := time.Now()
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for range t.C {
			send(map[string]any{"type": "status", "rssi": -55, "free_heap": 123456, "uptime_s": int(time.Since(start).Seconds())})
		}
	}()

	frames := 0
	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			log.Fatal(err)
		}
		if typ == websocket.MessageText {
			log.Printf("server: %s", data)
			continue
		}
		h, payload, err := frame.Decode(data, depth)
		if err != nil {
			log.Printf("bad frame: %v", err)
			continue
		}
		fb.Apply(h.Region, payload)
		frames++
		if *corrupt && frames == 1 {
			fb.Pix[0] ^= 0x80
		}
		kind := map[uint8]string{frame.KindFull: "full", frame.KindPartial: "partial"}[h.Kind]
		sum := fb.CRC32()
		if *badCRC {
			sum ^= 1
		}
		crc := fmt.Sprintf("%08x", sum)
		grey := ""
		if fb.HasGrey() {
			grey = " grey"
		}
		log.Printf("frame %d %s%s region=%+v bytes=%d crc=%s", h.Seq, kind, grey, h.Region, len(data), crc)
		if *out != "" {
			if f, err := os.Create(*out); err == nil {
				_ = png.Encode(f, fb.ToGray())
				f.Close()
			}
		}
		send(map[string]any{"type": "ack", "seq": h.Seq, "crc": crc})
	}
}
