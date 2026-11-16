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

## v1 catalogue

**Implemented** in `server/internal/render/widgets.go` as `value`, `temperature`, `climate`, `weather`, `lights`, `binary`, `clock` and `text`. The editor builds its forms from the field schema at `GET /api/widgets`, so adding a field in Go is enough. Differences from the original plan:
- **Light** is a list widget (`lights`) and works for one or more entities.
- Widgets have no frame option any more. The layout's **dividers** setting (`rules`, the default, or `frames`) separates them ([D-015](decisions.md#d-015)).
- **Weather** shows today's high and low and a strip of the next days from `weather.get_forecasts` when the widget is two rows tall.
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
| **Binary state** | Open/closed, on/off, present/away with icon | `binary_sensor.*`, `switch.*`, `person.*` |
| **Clock / date** | Date, time or both. The server re-renders every minute, so a time display costs one small partial refresh per minute. | none |
| **Text / heading** | Static label or section title | none |

Possible later additions: history chart or sparkline, calendar or agenda, waste collection, energy and solar, multi-entity tables, conditional widgets (shown only when e.g. a window is open).

## Open points

- ~~The schema for widget definitions~~: done (`render.Kind` / `render.Field`).
- ~~How to show unavailable or unknown entity states~~: shown as "–" for now.
- Per-widget minimum update intervals ([D-005](decisions.md#d-005)). Only a per-device minimum interval exists so far.
- ~~Themes, fonts and icon set~~: the panel redesign ([D-015](decisions.md#d-015)), implemented. [design.md](design.md#widgets) has each kind's layout at every size and what it drops first.
