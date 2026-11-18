//! Wire format, see docs/device-protocol.md.

use anyhow::{bail, Result};

pub const MAGIC: u16 = 0x5048; // "HP"
pub const VERSION: u8 = 1;
pub const HEADER_SIZE: usize = 16;
pub const KIND_FULL: u8 = 0;
pub const KIND_PARTIAL: u8 = 1;

#[derive(Debug, Clone, Copy)]
pub struct Region {
    pub x: u16,
    pub y: u16,
    pub w: u16,
    pub h: u16,
}

#[derive(Debug, Clone, Copy)]
pub struct Header {
    pub kind: u8,
    pub seq: u32,
    pub region: Region,
}

fn u16_at(b: &[u8], i: usize) -> u16 {
    u16::from_le_bytes([b[i], b[i + 1]])
}

/// Validates a frame against the panel size and returns its header and payload.
pub fn parse(buf: &[u8], width: usize, height: usize) -> Result<(Header, &[u8])> {
    if buf.len() < HEADER_SIZE {
        bail!("frame too short ({} bytes)", buf.len());
    }
    if u16_at(buf, 0) != MAGIC || buf[2] != VERSION {
        bail!("bad magic or version");
    }
    let h = Header {
        kind: buf[3],
        seq: u32::from_le_bytes([buf[4], buf[5], buf[6], buf[7]]),
        region: Region { x: u16_at(buf, 8), y: u16_at(buf, 10), w: u16_at(buf, 12), h: u16_at(buf, 14) },
    };
    let r = h.region;
    if r.x % 8 != 0 || r.w % 8 != 0 || r.x as usize + r.w as usize > width || r.y as usize + r.h as usize > height {
        bail!("region {r:?} outside the panel or not byte-aligned");
    }
    let data = &buf[HEADER_SIZE..];
    if data.len() != r.w as usize * crate::display::DEPTH / 8 * r.h as usize {
        bail!("payload {} bytes does not match region {r:?}", data.len());
    }
    if h.kind != KIND_FULL && h.kind != KIND_PARTIAL {
        bail!("unknown frame kind {}", h.kind);
    }
    Ok((h, data))
}
