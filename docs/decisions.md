# Decision log

This is the single source of truth for project decisions. Each entry has one of these statuses:

- **Decided**: agreed by the project owner.
- **Proposed**: recommended, not yet confirmed. Don't build on it as if it were final.
- **Open**: needs a decision.

When an item changes status, update it here (status, date, rationale) and in the topic doc it affects.

## Summary

| ID | Topic | Status |
|---|---|---|
| [D-001](#d-001) | Project name "homepaper" | Decided |
| [D-002](#d-002) | Two components: server + ESP32 device | Decided |
| [D-003](#d-003) | Server prerenders frames and pushes them over WebSocket | Decided |
| [D-004](#d-004) | Widget-based layouts configured in a web UI | Decided |
| [D-005](#d-005) | Server-side diffing and rate limiting | Decided |
| [D-006](#d-006) | No interaction in v1 | Decided |
| [D-007](#d-007) | Panel: 7.5" 800×480 black/white | Decided |
| [D-008](#d-008) | Prototype on a dev board with jumpers | Decided |
| [D-009](#d-009) | Backend: Go | Decided |
| [D-010](#d-010) | Frontend: Vue 3 + TypeScript, embedded in the Go binary | Decided |
| [D-011](#d-011) | Renderer: native Go, server-rendered preview | Decided |
| [D-012](#d-012) | Firmware: Rust + esp-idf-svc | Decided |
| [D-013](#d-013) | ~~HA URL and token configured in the web UI~~ | Superseded by D-016 |
| [D-014](#d-014) | One design language for web UI and panel | Decided, implemented |
| [D-015](#d-015) | Panel redesign: widgets, rules, inversion | Decided, implemented |
| [D-016](#d-016) | Login only via Home Assistant; one global HA connection from the environment | Decided, implemented |
| [D-017](#d-017) | Login details: shared displays, admins only by default, auth off only for development | Decided, implemented |
| [P-001](#p-001) | ~~Server stack: TypeScript / Node~~ | Superseded by D-009, D-010 |
| [P-002](#p-002) | ~~Renderer: Satori + resvg~~ | Superseded by D-011 |
| [P-003](#p-003) | Layout model: snap grid (e.g. 8×6) | Proposed |
| [P-004](#p-004) | Deployment: Docker container | Proposed |
| [P-005](#p-005) | ~~Firmware: PlatformIO + Arduino + GxEPD2~~ | Superseded by D-012 |
| [P-006](#p-006) | HA connection: WebSocket API + long-lived token | Proposed |
| [P-007](#p-007) | Device identity: device ID + token, one layout per device | Proposed |
| [P-008](#p-008) | Hardware: route A, HAT kit + the owner's ESP32-S3 | Proposed |
| [P-009](#p-009) | Frame format and protocol v1 as implemented | Proposed |
| [P-010](#p-010) | Device setup: SoftAP portal, settings in NVS | Proposed |
| [P-011](#p-011) | Server storage: one JSON file | Proposed |
| [P-012](#p-012) | ~~Web UI auth: none by default, optional basic auth~~ | Superseded by D-016 |
| [P-014](#p-014) | ~~Login details~~ | Decided as D-017 |
| [P-013](#p-013) | Fonts: Lucida Sans bitmaps for small text, Go fonts for large | Proposed |
| [O-001](#o-001) | ~~Hardware route~~ | Answered by P-008 |
| [O-002](#o-002) | ~~Is an ESP32-S3 dev board on hand?~~ | Answered: yes |
| [O-003](#o-003) | Grey mode (4 levels) | Open |
| [O-004](#o-004) | Home Assistant add-on packaging | Open |
| [O-005](#o-005) | Battery / HTTP polling mode | Open |

## Decided

### D-001
**Project name "homepaper"** (2026-11-10). *Home* (Home Assistant, smart home) plus *paper* (newspaper, e-paper): the newspaper of your smart home.

### D-002
**Two components** (2026-11-09). A hostable web app connected to HA, and an ESP32 device that displays what the server sends.

### D-003
**The server prerenders. The device receives frames over WebSocket from a configured URL** (2026-11-09). The device subscribes to a WebSocket on the server and is pushed updates as prerendered images, so it does little work. Rationale: simple, stable firmware, and layout changes never need reflashing. Details in [device-protocol.md](device-protocol.md) and [rendering.md](rendering.md).

### D-004
**Widget-based layouts configured in a web UI** (2026-11-09). The user picks from different widget kinds and combines them with HA values: temperature, weather, sensor data in general, light status, target temperature, etc. See [widgets.md](widgets.md).

### D-005
**Server-side diffing and rate limiting** (2026-11-09). The server skips unchanged frames, sends partial regions, debounces, sets minimum intervals per widget and schedules periodic full refreshes. This follows from D-003 and the e-ink panel's properties, and was accepted along with the architecture.

### D-006
**No interaction in v1** (2026-11-09). The display only shows information. No buttons, no touch.

### D-007
**Panel: Waveshare 7.5" 800×480 black/white** (2026-11-09). The user moved from the three-color (B/W/red) panel to the black/white one after the tradeoffs were compared. This follows the recommendation and wasn't confirmed in so many words. Rationale: partial refresh (0.4 s) and fast full refresh (4 s) suit push updates. The three-color panel has no partial refresh and takes about 15–25 s per refresh. Color is kept possible through a panel-agnostic protocol.

### D-008
**The prototype uses a dev board with jumpers** (2026-11-09). The user's starting point is an ESP32-S3 dev board, jumper wires and an e-ink display. Note: this only works with a driver HAT between the board and the panel, see [O-001](#o-001).

### D-009
**Backend: Go** (2026-11-10). The project owner's preference. Rationale: a single static binary and a tiny Docker image, and goroutines suit the device WebSocket hub and the HA client. The HA WebSocket protocol is implemented in-house. Candidate WebSocket library: `coder/websocket` or `gorilla/websocket` (to be chosen when the code starts).

### D-010
**Frontend: Vue 3 + TypeScript + Vite** (2026-11-10). The project owner's preference. The built assets are embedded into the Go binary with `go:embed`, so deployment stays a single binary or container. Candidate grid editor component: `grid-layout-plus`. The frontend **doesn't render widgets**. It edits the layout and shows the server-rendered preview ([D-011](#d-011)).

### D-011
**Renderer: native Go, server-rendered preview** (2026-11-10). This follows from D-009, because Satori is JavaScript-only and the frontend and backend can no longer share widget code. Instead:
- Widgets are implemented **only in Go** and draw with `golang.org/x/image` (opentype/font) and/or `fogleman/gg`.
- Text uses **bitmap fonts (BDF)** or a hard threshold, so 1-bit text stays crisp.
- The editor preview is the **same rendered frame**, sent to the browser as a PNG (updated live over WebSocket), with the editable grid laid over it. One renderer means the preview always matches the panel.
- Cost: widget layout is more manual than JSX/flexbox. A small layout helper in Go is planned.
- Rejected: headless Chromium via chromedp (heavy, works against the lean Go container), and calling Satori/resvg as a sidecar (a second runtime).

### D-012
**Firmware: Rust on ESP-IDF via `esp-idf-svc`** (2026-11-10). The project owner's preference. Rationale: the firmware's job is small (Wi-Fi, WebSocket, copy bytes to the panel, ack), so Rust adds little risk. `esp-idf-svc` (with std) has mature Wi-Fi, TLS and a WebSocket client.
- Panel driver: the `epd-waveshare` crate (0.6, embedded-hal 1.0) has an `epd7in5_v2` module. Whether it supports partial refresh **is unverified**. Otherwise a small custom driver (init, write RAM region, full/partial refresh).
- Toolchain: the ESP32 and ESP32-S3 are Xtensa and need the Espressif Rust toolchain installed with `espup`.
- Rejected for now: `esp-hal` + Embassy without ESP-IDF (leaner, but more work for networking and TLS), and Arduino + GxEPD2 (superseded P-005).

### D-013
**~~Home Assistant URL and token are configured in the web UI~~** (superseded by [D-016](#d-016) on 2026-11-17) (2026-11-10). The project owner's goal: the user enters the HA URL and long-lived token in the UI, without editing files or environment variables. **Implemented 2026-11-11** as the Home Assistant dialog in the header (`PUT /api/settings/ha`):
- The server tests the connection before saving and tells "can't reach" apart from "token rejected".
- Settings are saved in `homepaper.json` (key `home_assistant`) and the source switches without a restart.
- The token is never sent back to the browser. An empty token field keeps the saved token, but only for the same host and port, and never from `https` down to `http`. Otherwise anyone who can open the UI could have the server send the token to an address of their choice.
- **Precedence (implementation choice, not yet confirmed by the project owner):** settings saved in the UI win over `HA_URL`/`HA_TOKEN` from the environment or `server/.env`, which remain as a fallback for headless setups. With neither, the server shows demo entities. The server logs which source it uses at startup, and warns when it ignores `HA_URL`. Removing the connection in the UI falls back to the environment.

### D-014
**One design language for the web UI and the panel** (2026-11-16). The project owner approved the web UI design. Rationale: the panel can only show 1-bit black and white, so the web UI takes its rules from the panel and doesn't invent a second look:
- the same type scale (12/14/17/20/25/34) and the same family: Go fonts in the UI, matching the panel's Lucida bitmaps;
- 2 px ink strokes and an 8 px radius;
- patterns instead of greys;
- status shown by fill, not colour.

One functional accent, editor blue, marks only what never reaches the panel. Details in [design.md](design.md#shared-language-d-014).

### D-015
**Panel redesign** (2026-11-16). The project owner approved the panel mockups:
- large Go Bold values with a small raised decimal;
- three text levels;
- **2 px column rules as the default divider**, with frames kept as a layout-wide option that replaces the per-widget `border` option;
- inversion only for states that need action (open doors, windows and garages, unlocked locks);
- icons that draw the state;
- per-kind rules for what drops as a widget shrinks.

Confirmed by the project owner on 2026-11-16:
- the layout-wide divider setting replaces the per-widget frame option;
- inversion applies to exactly the states listed above;
- the forecast is built now, via `weather.get_forecasts`, fetched by the HA client after `get_states`, every 30 min, and when a weather entity changes (at most every 10 min).

Built on the 8 × 6 grid, which is still [P-003](#p-003). Details and mockups in [design.md](design.md#panel-d-015).

### D-016
**Login only via Home Assistant; one global HA connection from the environment** (2026-11-17). The project owner's decision:
- **Login:** the web UI and API accept only a Home Assistant login, through HA's own OAuth2-style flow (`/auth/authorize`, `/auth/token`; the client ID is homepaper's URL, so nothing is registered in HA). There is no homepaper password and no other login. Rationale: whoever has no access to Home Assistant has no reason to use homepaper, and accounts and MFA stay in one place. homepaper is not packaged as an HA add-on or integration (Ingress, [O-004](#o-004), not pursued).
- **HA address:** set with `HA_URL` in the environment when homepaper is set up.
- **One global HA connection:** the server keeps a single connection with a long-lived token from `HA_TOKEN`, used to render all displays around the clock, whoever is logged in. Per-user tokens were rejected: displays would stop when their owner's token expires or the user is removed, and HA lets every user read every entity's state anyway, so per-user rendering would protect nothing.
- **No privilege escalation through homepaper:** the global token should belong to a dedicated **non-admin** HA user. The server only reads states and fetches forecasts, and never forwards service calls or other HA requests on a user's behalf. After login it only asks HA who the user is, then revokes the login token and keeps its own session cookie; it stores no per-user HA tokens.
- **Consequences:** the "Connect Home Assistant" dialog and `PUT /api/settings/ha` go away, superseding [D-013](#d-013) (no one with UI access can repoint the server's HA connection any more). `HOMEPAPER_PASSWORD` basic auth ([P-012](#p-012)) is replaced. The display WebSocket keeps its device tokens. TLS comes from a reverse proxy in front.

**Implemented 2026-11-17** (`internal/api/auth.go`, `internal/ha/user.go`):
- `/auth/login` redirects to HA's `/auth/authorize` with a random `state` in a short-lived cookie.
- `/auth/callback` checks the state, trades the code at HA's `/auth/token`, asks `auth/current_user` over the WebSocket, then revokes the refresh token (`/auth/revoke`, falling back to `action=revoke` on `/auth/token`).
- The session is an HMAC-signed cookie (HttpOnly, SameSite=Lax, Secure behind HTTPS) with the user's ID, name and admin flag. It lasts 14 days, and the key is `session.key` in the data directory.
- A session can't be revoked on the server: a user removed from HA keeps access until the cookie expires, unless `session.key` is deleted, which logs everyone out. That's accepted for a home server.
- The client ID is homepaper's public URL (`HOMEPAPER_URL`, else taken from the request and proxy headers). The browser reaches HA at `HA_PUBLIC_URL` (default `HA_URL`).
- A connection saved in `homepaper.json` by D-013 is ignored, with a warning in the log. `HOMEPAPER_PASSWORD` is ignored with a warning. Details in [D-017](#d-017).

### D-017
**Login details for [D-016](#d-016)** (2026-11-17, proposed as P-014, confirmed by the project owner):
- **Displays are shared:** everyone allowed to log in sees and edits all displays. There is no per-user ownership.
- **Who may log in:** `HOMEPAPER_LOGIN=admins` (default: HA administrators and the owner) or `users` (every HA user). Others get a "No access" page; their HA login is revoked too.
- **Development and demo:** `HOMEPAPER_AUTH=off` turns the login off, with a warning in the log. Only then may `HA_URL` be empty, which shows demo entities (`make demo`). With the login on, the server refuses to start without `HA_URL` and `HA_TOKEN`.

## Proposed

These are recommended defaults. The project owner hasn't confirmed them yet.

### P-001
**~~Server stack: TypeScript / Node.~~** Superseded by [D-009](#d-009) and [D-010](#d-010) on 2026-11-10.

### P-002
**~~Renderer: Satori + resvg.~~** Superseded by [D-011](#d-011) on 2026-11-10 (not available from Go).

### P-003
**Layout model: snap grid**, e.g. 8 × 6 cells of 100 × 80 px. It's easier to build in the editor and easier to design widgets for than free pixel placement.

### P-004
**Deployment: Docker container** on the user's homelab, with the HA add-on kept as a later option ([O-004](#o-004)).

### P-005
**~~Firmware: PlatformIO + Arduino + GxEPD2.~~** Superseded by [D-012](#d-012) on 2026-11-10.

### P-006
**HA connection: Home Assistant WebSocket API with a long-lived access token**, subscribing to the entities that layouts use. Where the URL and token come from is decided in [D-013](#d-013).

### P-007
**Device identity: device ID + token, one layout per device.** It costs little now and allows several displays later.

*The prototype (2026-11-10) is built on P-003, P-004, P-006 and P-007 as described. They stay Proposed until the project owner confirms them.*

### P-008
**Hardware: route A**, the Waveshare 7.5" e-Paper HAT kit (SKU 13504) jumpered to the owner's ESP32-S3 dev board. This follows the recommendation once the owner said they have an S3 (2026-11-10). It's to be confirmed when ordering. See [hardware.md](hardware.md#controller-two-routes).

### P-009
**Frame format and protocol v1** as specified in [device-protocol.md](device-protocol.md): a 16-byte little-endian header, 1bpp with MSB = left and bit set = white, regions aligned to 8 px, acks carrying a framebuffer CRC-32, and flow control through acks. Implemented and tested with the Go fake device (2026-11-10). Not yet tested on hardware.

### P-010
**Device setup via a SoftAP portal.** The device opens an open access point `homepaper-setup` with a form at `http://192.168.71.1` in three cases:
- it isn't configured: the portal waits indefinitely;
- **BOOT is held for 3 s while running**: a flag is saved in NVS and the device restarts into setup. BOOT can't be held at power-on, because GPIO0 is a strapping pin and that enters the ROM download mode;
- it can't join Wi-Fi after 5 attempts: the portal **restarts after 5 min** to retry, e.g. when the router is still booting after a power cut.

The Wi-Fi password and token are never shown in the form; an empty field keeps the saved value. **Prefill link (2026-11-11):** the web UI links to `http://192.168.71.1/?server_url=…&device_id=…&token=…`; the portal fills the form from the query, carries the token in a hidden field and asks only for Wi-Fi. The token then sits in that browser's history, which is no worse than the form POST over the open access point, but worth knowing. `CONFIG_HTTPD_MAX_REQ_HDR_LEN` (2048) and `CONFIG_HTTPD_MAX_URI_LEN` (1024) are raised for it. *Not yet tested on hardware.* The device restarts on any fatal error, or after 5 min without a server connection. Wi-Fi credentials, server URL, device ID and token are stored in NVS. Values from `.env` are compiled in as defaults for development. There's no captive-portal DNS yet, so the address has to be typed in.

### P-011
**Server storage: a single JSON file** (`$HOMEPAPER_DATA/homepaper.json`, written atomically). It's enough for a handful of devices and layouts. Revisit if history or many users are needed.

### P-012
**~~Web UI authentication: none by default~~** (superseded by [D-016](#d-016) on 2026-11-17), meant for a trusted LAN. Setting `HOMEPAPER_PASSWORD` turns on HTTP basic auth for the UI and API. The device WebSocket always authenticates with device tokens. An HA add-on with ingress would replace this ([O-004](#o-004)). Without a password, anyone on the LAN can also change the Home Assistant connection (`PUT /api/settings/ha`); the server tests a new address before saving, but the request itself is unauthenticated.

### P-013
**Fonts: Lucida Sans bitmap fonts up to 25 px (and at 34 px), the Go fonts above** (2026-11-11, replacing "Go fonts, thresholded"). This carries out the bitmap-font plan from [D-011](#d-011).
- Why: a comparison of approaches at panel resolution showed that a thresholded outline renders stems unevenly below about 25 px, whatever the font. Running real TrueType hinting (`golang/freetype`) didn't change that for the Go fonts. DejaVu and Liberation stay even, but their 1 px strokes are too light for paper. The hand-drawn Lucida Sans bitmaps were clearly the most legible.
- Which: the X.Org `bh-75dpi`/`bh-100dpi` BDF files, regular and bold, at 12, 14, 17, 20, 25 and 34 px. They are by Bigelow & Holmes, who also drew the Go fonts, so the two match. They cover Latin-1 (umlauts, °, ·, –, €), but not "→"; per-glyph fallback takes that from the Go font on the same baseline.
- License: the files must stay **unmodified**, and their notice must appear in code comments and user documentation. It is in `server/internal/render/fonts.go`, the README and `fonts/lucida/COPYING`. The font's own "…" has only two dots, so shortened text ends in "...".
- How text is sized: from a fixed scale (bitmap sizes, then whole vector pixel sizes from 26 px), placed on whole pixels and clipped to its widget. Labels are 17 px, step down to 14 px, then get shortened. Content shrinks to 12 px before it is shortened.
- Alternatives that were considered: Helvetica bitmaps (too light), Spleen and Terminus (monospace), and TRMNL-style pixel fonts.

### P-014
**~~Login details~~.** Decided on 2026-11-17 as [D-017](#d-017).

## Open

### O-001
**Hardware route.** *Answered 2026-11-10 by [P-008](#p-008) (route A).*
- **A:** Waveshare 7.5" e-Paper HAT kit (SKU 13504), whose driver HAT is jumpered to an ESP32-S3 dev board. This matches D-008.
- **B:** a raw panel (Eckstein WS13187) plus the Waveshare Universal e-Paper Driver Board ESP32. No wiring, and tidier in a frame.

The raw panel alone **can't** be jumpered to a dev board. See [hardware.md](hardware.md#controller-two-routes). The answer depends on [O-002](#o-002).

### O-002
**Is an ESP32-S3 dev board already on hand?** *Answered 2026-11-10: yes.* So route A is the cheapest start.

### O-003
**Grey mode (4 levels, 2bpp).** The panel supports it (2.1 s refresh), but not with partial refresh. It could be a per-device option for nicer images.

### O-004
**Home Assistant add-on.** It would provide ingress and HA authentication for free. Considered after the Docker version works.

### O-005
**Battery / HTTP polling mode.** The WebSocket push requires constant power. A later mode could poll over HTTP with ETag and then deep sleep.
