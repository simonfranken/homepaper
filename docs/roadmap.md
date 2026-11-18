# Roadmap

Each milestone ends with something you can run and see. The order is chosen so the riskiest parts (panel, board, server → device path) are proven first.

## Status (2026-11-16)

Built and tested **without a panel**:
- **Server:** HA client (tested against a real HA on 2026-11-10: initial states, live `state_changed` events, entity picker in the editor, partial frames to `fakedevice`), demo mode, renderer, region diff, device hub, REST API, Docker image.
- **Editor UI:** tested in headless Chromium and in Chrome against a real HA. Reworked on 2026-11-11 (tested in Chrome with demo data and `fakedevice`): displays as tabs with a Layout/Display switch, a searchable entity picker, undo/redo, keyboard moves and resizing, flags for widgets without an entity, a guided connect flow. The HA settings dialog was replaced on 2026-11-17 by the login via Home Assistant ([D-016](decisions.md#d-016)).
- **Firmware:** setup portal, Wi-Fi, WebSocket, framebuffer, acks, `NullPanel` with ASCII-art output. It compiles, but **hasn't run on the ESP32-S3 yet**. The setup portal's prefill link (2026-11-11) is untested on hardware too.
- **Protocol:** verified end to end with `server/cmd/fakedevice`.
- **Grey mode (2026-11-18):** 2bpp frames with hybrid refresh, optional per display ([D-018](decisions.md#d-018)). Verified with `fakedevice` (grey changes go out as full frames, routine updates as partial ones); the editor toggle and the live preview's 2bpp decoding are not yet checked in a browser, and nothing is tested on the panel.

- **Design (2026-11-16):** the panel redesign ([D-015](decisions.md#d-015)) is rendered, with daily forecasts from Home Assistant. The web UI follows the shared design language ([D-014](decisions.md#d-014)). Tested with demo data, `fakedevice` and headless Chromium.

Next: flash the S3 and check the connection and setup on real hardware, then order the panel ([P-008](decisions.md#p-008)) and write the panel driver (the rest of M0, then M1).

## M0: hello panel

- Order the hardware, depending on the route chosen ([O-001](decisions.md#o-001)).
- Firmware (Rust + `esp-idf-svc`): Wi-Fi, WebSocket client and panel driver. It shows a full frame it receives. This includes checking whether `epd-waveshare`'s `epd7in5_v2` driver supports partial refresh, or whether a custom driver is needed ([D-012](decisions.md#d-012)).
- Server: a minimal endpoint that serves a static test image as a 1bpp frame.
- **Done when** a test image from the server appears on the panel.

## M1: partial updates

- The server sends partial frames for a changing region (e.g. a counter or clock).
- Firmware does partial refresh and acks. The server applies the full-refresh cadence.
- **Done when** a region updates in under a second without visible artefacts, and periodic full refreshes clear ghosting.

## M2: Home Assistant connection

- HA client using the WebSocket API and a long-lived token, plus the state store.
- A hard-coded layout with a few widgets bound to real entities.
- Debouncing, skipping unchanged frames and region diffs.
- **Done when** an HA state change appears on the panel within seconds, and nothing is sent when the frame didn't change. *(Server side verified with `fakedevice` against a real HA; still needs the panel.)*

## M3: layout editor

- Web UI: device management, grid editor, widget picker, entity binding, live preview from the same renderer. *(Done in the prototype.)*
- ~~Settings page for the HA URL and token ([D-013](decisions.md#d-013))~~: done 2026-11-11, replaced on 2026-11-17 by `HA_URL`/`HA_TOKEN` and the login via Home Assistant (M3b).
- Storage for devices and layouts.
- **Done when** a layout built entirely in the UI appears on the panel.

## M3b: login via Home Assistant ([D-016](decisions.md#d-016)), done 2026-11-17

- Log in to the web UI and API only through Home Assistant's OAuth2-style flow; a session cookie afterwards; the login token is revoked right after reading the user.
- `HA_URL` and `HA_TOKEN` from the environment only, as one global connection; the HA settings dialog goes away.
- The details in [D-017](decisions.md#d-017): shared displays, who may log in, auth off for development.
- **Done when** the UI is only reachable after an HA login, a non-admin service account renders the displays, and nothing in the UI can change the HA connection. *(Tested against a fake HA in `internal/api/auth_test.go`; not yet against a real HA.)*

## M4: polish and frame build

- The v1 widget catalogue complete. Fonts and icons tuned for 1 bit: the panel redesign ([D-015](decisions.md#d-015)) and the web UI restyle ([D-014](decisions.md#d-014)) are done; tune both on the real panel.
- Docker image and setup docs.
- The frame build, see [hardware.md](hardware.md#frame-build).
- **Done when** it hangs on the wall and runs unattended.

## Later

- Color panels, an HA add-on ([O-004](decisions.md#o-004)), a battery/polling mode ([O-005](decisions.md#o-005)), over-the-air firmware updates, more widgets, interaction.
