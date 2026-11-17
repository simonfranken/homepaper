# homepaper

**The newspaper of your smart home.** An e-ink wall display that shows Home Assistant data, laid out the way you configure it in a web UI.

The name comes from *home* (Home Assistant, the smart home) and *paper* (newspaper, e-paper).

## How it works

homepaper has two components:

1. **Server** (Go + Vue 3): a self-hosted web app that
   - connects to Home Assistant and keeps entity states up to date;
   - offers a web UI where you place **widgets** (temperature, weather, sensors, lights, thermostats, …) on a grid and bind them to HA entities;
   - renders each layout into a 1-bit bitmap for the panel;
   - pushes only what changed to devices over a WebSocket.
2. **Device** (Rust on ESP32-S3): connects to its configured server URL, receives prerendered frames and draws them. It does no layout or rendering of its own.

```
Home Assistant ──WS API──► homepaper server ──WS (binary frames)──► ESP32-S3 ──SPI──► e-ink panel
                                  ▲
                          browser: layout editor
```

## Status

The server, editor and firmware are built. The panel hasn't been bought yet, so the firmware uses a stand-in panel that prints frames as ASCII art to the serial monitor. See [roadmap](docs/roadmap.md#status-2026-11-11).

## Quick start: server

Requires Go ≥ 1.26 and Node ≥ 20.

**Try it without Home Assistant** (demo entities, no login):

```sh
cd server
make demo                # builds the UI and server, runs on :8080 with HOMEPAPER_AUTH=off
```

Open http://localhost:8080 and add a display. The **Display** tab walks you through connecting the panel and links to its setup page with the server address, device ID and token filled in.

**With your Home Assistant:**

1. In Home Assistant, create a user for homepaper (Settings → People → Users). Make it a **regular user, not an administrator**: homepaper only reads states, and that's all the token can do.
2. Log in to Home Assistant as that user and create a long-lived access token (your profile → Security → Long-lived access tokens).
3. Set `HA_URL` and `HA_TOKEN` (`cp server/.env.template server/.env`, or the environment), then `make run`.
4. Open homepaper. It sends you to Home Assistant's login page; after logging in you're back in the editor.

