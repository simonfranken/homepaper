# Architecture

## Overview

```
┌──────────────── homepaper server (self-hosted) ────────────────┐
│                                                                 │
│  HA client ──► state store ──► renderer ──► device hub (WS)     │
│  (HA WebSocket API,             (layout +    per-device         │
│   entity subscriptions)          widgets →   channels, diffing, │
│                                  bitmap)     rate limiting      │
│                                                                 │
│  Web UI: layout editor + live preview (same renderer)           │
│  Storage: devices, layouts, widget configs                      │
└─────────────────────────────────────────────────────────────────┘
                              │  binary WebSocket frames
                              ▼
               ESP32 ──SPI──► 7.5" e-ink panel (800×480, B/W)
```

Two components ([D-002](decisions.md#d-002)):

| Part | Technology |
|---|---|
| Backend (HA client, storage, renderer, device hub) | Go ([D-009](decisions.md#d-009)) |
| Renderer | Native Go ([D-011](decisions.md#d-011)) |
| Frontend (layout editor) | Vue 3 + TypeScript + Vite, embedded in the Go binary ([D-010](decisions.md#d-010)) |
| Firmware | Rust + `esp-idf-svc` ([D-012](decisions.md#d-012)) |

### 1. Server

| Module | Responsibility |
|---|---|
| **HA client** | Connects to Home Assistant's WebSocket API, subscribes to the entities that layouts use, keeps a local state store current ([P-006](decisions.md#p-006)). URL and token are entered in the web UI, with `HA_URL`/`HA_TOKEN` as a fallback ([D-013](decisions.md#d-013)). Planned: `HA_URL`/`HA_TOKEN` from the environment only, as one global connection, and login only via Home Assistant ([D-016](decisions.md#d-016)). `ha.Manager` swaps the source (demo or HA) at runtime. |
| **Storage** | Saves devices, their layouts, widget instances and their entity bindings. |
| **Renderer** | Turns layout + widget configs + current states into a bitmap for the device's panel (resolution, color depth, packing). See [rendering.md](rendering.md). |
| **Device hub** | Accepts device WebSocket connections, authenticates them, sends full and partial frames, decides when to send (diffing, rate limiting, full-refresh cadence). See [device-protocol.md](device-protocol.md). |
| **Web UI** | Layout editor: place widgets on a grid, pick widget kinds, bind entities. The preview is the server-rendered frame (PNG, updated live) with the editable grid laid over it, and the UI never draws widgets itself. |
| **Debug endpoints** | Rendered frames as PNG for the preview and troubleshooting. |

### 2. Device (ESP32)

- Connects to Wi-Fi and to its **configured server URL** ([D-003](decisions.md#d-003)).
- Sends a hello message (device ID, token, panel model, resolution, firmware version).
- Receives full or partial frames and writes them to the panel, using partial refresh where the server asks for it.
- Reconnects automatically. Has no layout or rendering logic.

## Data flow

1. A state changes in HA, and the HA client updates the state store.
2. The server finds which layouts or devices use that entity and re-renders them (debounced).
3. The device hub compares the new frame with the last one sent to that device:
   - if nothing changed, nothing is sent;
   - if some regions changed, a partial frame is sent for the changed rectangles;
   - every N updates, or when a time limit passes, a full refresh is sent to clear ghosting.
4. The ESP32 draws what it receives.

## Responsibility split

Anything that's hard, changes often, or needs configuration lives on the **server**. The **device** stays a dumb, stable frame sink. This keeps firmware updates rare and lets one firmware serve every layout.

## Deployment

The planned default is a Docker container on the user's homelab (e.g. Proxmox), next to HA ([P-004](decisions.md#p-004)). Running as an HA add-on, with ingress and HA authentication, is an option for later ([O-004](decisions.md#o-004)).

## Code map

```
server/
├── cmd/homepaper/        main: config from env, wiring
├── cmd/fakedevice/       protocol test client (acts like the firmware)
├── internal/ha/          state store, HA WebSocket client, demo source, Manager (switches source at runtime)
├── internal/store/       devices, layouts, saved HA settings (JSON file)
├── internal/render/      widgets, icons, fonts → frame.Bitmap
├── internal/frame/       1bpp bitmap, region diff, wire encoding (+ tests)
├── internal/devicehub/   device WebSocket: auth, sessions, diff/rate limit, acks
├── internal/api/         REST API, HA settings (settings.go), UI event WebSocket, basic auth, fonts, SPA serving
└── web/                  embed.go + dist/ (build output) + ui/ (Vue 3 source)

firmware/src/
├── main.rs       boot, setup decision, Wi-Fi, frame loop
├── config.rs     settings in NVS (.env defaults)
├── setup.rs      SoftAP setup portal
├── ws.rs         esp_websocket_client FFI, fragment reassembly
├── protocol.rs   frame header parsing/validation
└── display.rs    framebuffer, CRC-32, Panel trait, NullPanel
```

**UI events:** the editor keeps a WebSocket open to `/api/events`. The server sends small notifications (`devices`, `device` with an ID, `render` with an ID, `settings` after the HA connection changed), and the UI refetches what changed or reloads the preview image.

**Fonts:** `GET /api/fonts/{name}` serves the Go fonts embedded for the renderer, so the editor's text is set in the same typeface as the panel without shipping a second copy.
