# Widgets

## Concept

A **layout** belongs to one device and consists of **widget instances** placed on a grid ([P-003](decisions.md#p-003)). Each instance has:

- a **kind**, which decides how it looks and what it expects;
- **bindings** to HA entities and attributes, for the values it shows;
- **options** specific to the kind (label, unit, decimals, icon, size variant, …);
- a **position and size** in grid cells.

Users combine widget kinds with HA values in the web UI ([D-004](decisions.md#d-004)). They don't write code.

## Proposed layout grid

A snap grid such as **8 × 6 cells** on 800 × 480 (100 × 80 px per cell) is proposed. It's still to be decided ([P-003](decisions.md#p-003)). Widgets declare which sizes they support (e.g. 1×1, 2×1, 2×2) and adapt their content to the space.

### Effective grid and broken widgets ([D-021](decisions.md#d-021))

The layout is drawn into the device's **display area**, which can be smaller than the panel. Cells shrink with the area until they would fall below the minimum cell size of **80 × 64 px** (`render.MinCellW/H`, suggested, not yet confirmed); then the grid drops a whole column or row. `render.Grid` computes this **effective grid**, `render.Fits` says whether a widget lies inside it. The API hands `grid`, `area` and `broken` (plus the minimum cell size) to the editor. The one exception: the Display tab repeats the floor-and-clamp to preview the grid of values that aren't applied yet, using the server's minimum cell size.

- A widget outside the effective grid is **broken**: it isn't drawn, but stays stored and comes back when the area grows.
- Layouts are still validated against the stored 8 × 6 grid, so a layout with broken widgets saves.
- The editor shows broken widgets as red dashed outlines over the margin and lists them under the preview, with **Place** (the first free spot at the largest size that fits, down to the kind's minimum), **Edit** and **Delete**. Broken widgets still hold their cells: placing, adding and keyboard moves avoid them, and a drag onto them is undone, so nothing lands on top of them when the area grows.

## v1 catalogue

**Implemented** in `server/internal/render/widgets.go` as `value`, `temperature`, `climate`, `weather`, `lights`, `binary`, `clock` and `text`. The editor builds its forms from the field schema at `GET /api/widgets`, so adding a field in Go is enough. Differences from the original plan:
- **Light** is a list widget (`lights`) and works for one or more entities.
- Widgets have no frame option any more. The layout's **dividers** setting (`rules`, the default, or `frames`) separates them ([D-015](decisions.md#d-015)).
- **Weather** shows today's high and low and a strip of the next days from `weather.get_forecasts` when the widget is two rows tall.
- **Temperature** shows humidity at every size from 2×1. In a one-row widget it sits on the temperature's line at the opposite edge, or beside it when centred. The temperature may step down to 34 px to make room; humidity is dropped only below that, and in 1×1.
- **Alignment** (left, the default; centre; right) on Date / time, Text, Value, Temperature and State moves the label and content as one group. A heading's rule fills the space the text leaves, on both sides when centred. Weather, Lights and Thermostat have column layouts and no alignment.
- **State** takes one or more entities. One shows large, as before. Two or more show as a list: icon, name and state per row, bold when active. A row that asks for action is inverted on its own, never the whole widget. The label becomes a heading over the list. Rows are 40 px at most and 22 px at least; entities that don't fit are left out, not squeezed. Saved widgets keep working, because the `entity` option simply becomes a comma-separated list.
- **Departures** reads a db-infoscreen sensor's `next_departures` list. It uses `sched_datetime` and `rt_datetime`, not `departure_current`, which db-infoscreen appears to compute with the delay counted twice. It shows the actual time and, when late, a small inverted "+3", sorted by real departure. It drops trips that have already left (checked on the per-minute redraw, since db-infoscreen only updates every few minutes) and those leaving within the "walk" minutes. Line and type filters are comma-separated. A line can be limited to one direction with `>` and part of the destination: `2 > Stadtbergen, RB86 > Donauwörth` (case ignored; entries may also be separated by `;`, and a direction can't contain a comma). db-infoscreen's own Direction, Excluded directions and Favorite trains filters don't work with regional data sources (v2026.11.0 checks `direction` and `train` fields that aren't in the data), so filter here and raise the integration's number of departures instead. Cancelled trips get a hollow line sign and a struck-through time. The heading (default: the station) shows from two rows tall, with "as of" the sensor's last update from 3 cells wide.
- **Alarm panels** (`alarm_control_panel`) in the State widget read "Disarmed", "Armed away", "Arming", "Triggered" and so on, falling back to a short word ("Away") where the long one doesn't fit. The shield icon is hollow when disarmed, half-filled while arming, disarming or pending, and solid when armed or triggered. Alarms are not drawn inverted.
- **Text** sizes: small is a one-line note, medium a heading followed by a rule, large a big bold line.
- Doors, windows and garages that are open, and unlocked locks, are drawn **inverted**. Nothing else is.
- Fields can carry a `placeholder` (what an empty input means) and a one-line `help`; select `choices` are `{value, label}` pairs, so the editor shows "Date and time (Mon 2 Jan, 15:04)" instead of `datetime`. An empty **Decimals** shows the value as HA reports it.
- The editor flags widgets whose required entity is missing or unknown, on the preview and in the form, instead of leaving the user to puzzle over a dash.
- An unavailable or unknown entity is drawn as "–".


This list comes from the initial idea: current temperature, weather, sensor data in general, light status and target temperature.

| Kind | Shows | Typical bindings |
|---|---|---|
| **Value** (generic sensor) | One value with label, unit and optional icon | Any `sensor.*`, or an entity attribute |
| **Temperature** | Current temperature, optionally with humidity | `sensor.*_temperature`, `sensor.*_humidity` |
| **Climate** | Current and target temperature, HVAC mode | `climate.*` (`current_temperature`, `temperature`, `hvac_action`) |
| **Weather** | Condition icon, temperature, short forecast | `weather.*` (forecast through the HA `weather.get_forecasts` service) |
| **Light** | On/off state, optionally brightness. Can also list several lights | `light.*` |
| **Binary state** | Open/closed, on/off, present/away, alarm armed/disarmed, with icon | `binary_sensor.*`, `switch.*`, `person.*`, `lock.*`, `cover.*`, `alarm_control_panel.*` |
| **Departures** | Next buses, trams and trains: line sign, destination, platform, actual time, delay tag | a [db-infoscreen](https://github.com/FaserF/ha-db_infoscreen) sensor (`next_departures`) |
| **Clock / date** | Date, time or both. The server re-renders every minute, so a time display costs one small partial refresh per minute. | none |
| **Text / heading** | Static label or section title | none |

Possible later additions: history chart or sparkline, calendar or agenda, waste collection, energy and solar, multi-entity tables, conditional widgets (shown only when e.g. a window is open).

## Open points

- ~~The schema for widget definitions~~: done (`render.Kind` / `render.Field`).
- ~~How to show unavailable or unknown entity states~~: shown as "–" for now.
- Per-widget minimum update intervals ([D-005](decisions.md#d-005)). Only a per-device minimum interval exists so far.
- ~~Themes, fonts and icon set~~: the panel redesign ([D-015](decisions.md#d-015)), implemented. [design.md](design.md#widgets) has each kind's layout at every size and what it drops first.
