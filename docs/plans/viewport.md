# Plan: display area (viewport) with margin

Status: **implemented 2026-11-18** (server 0.2.0), with the suggested 80 × 64 px minimum cell size still to be confirmed and no width rounding. Decision: [D-021](../decisions.md#d-021). Written 2026-11-18.

## Goal

The panel's aspect ratio may not match the opening of the picture frame it ends up in. The user can shrink the area homepaper draws on, so the panel gets a white margin and the content fills the opening exactly.

- By default the area is the full resolution the device reports in `hello`.
- The user can make it smaller, but never larger than the panel.
- The content is centered by default and can be moved with an optional offset, because panels and frame openings are rarely exactly centered on each other.
- Grid cells shrink with the area. If a cell would fall below a minimum size, the grid loses a whole column or row.
- Widgets that no longer fit stay stored and show in the editor as "broken" until the user fixes them.

## Decided (2026-11-18)

1. **Widgets that no longer fit** are not drawn on the panel, but stay stored. The editor shows them as broken with a short reason. They come back on their own when the area grows again.
2. **The breakpoint is a minimum cell size.** The layout's grid stays 8 × 6. The user can't change the number of rows and columns, so the only thing that reduces them is the area dropping below the minimum cell size.
3. **Placement:** centered, with an optional X/Y offset.

## Still to decide

- **Minimum cell size.** Today's cells are 100 × 80 px. The suggestion is **80 × 64 px**: a column drops below 640 px width, a row below 384 px height.
- **Width alignment.** The suggestion is to round the width down to a multiple of 8. It isn't strictly needed, because full frames always cover the whole panel and diff regions are aligned separately, but it keeps the numbers clean.

## Design

### Data (`internal/store`)

A new field on `Device`, not on `Layout`, because it describes the physical setup:

```go
type Viewport struct {
    W       int `json:"w,omitempty"`        // 0 = full panel width
    H       int `json:"h,omitempty"`        // 0 = full panel height
    OffsetX int `json:"offset_x,omitempty"` // px from centered, may be negative
    OffsetY int `json:"offset_y,omitempty"`
}
```

- A helper `Device.Area() image.Rectangle` resolves it against `Panel`: it clamps W and H to the panel, centers the area, applies the offset, and clamps it so it stays inside the panel.
- If a device re-registers with a different resolution, the stored viewport is kept and clamped by `Area()`. It is never silently reset.
- Stored data stays compatible, because an empty viewport means the full panel. That makes it a MINOR bump.

### Effective grid (`internal/render`)

```go
const MinCellW, MinCellH = 80, 64 // to be confirmed
func Grid(layout store.Layout, area image.Rectangle) (cols, rows int)
// cols = clamp(floor(area.W / MinCellW), 1, layout.Cols), rows likewise
func Fits(wg store.Widget, cols, rows int) bool
```

- `Grid` and `Fits` are the only places where this rule lives. The renderer, the API and the editor all use them, so the editor never needs its own copy of the numbers.
- Cell edges are computed from the area's size, as they are today from the panel's size.
- Widgets already adapt in pixels (`small(r)`, `r.W >= 300` etc.), so smaller cells need no widget changes.

### Rendering

- `Render` takes the area. It draws the layout into a canvas the size of the area, exactly as it does today. That includes the grey layers and the rules, which use the area's edges and not the panel's.
- It then copies the result onto a white full-panel image at the area's position before converting it to 1bpp or 2bpp.
- Widgets for which `Fits` is false are skipped.
- **The device protocol doesn't change:** frames still cover the panel, the margin is constant and so never shows up in a diff. There's no protocol version bump and no firmware change.
- Changing the area changes most of the frame, so the next frame is a full refresh by the existing rule (more than 50 % changed).

### API (`internal/api`)

- `PATCH /api/devices/{id}` accepts `viewport`. It checks that W and H are 0 or between 1 cell and the panel size, and that the offset keeps the area on the panel. Then it wakes the device and tells editors to re-render.
- The device DTO gains computed fields, so the UI doesn't reimplement the rule:
  - `area` (x, y, w, h)
  - `grid` (cols, rows)
  - `broken`: the IDs of widgets outside the effective grid
- `validateLayout` keeps checking against the stored 8 × 6 grid, so a layout with broken widgets can still be saved.

### Editor (`web/ui`)

- **Display settings** (the Display tab, `DeviceView.vue`):
  - width and height, with "Full panel" as a reset;
  - X and Y offsets, with "Center" as a reset;
  - the size in mm next to the pixels, using the panel's pixel pitch (*to verify* from the datasheet);
  - a note when the grid has lost a column or row ("7 × 6 cells, 1 column fewer below 640 px").
- **Calibration:** a toggle that draws a 1 px border at the area's edges in the rendered frame, so the user can line it up with the real frame opening. It's off by default and only changes the frame while it's on.
- **Layout editor** (`Editor.vue`):
  - The preview still shows the whole panel, with the margin hatched in editor blue (`design.md`: blue never reaches the panel).
  - The grid overlay covers only the area: `colWidth`, `rowHeight`, `col-num` and `max-rows` come from `device.area` and `device.grid`, not from the panel and the layout.
  - `fits()` and free-spot search use the effective grid, so moving or resizing a widget keeps it inside it.
- **Broken widgets:**
  - Each one is listed in a "Doesn't fit at this display size" strip under the preview, with its kind and label.
  - Each one has **Place** (moves it to the first free spot, at a smaller size if needed), **Edit** and **Delete**.
  - The widget list marks them with the existing warning style, the same flag used for widgets without an entity.
- The live preview (`live.ts`) decodes full-panel frames already, so it needs no change.

### Tests

- `render`:
  - The default area gives byte-identical output to today, so the golden files don't change.
  - A smaller centered area gives white margins and the same content as a panel of that size.
  - Negative and positive offsets work, and the area is clamped at the panel edges.
  - At 639 px width the grid is 7 columns and a widget in column 8 isn't drawn.
- `store`: `Area()` clamps a viewport stored for a larger panel after a smaller one registers.
- `api`:
  - `PATCH` validation;
  - the DTO's `area`, `grid` and `broken`;
  - a layout with broken widgets saves.
- `devicehub`: a viewport change wakes the session and the next frame is full. Check with `fakedevice` too.
- UI: `vue-tsc` passes, then a manual check in the browser: resize, offset, the broken strip, and calibration on and off.

### Docs

- `decisions.md`: D-021. Fill in the minimum cell size once it's chosen, and mark it implemented when it is.
- `widgets.md`: the effective grid and broken widgets.
- `rendering.md`: the area and the margin.
- `design.md`: the hatched margin and the broken-widget strip.
- `device-protocol.md`: one sentence saying frames always cover the full panel and the margin is drawn by the server.
- `roadmap.md`: the status line.
- `VERSION`: MINOR bump.

## Order of work

1. Store `Viewport` and `Area()`, and `render.Grid`/`Fits` with tests.
2. Render into the area and place it on the panel. Check that the golden files are unchanged.
3. API: the `PATCH` field, the DTO fields and the device wake-up.
4. Editor: the grid overlay from the area, the hatched margin, the effective grid in `fits()`.
5. Editor: display settings, calibration toggle, broken-widget strip.
6. Docs, `VERSION`, check with `fakedevice` and in the browser.
