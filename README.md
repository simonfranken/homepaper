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

```sh
cd server
make run                 # builds the UI and server, runs with demo entities on :8080
```

Open http://localhost:8080 and add a display. You can lay it out right away with demo data. The **Display** tab walks you through connecting the panel and links to its setup page with the server address, device ID and token filled in.

To use your Home Assistant, create a long-lived access token (HA profile → Security → Long-lived access tokens) and click **Connect Home Assistant** in the header. The server tests the connection, saves it in `homepaper.json` and switches over without a restart. The token is never shown again.

For headless setups, `HA_URL` and `HA_TOKEN` also work, from the environment or from `.env` in the working directory (`cp .env.template .env`). Settings saved in the UI take precedence over them; remove the connection in the UI to use the environment again ([D-013](docs/decisions.md#d-013)).

Or with Docker:

```sh
cd server && make docker
docker run -d -p 8080:8080 -v homepaper-data:/data --env-file .env homepaper
```

`.env` is never copied into the image; pass it at runtime as above.

| Variable | Default | Meaning |
|---|---|---|
| `HA_URL`, `HA_TOKEN` | unset | Fallback Home Assistant connection, used while none is saved in the UI. Without either, **demo mode** is used. |
| `HOMEPAPER_ENV_FILE` | `.env` | File to load the settings in this table from |
| `HOMEPAPER_LISTEN` | `:8080` | Listen address |
| `HOMEPAPER_DATA` | `data` (`/data` in Docker) | Directory for `homepaper.json` |
| `HOMEPAPER_PASSWORD` | unset | Turns on HTTP basic auth for the UI and API (any username). Without it, anyone on the network can edit layouts and the HA connection. |
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

## Documentation

Design notes and decisions are in [`docs/`](docs/README.md):

- [Vision](docs/vision.md): goals and non-goals
- [Architecture](docs/architecture.md): components, data flow, code map
- [Device protocol](docs/device-protocol.md): the v1 wire format
- [Hardware](docs/hardware.md): panel, wiring, frame build
- [Decisions](docs/decisions.md): what is decided, proposed or open
- [Roadmap](docs/roadmap.md): milestones and current status
