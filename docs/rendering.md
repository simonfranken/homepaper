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
- It draws shapes anti-aliased with `fogleman/gg` into an RGBA canvas, converts to greyscale, and thresholds at 128 into a `frame.Bitmap`. Widget rectangles are rounded to whole pixels.
- Text ([P-013](decisions.md#p-013)):
  - **Bitmap fonts (Lucida Sans BDF, `bdf.go`) up to 25 px and at 34 px.** They are drawn pixel for pixel. Glyphs a font lacks come from the Go font on the same baseline.
  - **The Go fonts above that**, with hinting, then thresholded.
  - Text is drawn with `font.Drawer` straight onto the canvas, not through `gg`, which resamples every glyph. It sits on whole pixels: a fractional origin made the threshold eat stems unevenly. It is clipped to its widget.
- `ctx.text` fits text into a rectangle by picking from a fixed size scale (`sizeSteps`), not by shrinking continuously. It places text by cap height, so digits sit optically centred.
  - Content shrinks down to 12 px, then is shortened with "...".
  - Labels are 17 px, may step down to 14 px, and are then shortened.
  - Widgets drop secondary text before they cut primary text (e.g. weather drops humidity).
- Icons (weather, bulb, status dot) are solid shapes and thick strokes, drawn in code, with no image assets.
- All rendering is serialized by one mutex, because font faces aren't safe for concurrent use. A render takes milliseconds.

## Panel design ([D-015](decisions.md#d-015))

The renderer draws the design in [design.md](design.md#panel-d-015) (implemented 2026-11-16). `kinds.go` holds the widgets and `draw.go` the primitives:
- layout-level dividers (`Layout.Dividers`: column rules by default, frames as an option), replacing the per-widget `border` option, which the API now drops;
- inverted widgets for states that need action;
- drawn triangles and arrows;
- numbers with a small raised decimal;
- brightness meters and dotted row separators;
- state icons (door, window, lock, person, droplet, flame, bolt).

`TestGolden` renders the example layout with demo data at Mon 16 Nov 2026 18:42 and compares it pixel for pixel with `docs/design/hero-*.png`. After an intended change, `go test ./internal/render -run Golden -update` rewrites those images. Look at them before committing. `TestEveryKindEverySize` renders every kind at every size, with its entities present and unavailable, and fails on ink outside the cell. Set `HOMEPAPER_RENDER_OUT=dir` to write the images out for a look.

## E-ink considerations

- **1-bit typography.** Anti-aliased text breaks up when converted to pure black and white. Prefer **bitmap fonts (BDF)** at fixed sizes (implemented, see above). For large display numbers, use vector fonts with a hard threshold (bold enough at the chosen sizes). **Never dither text.**
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

- The editor preview is a **live virtual display** (`/api/devices/{id}/live`, see [device-protocol.md](device-protocol.md#editor-live-preview)). It gets the same pushed full and partial frames as the panel, but without the rate limit. Each refreshed region is outlined in blue for a moment, and a line under the preview names the last frame (number, kind, size, bytes, time).
- `GET /api/devices/{id}/preview.png` returns the current frame as a PNG. The editor falls back to it while the live connection is down.
- `server/cmd/fakedevice -png out.png` connects like a device and writes its reconstructed framebuffer after every frame.
- The firmware prints full frames as ASCII art (100 × 30 characters) to the serial monitor while no panel is connected.
- Later: an editor overlay highlighting regions that changed since the last frame.
