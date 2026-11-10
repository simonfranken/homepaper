# Rendering

All rendering happens on the server ([D-003](decisions.md#d-003)). The device receives panel-native pixel data.

## Pipeline

```
layout + widget configs + entity states
        │
        ▼
  widget rendering (vector, at panel resolution)
        │
        ▼
  rasterize ──► greyscale bitmap
        │
        ▼
  convert to panel colors:
    text and lines → threshold
    images and icons → dither
        │
        ▼
  pack to panel format (1bpp) ──► diff with last frame ──► changed regions
```

## Renderer technology ([D-011](decisions.md#d-011))

The renderer is **native Go**, in the backend:

- Each widget kind is a Go type that draws into an image of its grid area (`golang.org/x/image` with opentype/font, and/or `fogleman/gg`).
- A small layout helper (padding, alignment, text fitting, stacking) keeps widget code from repeating pixel arithmetic.
- **The preview is the device frame.** The editor gets the rendered frame as a PNG, updated live over WebSocket, and lays its editable grid over it. The frontend never draws widgets, so the preview can't differ from the panel.
- Headless Chromium (chromedp) and a Satori/resvg sidecar were rejected, see D-011.

**As implemented** (`server/internal/render`):
- It draws anti-aliased with `fogleman/gg` into an RGBA canvas, converts to greyscale, and thresholds at 128 into a `frame.Bitmap`.
- Text uses the embedded Go fonts, with full hinting ([P-013](decisions.md#p-013)).
- `ctx.text` fits text into a rectangle by shrinking the font and places it by cap height, so digits sit optically centred.
- Icons (weather, bulb, status dot) are solid shapes and thick strokes, drawn in code, with no image assets.
- All rendering is serialized by one mutex, because font faces aren't safe for concurrent use. A render takes milliseconds.

## E-ink considerations

- **1-bit typography.** Anti-aliased text breaks up when converted to pure black and white. Prefer **bitmap fonts (BDF)** at fixed sizes. For large display numbers, use vector fonts with a hard threshold (bold enough at the chosen sizes). **Never dither text.**
- **Dither only images and icons** (e.g. weather icons, charts with fills). Floyd–Steinberg or ordered dithering.
- **Design for paper**: high contrast, generous whitespace, no thin grey lines.
- **Grey mode** (4 levels, 2bpp) is possible but has no partial refresh. That makes it a later option, not the default ([O-003](decisions.md#o-003)).

## Update strategy ([D-005](decisions.md#d-005))

- **Debounce**: re-render a device at most every X seconds, batching changes that arrive close together.
- **Per-widget minimum interval**: e.g. a power sensor that updates every second only redraws every 5 minutes.
- **Skip unchanged frames**: if the packed bitmap is identical to the last one sent (e.g. 21.43 → 21.44 °C shown as "21.4°"), send nothing.
- **Region diff**: compare the new frame with the last one sent, merge changed pixels into a few byte-aligned rectangles, and send partial frames.
- **Ghosting control**: a full refresh every N partial updates and/or at least every M minutes, configurable per device.
- **Back-pressure**: wait for the device's ack before sending the next frame ([device-protocol.md](device-protocol.md)).

## Debugging

- `GET /api/devices/{id}/preview.png` returns the current frame as a PNG. The editor shows this.
- `server/cmd/fakedevice -png out.png` connects like a device and writes its reconstructed framebuffer after every frame.
- The firmware prints full frames as ASCII art (100 × 30 characters) to the serial monitor while no panel is connected.
- Later: an editor overlay highlighting regions that changed since the last frame.
