// Package ha keeps a local copy of Home Assistant entity states, fed either
// by a real HA WebSocket connection or by a demo source.
package ha

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// Entity is the subset of an HA state object homepaper uses.
type Entity struct {
	ID          string         `json:"entity_id"`
	State       string         `json:"state"`
	Attributes  map[string]any `json:"attributes"`
	LastChanged time.Time      `json:"last_changed"`
}

// Domain is the part of the entity ID before the dot, e.g. "sensor".
func (e Entity) Domain() string {
	d, _, _ := strings.Cut(e.ID, ".")
	return d
}

// Name is the friendly name, falling back to the entity ID.
func (e Entity) Name() string {
	if n, ok := e.Attributes["friendly_name"].(string); ok && n != "" {
		return n
	}
	return e.ID
}

// Attr returns an attribute as a string ("" if missing).
func (e Entity) Attr(key string) string {
	v, ok := e.Attributes[key]
	if !ok || v == nil {
		return ""
	}
	return formatValue(v)
}

// AttrFloat returns a numeric attribute.
func (e Entity) AttrFloat(key string) (float64, bool) {
	switch v := e.Attributes[key].(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	}
	return 0, false
}

// Store is the thread-safe state cache. Listeners are called with the IDs of
// changed entities.
type Store struct {
	mu        sync.RWMutex
	entities  map[string]Entity
	listeners []func(ids []string)
	connected bool
	source    string
	lastErr   string
}

func NewStore(source string) *Store {
	return &Store{entities: map[string]Entity{}, source: source}
}

func (s *Store) Get(id string) (Entity, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entities[id]
	return e, ok
}

// All returns all entities sorted by ID.
func (s *Store) All() []Entity {
	s.mu.RLock()
	out := make([]Entity, 0, len(s.entities))
	for _, e := range s.entities {
		out = append(out, e)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Replace swaps in a full snapshot (e.g. after get_states).
func (s *Store) Replace(entities []Entity) {
	s.mu.Lock()
	s.entities = make(map[string]Entity, len(entities))
	ids := make([]string, 0, len(entities))
	for _, e := range entities {
		s.entities[e.ID] = e
		ids = append(ids, e.ID)
	}
	s.mu.Unlock()
	s.notify(ids)
}

// Set updates or removes (when e is nil) a single entity.
func (s *Store) Set(id string, e *Entity) {
	s.mu.Lock()
	if e == nil {
		delete(s.entities, id)
	} else {
		s.entities[id] = *e
	}
	s.mu.Unlock()
	s.notify([]string{id})
}

func (s *Store) OnChange(fn func(ids []string)) {
	s.mu.Lock()
	s.listeners = append(s.listeners, fn)
	s.mu.Unlock()
}

func (s *Store) notify(ids []string) {
	s.mu.RLock()
	ls := append([]func([]string){}, s.listeners...)
	s.mu.RUnlock()
	for _, fn := range ls {
		fn(ids)
	}
}

func (s *Store) setConnected(c bool) {
	s.mu.Lock()
	s.connected = c
	if c {
		s.lastErr = ""
	}
	s.mu.Unlock()
}

// setError records why the source is not connected, for the UI.
func (s *Store) setError(msg string) {
	s.mu.Lock()
	s.lastErr = msg
	s.mu.Unlock()
}

// reset switches the store to another source and drops all entities.
// Listeners are not told: panels keep their frame until the new source's
// snapshot arrives (Replace notifies) or the next minute's render, instead
// of flashing a full frame of dashes in between.
func (s *Store) reset(source string) {
	s.mu.Lock()
	s.entities = map[string]Entity{}
	s.source = source
	s.connected = false
	s.lastErr = ""
	s.mu.Unlock()
}

// Status is the state of the entity source as shown in the UI.
type Status struct {
	Source    string `json:"source"` // SourceDemo or SourceHA
	Connected bool   `json:"connected"`
	Error     string `json:"error,omitempty"` // why it is not connected
	Entities  int    `json:"entities"`
}

const (
	SourceDemo = "demo"
	SourceHA   = "homeassistant"
)

// Status reports the source and its connection state.
func (s *Store) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Status{Source: s.source, Connected: s.connected, Error: s.lastErr, Entities: len(s.entities)}
}
