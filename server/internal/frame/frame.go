// Package frame holds the panel-native 1-bit bitmap format, region diffing
// and the binary wire encoding defined in docs/device-protocol.md.
package frame

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
)

// Bitmap is a packed 1 bit per pixel image. Rows are byte-aligned, the most
// significant bit is the leftmost pixel, and a set bit means white.
type Bitmap struct {
	W, H int
	Pix  []byte
}

// New returns an all-white bitmap. Width must be a multiple of 8.
func New(w, h int) *Bitmap {
	b := &Bitmap{W: w, H: h, Pix: make([]byte, w/8*h)}
	for i := range b.Pix {
		b.Pix[i] = 0xFF
	}
	return b
}

// Stride is the number of bytes per row.
func (b *Bitmap) Stride() int { return b.W / 8 }

// FromGray thresholds a greyscale image into a bitmap: pixels darker than
// threshold become black.
func FromGray(img *image.Gray, threshold uint8) *Bitmap {
	r := img.Bounds()
	b := New(r.Dx(), r.Dy())
	for y := 0; y < b.H; y++ {
		row := img.Pix[(y)*img.Stride:]
		for x := 0; x < b.W; x++ {
			if row[x] < threshold {
				b.Pix[y*b.Stride()+x/8] &^= 0x80 >> (x % 8)
			}
		}
	}
	return b
}

// ToGray expands the bitmap to black/white greyscale, e.g. for PNG previews.
func (b *Bitmap) ToGray() *image.Gray {
	img := image.NewGray(image.Rect(0, 0, b.W, b.H))
	for y := 0; y < b.H; y++ {
		for x := 0; x < b.W; x++ {
			if b.Pix[y*b.Stride()+x/8]&(0x80>>(x%8)) != 0 {
				img.Pix[y*img.Stride+x] = 0xFF
			}
		}
	}
	return img
}

// CRC32 is the IEEE checksum of the whole bitmap, compared against the
// checksum a device reports in its ack.
func (b *Bitmap) CRC32() uint32 { return crc32.ChecksumIEEE(b.Pix) }

// Clone returns a deep copy.
func (b *Bitmap) Clone() *Bitmap {
	c := &Bitmap{W: b.W, H: b.H, Pix: make([]byte, len(b.Pix))}
	copy(c.Pix, b.Pix)
	return c
}

// Region is a rectangle in pixels. X and W are multiples of 8.
type Region struct{ X, Y, W, H int }

// Full is the region covering the whole bitmap.
func (b *Bitmap) Full() Region { return Region{0, 0, b.W, b.H} }

// Area in pixels.
func (r Region) Area() int { return r.W * r.H }

// Extract returns the packed bytes of a region, row by row.
func (b *Bitmap) Extract(r Region) []byte {
	stride := r.W / 8
	out := make([]byte, 0, stride*r.H)
	for y := r.Y; y < r.Y+r.H; y++ {
		off := y*b.Stride() + r.X/8
		out = append(out, b.Pix[off:off+stride]...)
	}
	return out
}

// mergeGap is how many unchanged rows may separate two changed bands before
// they are sent as separate regions instead of one.
const mergeGap = 16

// Diff returns the byte-aligned regions where next differs from prev. Rows
// with changes are grouped into horizontal bands; each band becomes one
// region spanning the changed byte columns of all its rows.
func Diff(prev, next *Bitmap) []Region {
	if prev == nil || prev.W != next.W || prev.H != next.H {
		return []Region{next.Full()}
	}
	stride := next.Stride()
	var regions []Region
	var cur *Region
	minB, maxB := 0, 0
	lastChanged := -mergeGap - 1

	flush := func() {
		if cur != nil {
			cur.X = minB * 8
			cur.W = (maxB - minB + 1) * 8
			cur.H = lastChanged - cur.Y + 1
			regions = append(regions, *cur)
			cur = nil
		}
	}

	for y := 0; y < next.H; y++ {
		row := y * stride
		lo, hi := -1, -1
		for x := 0; x < stride; x++ {
			if prev.Pix[row+x] != next.Pix[row+x] {
				if lo < 0 {
					lo = x
				}
				hi = x
			}
		}
		if lo < 0 {
			continue
		}
		if cur != nil && y-lastChanged > mergeGap {
			flush()
		}
		if cur == nil {
			cur = &Region{Y: y}
			minB, maxB = lo, hi
		} else {
			minB, maxB = min(minB, lo), max(maxB, hi)
		}
		lastChanged = y
	}
	flush()
	return regions
}

// Frame kinds on the wire.
const (
	KindFull    = 0
	KindPartial = 1
)

// Wire header constants, see docs/device-protocol.md.
const (
	Magic      = 0x5048 // "HP" in little-endian byte order
	Version    = 1
	HeaderSize = 16
)

// Encode builds a binary frame: 16-byte little-endian header followed by the
// packed region data.
func Encode(kind uint8, seq uint32, r Region, data []byte) []byte {
	buf := make([]byte, HeaderSize+len(data))
	binary.LittleEndian.PutUint16(buf[0:], Magic)
	buf[2] = Version
	buf[3] = kind
	binary.LittleEndian.PutUint32(buf[4:], seq)
	binary.LittleEndian.PutUint16(buf[8:], uint16(r.X))
	binary.LittleEndian.PutUint16(buf[10:], uint16(r.Y))
	binary.LittleEndian.PutUint16(buf[12:], uint16(r.W))
	binary.LittleEndian.PutUint16(buf[14:], uint16(r.H))
	copy(buf[HeaderSize:], data)
	return buf
}

// Header is a decoded frame header.
type Header struct {
	Kind   uint8
	Seq    uint32
	Region Region
}

// Decode parses a frame and returns its header and payload. It is the
// reference implementation of what the firmware does.
func Decode(buf []byte) (Header, []byte, error) {
	if len(buf) < HeaderSize {
		return Header{}, nil, errors.New("frame too short")
	}
	if binary.LittleEndian.Uint16(buf[0:]) != Magic || buf[2] != Version {
		return Header{}, nil, errors.New("bad magic or version")
	}
	h := Header{
		Kind: buf[3],
		Seq:  binary.LittleEndian.Uint32(buf[4:]),
		Region: Region{
			X: int(binary.LittleEndian.Uint16(buf[8:])),
			Y: int(binary.LittleEndian.Uint16(buf[10:])),
			W: int(binary.LittleEndian.Uint16(buf[12:])),
			H: int(binary.LittleEndian.Uint16(buf[14:])),
		},
	}
	data := buf[HeaderSize:]
	if len(data) != h.Region.W/8*h.Region.H {
		return Header{}, nil, errors.New("payload size does not match region")
	}
	return h, data, nil
}

// Apply writes a region's packed data into the bitmap.
func (b *Bitmap) Apply(r Region, data []byte) {
	stride := r.W / 8
	for y := 0; y < r.H; y++ {
		off := (r.Y+y)*b.Stride() + r.X/8
		copy(b.Pix[off:off+stride], data[y*stride:(y+1)*stride])
	}
}
