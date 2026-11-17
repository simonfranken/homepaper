package devicehub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"homepaper/internal/frame"
	"homepaper/internal/render"
)

// maxRegions and maxPartialShare decide when a diff is sent as a full frame.
const (
	maxRegions      = 6
	maxPartialShare = 0.5
)

type session struct {
	hub      *Hub
	conn     *websocket.Conn
	deviceID string
	refs     atomic.Value // map[string]bool
	wakeCh   chan struct{}
	acks     chan inbound

	shown    *frame.Bitmap // what the device displays, as confirmed by acks
	seq      uint32
	partials int
	lastFull time.Time
	lastSend time.Time
	// fullMismatch is set when even a full frame's checksum did not match,
	// i.e. the mismatch is systematic; partial mismatches then stop
	// triggering resyncs until a checksum matches again.
	fullMismatch bool
	// viewer marks an editor tab watching the device's frames (Hub.ServeViewer).
	// It gets the same frames on the same schedule, but leaves the device's
	// status alone.
	viewer bool
}

// status updates the device's status, unless this is a viewer or a session
// that a newer connection of the device has replaced.
func (s *session) status(fn func(st *Status)) {
	if !s.viewer {
		s.hub.updateStatusIf(s.deviceID, s, fn)
	}
}

func (s *session) wake() {
	select {
	case s.wakeCh <- struct{}{}:
	default:
	}
}

func (s *session) uses(ids []string) bool {
	refs, _ := s.refs.Load().(map[string]bool)
	for _, id := range ids {
		if refs[id] {
			return true
		}
	}
	return false
}

func (s *session) run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go s.readLoop(ctx, cancel)

	// Ticks on wall-clock minute boundaries keep clock widgets current;
	// unchanged frames are not sent. A plain time.Ticker would be anchored to
	// the connect time and let the clock lag by up to a minute.
	untilNextMinute := func() time.Duration {
		now := time.Now()
		return now.Truncate(time.Minute).Add(time.Minute).Sub(now)
	}
	minute := time.NewTimer(untilNextMinute())
	defer minute.Stop()

	s.wake()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.wakeCh:
		case <-minute.C:
			minute.Reset(untilNextMinute())
		}
		// Debounce: collect changes that arrive close together. Viewers (the
		// editor) are not rate limited: they render at once, held back only
		// by their own acks.
		debounce, minInterval := s.hub.Settings.Debounce, s.hub.Settings.MinInterval
		if s.viewer {
			debounce, minInterval = 0, 0
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(debounce):
		}
		if wait := minInterval - time.Since(s.lastSend); wait > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
		}
		// Drain a wake that arrived while waiting; this render covers it.
		select {
		case <-s.wakeCh:
		default:
		}
		if err := s.update(ctx); err != nil {
			s.hub.Log.Warn("device update failed", "device", s.deviceID, "err", err)
			s.status(func(st *Status) { st.LastError = err.Error() })
			s.conn.Close(websocket.StatusInternalError, "update failed")
			return
		}
	}
}

// update renders the layout and sends whatever changed.
func (s *session) update(ctx context.Context) error {
	dev, err := s.hub.Store.Get(s.deviceID)
	if err != nil {
		return err
	}
	s.refs.Store(ReferencedEntities(dev.Layout))
	next := render.Render(dev.Layout, dev.Panel, s.hub.States, time.Now().In(s.hub.Settings.Location))

	regions := frame.Diff(s.shown, next)
	if len(regions) == 0 {
		return nil
	}
	area := 0
	for _, r := range regions {
		area += r.Area()
	}
	st := s.hub.Settings
	full := s.shown == nil ||
		len(regions) > maxRegions ||
		float64(area) > maxPartialShare*float64(next.W*next.H) ||
		s.partials >= st.FullRefreshEvery ||
		time.Since(s.lastFull) > st.FullRefreshAfter
	if full {
		return s.send(ctx, frame.KindFull, next.Full(), next)
	}
	for _, r := range regions {
		if err := s.send(ctx, frame.KindPartial, r, next); err != nil {
			return err
		}
		if s.shown == nil {
			// Mismatch: the remaining regions are covered by the full resync.
			return nil
		}
	}
	return nil
}

// send transmits one region of next and waits for the device's ack.
func (s *session) send(ctx context.Context, kind uint8, r frame.Region, next *frame.Bitmap) error {
	s.seq++
	seq := s.seq
	buf := frame.Encode(kind, seq, r, next.Extract(r))

	expected := next
	if kind == frame.KindPartial {
		expected = s.shown.Clone()
		expected.Apply(r, next.Extract(r))
	}

	wctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	err := s.conn.Write(wctx, websocket.MessageBinary, buf)
	cancel()
	if err != nil {
		return err
	}
	s.lastSend = time.Now()
	kindName := "partial"
	if kind == frame.KindFull {
		kindName = "full"
		s.partials = 0
		s.lastFull = time.Now()
	} else {
		s.partials++
	}
	s.status(func(st *Status) {
		st.FramesSent++
		st.BytesSent += len(buf)
		st.LastKind = kindName
		st.LastSeq = seq
	})

	timeout := time.NewTimer(s.hub.Settings.AckTimeout)
	defer timeout.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout.C:
			return fmt.Errorf("no ack for frame %d", seq)
		case a := <-s.acks:
			if a.Seq != seq {
				continue
			}
			want := fmt.Sprintf("%08x", expected.CRC32())
			inSync := a.CRC == want
			s.status(func(st *Status) {
				st.LastAckSeq = a.Seq
				st.LastAckAt = time.Now()
				st.InSync = inSync
				if !inSync {
					st.LastError = fmt.Sprintf("checksum mismatch on frame %d: device %s, server %s", seq, a.CRC, want)
				}
			})
			if inSync {
				s.fullMismatch = false
			} else if kind == frame.KindFull {
				s.fullMismatch = true
			}
			if !inSync && kind == frame.KindPartial && !s.fullMismatch {
				// A partial update went wrong: resync once with a full frame.
				s.shown = nil
				s.wake()
				return nil
			}
			// In sync, or a full frame still mismatched. Resending full frames
			// would loop (and flash the panel) if the mismatch is systematic,
			// e.g. a checksum bug, so assume the device shows the frame and
			// leave the device marked out of sync in the UI.
			s.shown = expected
			return nil
		}
	}
}

func (s *session) readLoop(ctx context.Context, cancel context.CancelFunc) {
	defer cancel()
	for {
		typ, data, err := s.conn.Read(ctx)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				s.hub.Log.Debug("device read ended", "device", s.deviceID, "err", err)
			}
			return
		}
		if typ != websocket.MessageText {
			continue
		}
		var m inbound
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		switch m.Type {
		case "ack":
			select {
			case s.acks <- m:
			default:
			}
		case "status":
			s.status(func(st *Status) {
				st.RSSI, st.FreeHeap, st.Uptime = m.RSSI, m.FreeHeap, m.Uptime
			})
		case "log":
			s.hub.Log.Info("device log", "device", s.deviceID, "msg", m.Message)
		}
	}
}
