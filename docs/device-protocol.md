# Device protocol v1

Status: **implemented** in `server/internal/devicehub`, `server/internal/frame` and `firmware/src/{ws,protocol}.rs`. Not yet tested on real hardware. `server/cmd/fakedevice` is the reference client used for testing. Changes need a protocol version bump.

## Transport

- The device opens a **WebSocket** to its configured server URL, e.g. `ws://192.168.1.10:8080/ws/device` (or `wss://…` behind TLS) ([D-003](decisions.md#d-003)).
- **Text messages** carry JSON. **Binary messages** carry frames.
- The device reconnects on its own (the C client retries every 5 s). After each (re)connect it sends `hello` again, and the server always starts with a full frame.
- A newer connection for the same device ID replaces the older one. The server drops the old connection without a close handshake (after a device reset it is usually half-open), and only the newest connection sets the status the UI shows.

## Handshake

Device → server, the first message:

```json
{
  "type": "hello",
  "device_id": "living-room",
  "token": "5bedfeb8541aace0c81ee30f453cdfde",
  "fw_version": "0.1.0",
  "panel": { "model": "none-800x480", "width": 800, "height": 480, "format": "2bpp" },
  "capabilities": ["partial"]
}
```

- `device_id` and `token` are created in the web UI ([P-007](decisions.md#p-007)).
- `panel.width` must be a multiple of 8, and `format` must be `1bpp` or `2bpp` (4 grey levels, [D-018](decisions.md#d-018)). The format fixes the pixel data of every frame on this connection. The server renders at this size and stores the panel info on the device. The firmware reports `2bpp`; the frame header version stays `1`, because only the payload length depends on the negotiated format and 1bpp devices are unaffected.
- `model` is informational. Until a panel is connected, the firmware reports `none-800x480` (it uses `NullPanel`).

Server → device on success:

```json
{ "type": "welcome", "full_refresh_every": 20 }
```

On failure (unknown device, wrong token, unsupported panel), the server sends this and closes the connection with status 1008:

```json
{ "type": "error", "message": "unknown device or wrong token" }
```

## Frames (server → device, binary)

A 16-byte header, **little-endian**, followed by the pixel data:

| Offset | Size | Field | Value |
|---|---|---|---|
| 0 | 2 | magic | `0x5048`, i.e. the bytes `'H' 'P'` |
| 2 | 1 | version | `1` |
| 3 | 1 | kind | `0` = full refresh, `1` = partial refresh |
| 4 | 4 | seq | frame sequence number, u32, counts up per connection |
| 8 | 2 | x | region left edge, px, **multiple of 8** |
| 10 | 2 | y | region top edge, px |
| 12 | 2 | w | region width, px, **multiple of 8** |
| 14 | 2 | h | region height, px |
| 16 | w × bpp / 8 × h | data | packed pixels of the region (`w/8 × h` at 1bpp, `w/4 × h` at 2bpp) |

**Pixel format (1bpp):**
- Rows are top to bottom, each row is `w/8` bytes.
- Within a byte, the **most significant bit is the leftmost pixel**.
- **Bit set (1) = white, bit clear (0) = black.** A white screen is all `0xFF`.

**Pixel format (2bpp):**
- Rows are top to bottom, each row is `w/4` bytes.
- Within a byte, the **most significant two bits are the leftmost pixel**.
- **0 = black, 1 = dark grey, 2 = light grey, 3 = white.** A white screen is all `0xFF`.
- Regions stay aligned to 8 px, so each region row is a whole number of bytes in both formats.

A panel driver converts this to whatever the controller expects (some invert it).

**Semantics:**
- The device copies the region into its framebuffer (full size, allocated once at boot), then refreshes the panel. A full frame uses a full refresh. A partial frame uses a partial refresh of that region.
- A full frame is always the whole panel (`x = y = 0`, `w × h` = panel size). At 800 × 480 that's 48,000 data bytes at 1bpp or 96,000 at 2bpp, plus the 16-byte header.
- **On a 2bpp panel, a full frame is a 4-grey refresh and a partial frame a black/white partial refresh.** The server guarantees that every pixel that *changes* in a partial frame is black or white before and after; it sends a full frame whenever a grey pixel changes. A partial region may still contain unchanged grey pixels (e.g. a grey rule inside a changed band). For a partial refresh the driver must leave them as they are, e.g. by writing the same black/white value into the controller's old and new RAM for unchanged pixels *(to verify on hardware)*.
- The device rejects frames whose region falls outside the panel, isn't aligned to 8, or doesn't match the payload length (it logs a warning and sends no ack).

## Ack (device → server)

After the panel refresh finishes:

```json
{ "type": "ack", "seq": 17, "crc": "9db65e8e" }
```

- `crc` is the **CRC-32 (IEEE, as in Go's `crc32.ChecksumIEEE`) of the whole framebuffer** after applying the frame, as 8 lowercase hex digits.
- The server compares it with its own copy. If they match, the UI shows the device as "in sync". On a mismatch:
  - **after a partial frame**, the server resyncs once with a full frame;
  - **after a full frame**, the mismatch is treated as systematic (e.g. a checksum bug). The server assumes the device shows the frame, marks it *out of sync* in the UI, and stops resyncing on later partial mismatches until a checksum matches again. This avoids an endless loop of flashing full refreshes. The next scheduled full refresh (at most 20 partial updates or 1 hour later) fixes a one-off error.
- **Flow control:** the server sends the next frame only after the ack for the previous one. This limits updates to what the panel can manage. With no ack within 60 s, the server closes the connection.

## Status (device → server)

Sent right after connecting and then every 60 s:

```json
{ "type": "status", "rssi": -58, "free_heap": 182344, "uptime_s": 3600 }
```

The server shows these values on the device's page in the UI.

## When the server sends what

Settings are in `devicehub.Settings`. Some can be changed with environment variables, see the server README section.

- A re-render happens when an entity used by the layout changes, the layout is saved, or on every wall-clock minute boundary (for clock widgets).
- It waits for a **debounce** of 1 s, and keeps at least **3 s** between frames (`HOMEPAPER_MIN_INTERVAL`).
- If the new frame is identical to what the device shows, **nothing is sent**.
- Otherwise the changed rows are grouped into bands, and each band becomes one byte-aligned region (see `frame.Diff`).
- The server sends a **full frame** instead of partial ones when:
  - it's the first frame after connecting;
  - more than 6 regions changed;
  - more than 50 % of the panel changed;
  - there have been 20 partial updates since the last full one (`HOMEPAPER_FULL_REFRESH_EVERY`), which clears ghosting;
  - more than 1 hour has passed since the last full frame;
  - a partial frame's checksum didn't match (see Ack);
  - on a 2bpp panel, a changed pixel is grey before or after (`frame.ChangedGrey`, [D-018](decisions.md#d-018)).

## Editor live preview

`GET /api/devices/{id}/live` (behind the UI's auth, not the device token) lets an editor tab act as a virtual copy of the display. After the server's `welcome` (which also carries `panel`), it is the same protocol: binary frames in, acks with the framebuffer CRC-32 out. The same session code produces the frames, so the preview gets full and partial frames exactly as the panel would.

Differences from a display:
- **No rate limit:** no debounce and no minimum interval. Each change is rendered at once, held back only by the preview's own acks.
- It doesn't replace the display's connection and doesn't change its status in the UI.

The client is `server/web/ui/src/live.ts`.

## Possible later additions

- An HTTP polling mode for battery devices: `GET /api/device/{id}/frame` with `ETag` / `If-None-Match`, then deep sleep ([O-005](decisions.md#o-005)).
- Over-the-air firmware updates announced over the same socket.
