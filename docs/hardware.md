# Hardware

*(verified)* means the fact was checked on a product page. *(to verify)* means it comes from memory or inference and must be checked before code depends on it.

## Panel

**Waveshare 7.5" e-Paper, 800×480, black/white** ([D-007](decisions.md#d-007)).

| Property | Value | Source |
|---|---|---|
| Resolution | 800 × 480 | *(verified)* Waveshare product page |
| Colors | Black/white, 4 grey levels | *(verified)* |
| Full refresh | 4 s | *(verified)* |
| Fast refresh | 1.5 s | *(verified)* |
| Partial refresh | Supported, 0.4 s | *(verified)* |
| 4-grey refresh | 2.1 s | *(verified)* |
| Partial refresh in 4-grey mode | Not offered (Waveshare lists partial refresh for black/white only) | *(to verify)*: homepaper assumes it and uses black/white partials over a grey image ([D-018](decisions.md#d-018)) |
| Outline | 170.2 × 111.2 mm | *(verified)* |
| Display area | 163.2 × 97.92 mm | *(verified)* |
| Interface | SPI | *(verified)* |
| Revision | V2 | *(to verify)*: inferred because V1 was 640×384. The product pages don't say "V2". |
| Rust driver | `epd-waveshare` crate, module `epd7in5_v2` | *(verified, docs.rs, v0.6)*: partial refresh support *(to verify)* |
| GxEPD2 driver class (C++ reference for init sequences) | `GxEPD2_750_T7` | *(to verify)* |
| Ribbon-cable marking | e.g. `GDEW075T7` or `HINK-E075A01` | *(to verify)*: check when the panel arrives |

### Panels considered and rejected for v1

**7.5" 800×480 black/white/red ("(B)" variant).** It has no partial refresh and a full refresh takes about 15–25 s with flashing. Waveshare's guidance for color panels is also a minimum time between refreshes of roughly 3 minutes *(to verify)*. That conflicts with push updates, so it's an option for a later "accent color" version, not the prototype ([D-007](decisions.md#d-007)). The protocol stays panel-agnostic so it can be added later.

### Panel driver requirements for grey mode

The driver isn't written yet (the firmware uses `NullPanel`). For [D-018](decisions.md#d-018) it needs:
- a 4-grey full refresh for full frames: Waveshare's C demo has `EPD_7IN5_V2_Init_4Gray` and a 4-grey display routine with their own LUTs. Whether the `epd-waveshare` crate's `epd7in5_v2` supports 4 greys is *(to verify, likely not)*, so the init and LUTs may have to be ported;
- a black/white partial refresh for partial frames, which must leave unchanged grey pixels alone, e.g. by writing the same black/white value into the controller's old and new RAM for them *(to verify)*;
- memory: two 96 KB buffers on the ESP32-S3. Check `free_heap` in the status message after boot.

## Controller: two routes

See [O-001](decisions.md#o-001).

> **Important:** the **raw panel** has only a ribbon (FPC) connector. You **can't** jumper it to an ESP32 dev board directly. It always needs a driver PCB: either Waveshare's driver HAT (route A) or the ESP32 driver board (route B).

### Route A: HAT kit + ESP32-S3 dev board

**This is the chosen route** ([D-020](decisions.md#d-020)). It matches the original prototype idea (S3 dev board + jumpers, [D-008](decisions.md#d-008)).

- **Waveshare 7.5inch e-Paper HAT**, SKU 13504 *(verified)*. Waveshare lists the contents as the panel, the e-Paper Driver HAT (with a 3.3 V/5 V level shifter), an RPi screw pack and a GH1.25 9-pin cable of ~20 cm *(verified, Waveshare product page, 2026-11-18)*. Whether the cable's far end has loose wires or female jumper plugs is *(to verify)* when it arrives.
- Pins: VCC, GND, DIN, CLK, CS, DC, RST, BUSY, PWR *(verified)*. These are jumpered to the S3's SPI-capable GPIOs.
- **Bought from Eckstein:** [WaveShare 7.5inch E-Ink Display HAT 800x480 for Raspberry Pi](https://eckstein-shop.de/WaveShare800C3974802C75inchE-InkdisplayHATforRaspberryPi), art. no. WS13504, €67.82 incl. VAT, about 3 weeks delivery *(verified, 2026-11-18)*. The listing doesn't state the package contents. Waveshare's own price is $54.59.
- Don't confuse it with the 640×384 V1 HAT, the (B) red/black/white HAT (SKU 13505), the (H) four-colour HAT, the 880×528 "HD" HATs or the raw panel without driver board. Some shops list SKU 13504 for the raw panel.

**The owner's ESP32-S3 dev board** *(verified with `esptool chip-id`/`flash-id`, 2026-11-18)*: ESP32-S3 (QFN56) revision v0.2, 16 MB quad flash, **8 MB embedded PSRAM** (`AP_3v3`), i.e. the N16R8 variant. In-package 8 MB PSRAM at 3.3 V is normally octal *(to verify)*. The firmware doesn't enable PSRAM yet (`sdkconfig.defaults` has no `CONFIG_SPIRAM*`).
- Pros: cheap if an S3 is already on hand, and the S3 has more RAM (plus PSRAM, depending on the board). Cons: wires and two boards in the frame.

**Planned wiring for route A** ([P-008](decisions.md#p-008)). The firmware doesn't drive the panel yet. These are the pins the panel driver will use:

| HAT pin | ESP32-S3 GPIO | Note |
|---|---|---|
| VCC | 3V3 | |
| GND | GND | |
| DIN (MOSI) | 11 | SPI2 (FSPI) default pins |
| CLK (SCLK) | 12 | |
| CS | 10 | |
| DC | 9 | |
| RST | 8 | |
| BUSY | 7 | input |
| PWR | 6 | panel power enable on newer HAT revisions, drive high |

These avoid the strapping pins (0, 3, 45, 46), the USB pins (19, 20) and the pins used by octal flash/PSRAM (26–37) *(to verify against the owner's board pinout)*.

### Route B: raw panel + Waveshare Universal e-Paper Driver Board ESP32

- **Raw panel**: Eckstein WS13187, "7.5inch E-Ink Raw Display 800x480", €57.11 including VAT, about 8–10 days delivery *(verified, 2026-11-09)*. The listing's reviews from 2017/18 claim there's no partial refresh. They predate the 800×480 panel and are considered outdated.
- **Driver board**: €14.90 at Berrybase, 1–3 days delivery (from the user's research).
  - MCU: ESP32-WROOM-32 without PSRAM *(to verify)*. The firmware now keeps a 96 KB 2bpp framebuffer plus a 96 KB WebSocket receive buffer ([D-018](decisions.md#d-018)). Whether both fit next to Wi-Fi and TLS without PSRAM is *(to verify)*.
  - Pin mapping: GxEPD2's examples include this board and serve as a reference for our Rust pin config *(to verify)*.
  - Has an **A/B switch** for the panel's driving resistor. Check the Waveshare wiki for the 7.5" V2 before the first power-on *(to verify)*.
- Pros: no wiring, one compact board, tidy in a frame. Cons: ~€72 total, and the less powerful MCU.

The panel is the same in both routes. A panel bought with the HAT kit can later be moved to the ESP32 driver board.

Firmware keeps pin assignments in a per-board config, so both routes run the same code.

## Frame build

- **Frame**: 13 × 18 cm. It fits the panel's 170.2 × 111.2 mm outline.
- **Passe-partout**: window about **163 × 98 mm**, with a few mm of overlap so the panel's border is covered. It probably needs to be cut by hand or ordered to size.
- **Depth**: the electronics (about 48 × 29 mm for the route B board, or the S3 and HAT for route A), the ribbon cable and the USB cable need a **box frame** or a spacer behind the back panel.
- **The ribbon cable is fragile.** Don't fold it sharply, and secure the board so it can't pull on the cable.
- **Power**: the device needs constant USB power because it keeps a WebSocket open ([O-005](decisions.md#o-005)). A flat right-angle USB cable looks tidiest.

## Shopping list (route A, chosen)

| Item | Where | Price |
|---|---|---|
| Waveshare 7.5inch e-Paper HAT, SKU 13504 | [Eckstein](https://eckstein-shop.de/WaveShare800C3974802C75inchE-InkdisplayHATforRaspberryPi) | €67.82 |
| ESP32-S3 dev board (N16R8) | on hand | – |
| Jumper wires, if the HAT cable's ends need them | Any | a few € |
| Box frame 13×18 cm + passe-partout (cut to ~163 × 98 mm) | Any | ~€10–15 |
| USB power supply + cable (flat, right-angle) | Any | a few € |

## Shopping list (route B example)

| Item | Where | Price |
|---|---|---|
| Waveshare Universal e-Paper Driver Board ESP32 | Berrybase | €14.90 |
| Waveshare 7.5" 800×480 B/W raw panel (WS13187) | Eckstein | €57.11 |
| Box frame 13×18 cm + passe-partout (cut to ~163 × 98 mm) | Any | ~€10–15 |
| USB cable (flat, right-angle) | Any | a few € |
| **Total** | | **~€85–90** |

For route A, buy the HAT kit instead of the first two items, plus an ESP32-S3 dev board if one isn't on hand.
