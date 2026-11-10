// Package store persists devices and their layouts in a single JSON file.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"
)

// Default grid on an 800×480 panel: 100×80 px cells.
const (
	DefaultCols = 8
	DefaultRows = 6
)

type Widget struct {
	ID      string         `json:"id"`
	Kind    string         `json:"kind"`
	X       int            `json:"x"`
	Y       int            `json:"y"`
	W       int            `json:"w"`
	H       int            `json:"h"`
	Options map[string]any `json:"options"`
}

type Layout struct {
	Cols    int      `json:"cols"`
	Rows    int      `json:"rows"`
	Widgets []Widget `json:"widgets"`
}

// Panel describes the display a device reported in its hello message.
type Panel struct {
	Model  string `json:"model"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Format string `json:"format"`
}

type Device struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Token     string    `json:"token"`
	Panel     Panel     `json:"panel"`
	Layout    Layout    `json:"layout"`
	CreatedAt time.Time `json:"created_at"`
	LastSeen  time.Time `json:"last_seen,omitempty"`
}

// DefaultPanel is assumed until a device reports its own.
var DefaultPanel = Panel{Model: "waveshare-7in5-v2", Width: 800, Height: 480, Format: "1bpp"}

var ErrNotFound = errors.New("not found")

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// ValidID reports whether id is usable as a device ID (lowercase letters,
// digits, dashes; max 32 chars).
func ValidID(id string) bool { return idPattern.MatchString(id) }

type Store struct {
	path    string
	mu      sync.RWMutex
	devices map[string]*Device
}

type fileFormat struct {
	Devices []*Device `json:"devices"`
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, devices: map[string]*Device{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var f fileFormat
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	for _, d := range f.Devices {
		s.devices[d.ID] = d
	}
	return s, nil
}

// save writes atomically. Caller holds mu.
func (s *Store) save() error {
	f := fileFormat{Devices: s.sortedLocked()}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) sortedLocked() []*Device {
	out := make([]*Device, 0, len(s.devices))
	for _, d := range s.devices {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// List returns copies of all devices.
func (s *Store) List() []Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Device
	for _, d := range s.sortedLocked() {
		out = append(out, *d)
	}
	return out
}

func (s *Store) Get(id string) (Device, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.devices[id]
	if !ok {
		return Device{}, ErrNotFound
	}
	return *d, nil
}

func newToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Create adds a device with a fresh token and an example layout.
func (s *Store) Create(id, name string) (Device, error) {
	if !ValidID(id) {
		return Device{}, errors.New("invalid device id: use lowercase letters, digits and dashes (max 32)")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.devices[id]; ok {
		return Device{}, errors.New("device already exists")
	}
	if name == "" {
		name = id
	}
	d := &Device{ID: id, Name: name, Token: newToken(), Panel: DefaultPanel, Layout: ExampleLayout(), CreatedAt: time.Now()}
	s.devices[id] = d
	return *d, s.save()
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.devices[id]; !ok {
		return ErrNotFound
	}
	delete(s.devices, id)
	return s.save()
}

// Update applies fn to a device and persists the result.
func (s *Store) Update(id string, fn func(d *Device)) (Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.devices[id]
	if !ok {
		return Device{}, ErrNotFound
	}
	fn(d)
	return *d, s.save()
}

// ExampleLayout is what a new device starts with. Entity IDs match the demo
// source; with a real HA they show as "unavailable" until rebound.
func ExampleLayout() Layout {
	return Layout{
		Cols: DefaultCols,
		Rows: DefaultRows,
		Widgets: []Widget{
			{ID: "w1", Kind: "clock", X: 0, Y: 0, W: 4, H: 1, Options: map[string]any{"format": "date"}},
			{ID: "w2", Kind: "weather", X: 4, Y: 0, W: 4, H: 2, Options: map[string]any{"entity": "weather.home"}},
			{ID: "w3", Kind: "temperature", X: 0, Y: 1, W: 2, H: 2, Options: map[string]any{"entity": "sensor.living_room_temperature", "humidity_entity": "sensor.living_room_humidity", "label": "Living room"}},
			{ID: "w4", Kind: "temperature", X: 2, Y: 1, W: 2, H: 2, Options: map[string]any{"entity": "sensor.outdoor_temperature", "label": "Outdoor"}},
			{ID: "w5", Kind: "climate", X: 0, Y: 3, W: 4, H: 1, Options: map[string]any{"entity": "climate.living_room"}},
			{ID: "w6", Kind: "value", X: 4, Y: 2, W: 2, H: 2, Options: map[string]any{"entity": "sensor.power_consumption", "label": "Power", "decimals": 0.0}},
			{ID: "w7", Kind: "binary", X: 6, Y: 2, W: 2, H: 1, Options: map[string]any{"entity": "binary_sensor.front_door"}},
			{ID: "w8", Kind: "binary", X: 6, Y: 3, W: 2, H: 1, Options: map[string]any{"entity": "binary_sensor.bathroom_window"}},
			{ID: "w9", Kind: "lights", X: 0, Y: 4, W: 4, H: 2, Options: map[string]any{"entities": "light.living_room, light.kitchen, light.hallway", "label": "Lights"}},
			{ID: "w10", Kind: "text", X: 4, Y: 4, W: 4, H: 2, Options: map[string]any{"text": "homepaper", "size": "large"}},
		},
	}
}

// ResetToken issues a new token; the old one stops working immediately.
func (s *Store) ResetToken(id string) (Device, error) {
	return s.Update(id, func(d *Device) { d.Token = newToken() })
}
