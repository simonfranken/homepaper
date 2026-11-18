//! Framebuffer and panel abstraction.
//!
//! The framebuffer uses the wire format from docs/device-protocol.md: 2 bits
//! per pixel (`2bpp`), rows byte-aligned, the most significant bits are the
//! leftmost pixel, 0 = black, 1 = dark grey, 2 = light grey, 3 = white. A
//! panel driver turns it into whatever the controller expects: a full frame
//! is a 4-grey refresh, a partial frame a black/white one (the server never
//! changes grey pixels in a partial frame, D-018).

use crate::protocol::{Region, KIND_FULL};

pub const WIDTH: usize = 800;
pub const HEIGHT: usize = 480;
/// Bits per pixel.
pub const DEPTH: usize = 2;
pub const STRIDE: usize = WIDTH * DEPTH / 8;
pub const FB_SIZE: usize = STRIDE * HEIGHT;

/// Reported to the server in the hello message.
pub const PANEL_FORMAT: &str = "2bpp";

pub struct Framebuffer {
    pub pix: Vec<u8>,
}

impl Framebuffer {
    /// Allocated once at boot (all white) and never resized.
    pub fn new() -> Self {
        Self { pix: vec![0xFF; FB_SIZE] }
    }

    /// Copies a region's packed rows into the framebuffer.
    pub fn apply(&mut self, r: &Region, data: &[u8]) {
        let stride = r.w as usize * DEPTH / 8;
        for row in 0..r.h as usize {
            let dst = (r.y as usize + row) * STRIDE + r.x as usize * DEPTH / 8;
            self.pix[dst..dst + stride].copy_from_slice(&data[row * stride..(row + 1) * stride]);
        }
    }

    pub fn crc32(&self) -> u32 {
        crc32(&self.pix)
    }
}

/// A display panel. `show` refreshes the panel from the framebuffer; for a
/// partial update only `region` changed.
pub trait Panel {
    fn model(&self) -> &'static str;
    fn show(&mut self, fb: &Framebuffer, region: &Region, kind: u8) -> anyhow::Result<()>;
}

/// Stand-in until the e-ink panel is connected: logs what a real panel would
/// do and prints full frames as ASCII art to the serial monitor.
pub struct NullPanel;

impl Panel for NullPanel {
    fn model(&self) -> &'static str {
        "none-800x480"
    }

    fn show(&mut self, fb: &Framebuffer, region: &Region, kind: u8) -> anyhow::Result<()> {
        if kind == KIND_FULL {
            log::info!("[panel] full refresh\n{}", ascii_art(fb));
        } else {
            log::info!("[panel] partial refresh x={} y={} w={} h={}", region.x, region.y, region.w, region.h);
        }
        Ok(())
    }
}

/// Downsamples the framebuffer to 100x30 characters (8x16 px per cell).
fn ascii_art(fb: &Framebuffer) -> String {
    const CW: usize = 8;
    const CH: usize = 16;
    let ramp = [' ', '.', ':', '+', '#', '@'];
    let mut out = String::with_capacity((WIDTH / CW + 1) * (HEIGHT / CH));
    for cy in 0..HEIGHT / CH {
        for cx in 0..WIDTH / CW {
            // Darkness 0..3 per pixel (3 = black), summed over the cell.
            let mut black = 0u32;
            for y in cy * CH..(cy + 1) * CH {
                // Two bytes are exactly the 8 pixels of this cell's row.
                for b in &fb.pix[y * STRIDE + cx * 2..y * STRIDE + cx * 2 + 2] {
                    for i in 0..4 {
                        black += 3 - ((b >> (6 - 2 * i)) & 3) as u32;
                    }
                }
            }
            let max = (CW * CH * 3) as usize;
            let idx = (black as usize * (ramp.len() - 1) + max / 2) / max;
            out.push(ramp[idx.min(ramp.len() - 1)]);
        }
        out.push('\n');
    }
    out
}

/// CRC-32 (IEEE 802.3), as Go's `crc32.ChecksumIEEE`.
pub fn crc32(data: &[u8]) -> u32 {
    static TABLE: std::sync::OnceLock<[u32; 256]> = std::sync::OnceLock::new();
    let table = TABLE.get_or_init(|| {
        let mut t = [0u32; 256];
        for (i, e) in t.iter_mut().enumerate() {
            let mut c = i as u32;
            for _ in 0..8 {
                c = if c & 1 != 0 { 0xEDB8_8320 ^ (c >> 1) } else { c >> 1 };
            }
            *e = c;
        }
        t
    });
    let mut crc = !0u32;
    for &b in data {
        crc = table[((crc ^ b as u32) & 0xFF) as usize] ^ (crc >> 8);
    }
    !crc
}