**Login ([D-016](docs/decisions.md#d-016)):** homepaper has no accounts of its own. You log in with your Home Assistant user, by default only Home Assistant administrators (`HOMEPAPER_LOGIN=users` lets every HA user in). homepaper only asks HA who you are, ends that HA login right away and keeps its own session cookie for 14 days. All displays are shared by everyone who may log in. The display itself connects with its device token, not a login.

**Behind a reverse proxy:** put homepaper behind your proxy for HTTPS, pass `/ws/device` through as a WebSocket, and set `HOMEPAPER_URL` to the address you open it at. Without it, homepaper takes the address from the `Host` and `X-Forwarded-Proto`/`-Host` headers. If the browser reaches Home Assistant at another address than the server does (e.g. the server uses a Docker-internal name), set `HA_PUBLIC_URL` to the browser's one.

Or with Docker:

```sh
cd server && make docker
docker run -d -p 8080:8080 -v homepaper-data:/data --env-file .env homepaper
```

GitHub Actions (`.github/workflows/docker.yml`) runs `make test` and builds the image for `linux/amd64` and `linux/arm64`. Pushes to `main` publish `ghcr.io/simonfranken/homepaper:latest` (plus a `sha-…` tag), and `v*` tags publish versioned images. Pull requests only build. To use the published image:

```sh
docker run -d -p 8080:8080 -v homepaper-data:/data --env-file .env ghcr.io/simonfranken/homepaper:latest
```

`.env` is never copied into the image; pass it at runtime as above.

| Variable | Default | Meaning |
|---|---|---|
| `HA_URL`, `HA_TOKEN` | unset | The Home Assistant connection (required unless `HOMEPAPER_AUTH=off`). Use a token of a non-admin HA user. |
| `HA_PUBLIC_URL` | `HA_URL` | Home Assistant's address as the browser reaches it, for the login redirect |
| `HOMEPAPER_URL` | from the request | homepaper's address as the browser reaches it, e.g. `https://homepaper.example.org` |
| `HOMEPAPER_LOGIN` | `admins` | Who may log in: `admins` (HA administrators and the owner) or `users` (every HA user) |
| `HOMEPAPER_AUTH` | `on` | `off` turns the login off and, without `HA_URL`, shows demo entities. **Development and demos only.** |
| `HOMEPAPER_ENV_FILE` | `.env` | File to load the settings in this table from |
| `HOMEPAPER_LISTEN` | `:8080` | Listen address |
| `HOMEPAPER_DATA` | `data` (`/data` in Docker) | Directory for `homepaper.json` and `session.key` (signs logins; delete it to log everyone out) |
| `HOMEPAPER_MIN_INTERVAL` | `3s` | Minimum time between frames to one device |
| `HOMEPAPER_FULL_REFRESH_EVERY` | `20` | Partial updates before a forced full refresh |
| `TZ` | system | Timezone for clock widgets |

The display must be able to reach the server's address. A server running in a VM or container on a laptop may only be reachable as `localhost` on that machine.

## Quick start: firmware

Requires the Rust ESP toolchain (`espup install`) and `espflash`. Build on any machine. Flash from the machine the board is plugged into.

```sh
cd firmware
cargo build --release
espflash flash --monitor target/xtensa-esp32s3-espidf/release/homepaper-fw
```

Run `espflash` from the `firmware/` directory so it picks up `espflash.toml` (the 4 MB partition table). Optionally copy `.env.template` to `.env` before building to compile in default setup values for development.

1. On first boot, the device opens the Wi-Fi network **`homepaper-setup`**. Join it and open **http://192.168.71.1**.
2. Use **Open the setup page** on the display's **Display** tab in the web UI: the form arrives with the server URL, device ID and token filled in, and you only choose your Wi-Fi. (Or type them in by hand; the same tab lists them.) The device restarts and connects.
3. To get back to setup later, hold **BOOT** for 3 seconds while the device is running. (Not at power-on: that puts the ESP32-S3 into its flashing mode.) If Wi-Fi fails, the portal opens by itself for 5 minutes, then the device retries.

The web UI shows the device as online and *in sync* once its framebuffer checksum matches the server's.

## Testing without hardware

```sh
cd server && make fakedevice
./bin/fakedevice -id <device-id> -token <token> -png frame.png
```

This speaks the device protocol like the firmware, and writes what a panel would show to `frame.png`.

## Fonts

The panel's text is set in the Go fonts (BSD license) and, for small sizes, in the Lucida Sans bitmap fonts from the X.Org `bh-75dpi` and `bh-100dpi` packages, included unmodified:

> (c) Copyright 1989 Sun Microsystems, Inc. Sun design patents pending in the U.S. and foreign countries. OPEN LOOK is a trademark of AT&T. Used by written permission of the owners.
>
> (c) Copyright Bigelow & Holmes 1986, 1985. Lucida is a registered trademark of Bigelow & Holmes. Permission to use the Lucida trademark is hereby granted only in association with the images and fonts described in this file.

The full notice is in [`server/internal/render/fonts/lucida/COPYING`](server/internal/render/fonts/lucida/COPYING).

## Documentation

Design notes and decisions are in [`docs/`](docs/README.md):

- [Vision](docs/vision.md): goals and non-goals
- [Architecture](docs/architecture.md): components, data flow, code map
- [Device protocol](docs/device-protocol.md): the v1 wire format
- [Hardware](docs/hardware.md): panel, wiring, frame build
- [Design](docs/design.md): the shared design language, the web UI and the panel redesign
- [Decisions](docs/decisions.md): what is decided, proposed or open
- [Roadmap](docs/roadmap.md): milestones and current status
