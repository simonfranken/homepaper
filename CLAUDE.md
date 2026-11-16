# CLAUDE.md

Guidance for Claude Code when working in this repository.

## Project

homepaper is an e-ink smart home dashboard. A self-hosted server connects to Home Assistant, lets the user arrange widgets bound to HA entities in a web UI, renders the layout into panel-ready 1-bit bitmaps, and pushes them over WebSocket to an ESP32-S3 that drives an e-ink panel. See `README.md` for an overview and `docs/` for details.

## Source of truth

- `docs/decisions.md` is the decision log. Every item there is **Decided**, **Proposed** or **Open**.
  - Don't treat a Proposed item as decided. Ask the user, or say clearly that you're building on a proposal.
  - When the user decides something, update its entry in `docs/decisions.md` (status, date, rationale) and the topic doc it affects.
- `docs/device-protocol.md` is the wire spec. Server (`internal/frame`, `internal/devicehub`), firmware (`src/protocol.rs`, `src/ws.rs`) and `cmd/fakedevice` must agree with it. Bump the protocol version for incompatible changes.
- Facts in `docs/hardware.md` marked *(to verify)* come from memory, not from a checked source. Verify them before code depends on them.
- Keep the docs current as part of every change.

## Tech stack

- **Backend:** Go (`server/`). HA client, storage, renderer, device WebSocket hub.
- **Renderer:** native Go (`server/internal/render`). Widgets exist **only in Go**. The frontend shows server-rendered PNG previews and never draws widgets itself (D-011). The editor builds its forms from the field schema at `GET /api/widgets`.
- **Frontend:** Vue 3 + TypeScript + Vite in `server/web/ui`. It builds into `server/web/dist`, which `server/web/embed.go` embeds.
- **Firmware:** Rust on ESP-IDF v5.5.3 via `esp-idf-svc` 0.52 (`firmware/`), target `xtensa-esp32s3-espidf`.

## Commands

Server (run in `server/`):

```sh
make run          # build UI + server, run on :8080 (reads server/.env; demo data if HA_URL is empty)
make test         # go vet, go test, vue-tsc
make fakedevice   # build bin/fakedevice, a protocol test client
make docker       # build the Docker image
cd web/ui && npm run dev   # Vite dev server on :5173, proxies to the Go server on :8080
```

Firmware (run in `firmware/`):

```sh
source ~/export-esp.sh      # Xtensa toolchain env (path depends on the machine)
cargo build --release       # verification. The first build downloads ESP-IDF into .embuild/ (slow)
espflash save-image --chip esp32s3 --flash-size 4mb target/xtensa-esp32s3-espidf/release/homepaper-fw target/homepaper-fw.bin   # checks the app fits the partition
espflash flash --monitor target/xtensa-esp32s3-espidf/release/homepaper-fw   # needs the board attached
```

## Gotchas

- **Never run `cargo run` or `cargo test` in `firmware/`.** The cargo runner flashes attached hardware. Use `cargo build`.
- The partition table (`firmware/partitions.csv`, 3 MB factory app, fits 4 MB flash) is applied by **espflash** through `espflash.toml`, not by ESP-IDF. Run espflash from `firmware/`.
- After changing `extra_components` in `firmware/Cargo.toml`, run `cargo clean -p esp-idf-sys`. Otherwise a stale build may lack the new component's bindings.
- `ws.rs` uses the C `esp_websocket_client` API directly because the safe wrapper can't reassemble fragmented messages. A 48 KB frame arrives in many chunks. Buffers are allocated once at boot and must never grow.
- The BDF files in `server/internal/render/fonts/lucida/` must stay byte-for-byte unmodified (their license). Don't subset, convert or patch them; handle missing glyphs in code (`bitmapFace` falls back to the Go font).
- In `server/`, `coder/websocket` has a 32 KB default read limit. The HA client raises it, because `get_states` responses are large.
- When killing the dev server, use `pkill -x homepaper`, not `pkill -f bin/homepaper`, which also matches your own shell.
- HA's WebSocket API rejects message IDs that don't increase within a connection ("Identifier values have to increase"). The HA client subscribes with id 1, then sends `get_states` with id 2. Keep that order when adding requests.

## Conventions

- In docs and UI text, refer to people using homepaper as "the user" or "they".
- Keep the device side simple. Rendering, layout, diffing and rate limiting belong on the server.
- Don't put secrets (HA tokens, Wi-Fi credentials, device tokens) in the repo. Use `.env` (gitignored) and `.env.template`. HA settings are entered in the web UI and saved in `homepaper.json` (D-013); `HA_URL`/`HA_TOKEN` in `server/.env` are only a fallback.
