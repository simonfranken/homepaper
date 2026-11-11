package ha

import (
	"context"
	"log/slog"
	"sync"
)

// Manager runs the entity source behind a Store and can swap it while the
// server runs: the demo source, or a Home Assistant connection. Changing the
// HA settings in the UI therefore needs no restart (D-013).
type Manager struct {
	Store *Store
	Log   *slog.Logger

	mu     sync.Mutex
	parent context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

func NewManager(ctx context.Context, log *slog.Logger) *Manager {
	return &Manager{Store: NewStore(SourceDemo), Log: log, parent: ctx}
}

// Use stops the running source and starts a new one: the demo source when
// url is empty, a Home Assistant client otherwise.
func (m *Manager) Use(url, token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
		<-m.done
	}
	ctx, cancel := context.WithCancel(m.parent)
	done := make(chan struct{})
	m.cancel, m.done = cancel, done

	if url == "" {
		m.Store.reset(SourceDemo)
		go func() {
			defer close(done)
			RunDemo(ctx, m.Store)
		}()
		return
	}
	m.Store.reset(SourceHA)
	go func() {
		defer close(done)
		(&Client{URL: url, Token: token, Store: m.Store, Log: m.Log}).Run(ctx)
	}()
}
