# Vision

## The idea

homepaper is **the newspaper of your smart home**: a calm, always-on e-ink display, for example in a picture frame on the wall, showing what matters at a glance. That includes temperatures, weather, sensor readings, which lights are on, thermostat targets and more.

The name is *home* (Home Assistant, smart home) plus *paper* (newspaper, e-paper).

## Goals

- **Configurable without code.** The user builds the screen in a web UI by choosing widget kinds and binding them to Home Assistant entities ([D-004](decisions.md#d-004)).
- **Thin device.** The ESP32 only receives prerendered frames and drives the panel. Changing layouts or adding widgets never requires reflashing ([D-003](decisions.md#d-003)).
- **Timely but calm.** Changes in HA reach the display through push updates, rate-limited and diffed so the e-ink panel isn't refreshing all the time ([D-005](decisions.md#d-005)).
- **Self-hosted.** The server runs in the user's own infrastructure, next to Home Assistant.
- **Looks good on e-ink.** Crisp 1-bit typography and a layout designed for paper, not a shrunken web dashboard.

## Non-goals (for now)

- **Interaction** on the device (buttons, touch, controlling entities) ([D-006](decisions.md#d-006)).
- **Battery operation.** The prototype runs on USB power. A deep-sleep polling mode is kept possible but isn't planned ([O-005](decisions.md#o-005)).
- **Color panels.** The protocol stays panel-agnostic, but v1 targets black/white ([D-007](decisions.md#d-007)).
- **Rendering on the device.**

## Target user

A Home Assistant user who is comfortable running a Docker container and flashing an ESP32 once, and who wants a nice-looking status display without writing display code.
