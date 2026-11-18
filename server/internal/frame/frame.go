// Package frame holds the panel-native bitmap formats (1bpp and 2bpp),
// region diffing and the binary wire encoding defined in
// docs/device-protocol.md.
package frame

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
)

// Bitmap is a packed image with 1 or 2 bits per pixel (Depth). Rows are
// byte-aligned and the most significant bits are the leftmost pixel.
//   - 1bpp: a set bit is white, a clear bit black.
//   - 2bpp: 0 black, 1 dark grey, 2 light grey, 3 white.
//
// An all-white bitmap is all 0xFF in both formats.
type Bitmap struct {
	W, H  int
	Depth int
	Pix   []byte
}

// Pixel formats as named in the device's hello message.
const (
	Format1bpp = "1bpp"
	Format2bpp = "2bpp"
)

// DepthOf returns the bits per pixel of a format name, or 0 if unknown.
func DepthOf(format string) int {
	switch format {
	case Format1bpp:
		return 1
	case Format2bpp:
		return 2
	}
	return 0
}

// New returns an all-white 1bpp bitmap. Width must be a multiple of 8.
func New(w, h int) *Bitmap { return NewDepth(w, h, 1) }

// NewDepth returns an all-white bitmap with 1 or 2 bits per pixel. Width
// must be a multiple of 8.
func NewDepth(w, h, depth int) *Bitmap {
	b := &Bitmap{W: w, H: h, Depth: depth, Pix: make([]byte, w*depth/8*h)}
	for i := range b.Pix {
		b.Pix[i] = 0xFF
	}
	return b
}

// Stride is the number of bytes per row.
func (b *Bitmap) Stride() int { return b.W * b.Depth / 8 }

// pxPerByte is how many pixels one byte holds.
func (b *Bitmap) pxPerByte() int { return 8 / b.Depth }

// RegionBytes is the payload size of a region at depth.
func RegionBytes(r Region, depth int) int { return r.W * depth / 8 * r.H }

// Level is the 2bpp value of pixel (x, y): 0 black … 3 white. On a 1bpp
// bitmap it returns 0 or 3.
func (b *Bitmap) Level(x, y int) uint8 {
	if b.Depth == 1 {
		if b.Pix[y*b.Stride()+x/8]&(0x80>>(x%8)) != 0 {
			return 3
		}
		return 0
	}
	shift := 6 - 2*(x%4)
	return b.Pix[y*b.Stride()+x/4] >> shift & 3
}

// setLevel sets a 2bpp pixel.
func (b *Bitmap) setLevel(x, y int, v uint8) {
	shift := 6 - 2*(x%4)
	i := y*b.Stride() + x/4
	b.Pix[i] = b.Pix[i]&^(3<<shift) | v<<shift
}

// FromGray thresholds a greyscale image into a bitmap: pixels darker than
// threshold become black.
func FromGray(img *image.Gray, threshold uint8) *Bitmap {
	r := img.Bounds()
	b := New(r.Dx(), r.Dy())
	for y := 0; y < b.H; y++ {
		row := img.Pix[y*img.Stride:]
		for x := 0; x < b.W; x++ {
			if row[x] < threshold {
				b.Pix[y*b.Stride()+x/8] &^= 0x80 >> (x % 8)
			}
		}
	}
	return b
}

// FromLevels quantizes a greyscale image to 2bpp: each pixel goes to the
// nearest of black, dark grey (0x55), light grey (0xAA) and white.
func FromLevels(img *image.Gray) *Bitmap {
	r := img.Bounds()
	b := NewDepth(r.Dx(), r.Dy(), 2)
	for y := 0; y < b.H; y++ {
		row := img.Pix[y*img.Stride:]
		for x := 0; x < b.W; x++ {
			if v := (uint16(row[x]) + 42) / 85; v != 3 {
				b.setLevel(x, y, uint8(v))
			}
		}
	}
	return b
}

// ToGray expands the bitmap to greyscale (0, 0x55, 0xAA, 0xFF), e.g. for
// PNG previews.
func (b *Bitmap) ToGray() *image.Gray {
	img := image.NewGray(image.Rect(0, 0, b.W, b.H))
	for y := 0; y < b.H; y++ {
		for x := 0; x < b.W; x++ {
			img.Pix[y*img.Stride+x] = b.Level(x, y) * 0x55
		}
	}
	return img
}

// HasGrey reports whether any pixel is grey.
func (b *Bitmap) HasGrey() bool {
	if b.Depth != 2 {
		return false
	}
	for _, v := range b.Pix {
		// A byte holds no grey when each pixel pair is 00 or 11.
		if (v^v>>1)&0x55 != 0 {
			return true
		}
	}
	return false
}

// ChangedGrey reports whether a pixel in r that differs between prev and
// next is grey in either. Unchanged grey pixels don't count: a B/W partial
// refresh leaves them alone.
func ChangedGrey(prev, next *Bitmap, r Region) bool {
	if next.Depth != 2 || prev == nil || prev.Depth != 2 {
		return false
	}
	for y := r.Y; y < r.Y+r.H; y++ {
		for x := r.X; x < r.X+r.W; x++ {
			a, b := prev.Level(x, y), next.Level(x, y)
			if a != b && (a == 1 || a == 2 || b == 1 || b == 2) {
				return true
			}
		}
	}
	return false
}

// CRC32 is the IEEE checksum of the whole bitmap, compared against the
// checksum a device reports in its ack.
func (b *Bitmap) CRC32() uint32 { return crc32.ChecksumIEEE(b.Pix) }

// Clone returns a deep copy.
func (b *Bitmap) Clone() *Bitmap {
	c := &Bitmap{W: b.W, H: b.H, Depth: b.Depth, Pix: make([]byte, len(b.Pix))}
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
	ppb := b.pxPerByte()
	stride := r.W / ppb
	out := make([]byte, 0, stride*r.H)
	for y := r.Y; y < r.Y+r.H; y++ {
		off := y*b.Stride() + r.X/ppb
		out = append(out, b.Pix[off:off+stride]...)
	}
	return out
}

// mergeGap is how many unchanged rows may separate two changed bands before
// they are sent as separate regions instead of one.
const mergeGap = 16

// Diff returns the 8-pixel-aligned regions where next differs from prev.
// Rows with changes are grouped into horizontal bands; each band becomes one
// region spanning the changed columns of all its rows.
func Diff(prev, next *Bitmap) []Region {
	if prev == nil || prev.W != next.W || prev.H != next.H || prev.Depth != next.Depth {
		return []Region{next.Full()}
	}
	stride := next.Stride()
	// Bytes per 8 pixels: 1 at 1bpp, 2 at 2bpp. Regions snap to 8 px.
	group := next.Depth
	var regions []Region
	var cur *Region
	minB, maxB := 0, 0
	lastChanged := -mergeGap - 1

	flush := func() {
		if cur != nil {
			lo, hi := minB/group, maxB/group
			cur.X = lo * 8
			cur.W = (hi - lo + 1) * 8
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

// Decode parses a frame for a panel with depth bits per pixel and returns
// its header and payload. It is the reference implementation of what the
// firmware does.
func Decode(buf []byte, depth int) (Header, []byte, error) {
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
	if len(data) != RegionBytes(h.Region, depth) {
		return Header{}, nil, errors.New("payload size does not match region")
	}
	return h, data, nil
}

// Apply writes a region's packed data into the bitmap.
func (b *Bitmap) Apply(r Region, data []byte) {
	ppb := b.pxPerByte()
	stride := r.W / ppb
	for y := 0; y < r.H; y++ {
		off := (r.Y+y)*b.Stride() + r.X/ppb
		copy(b.Pix[off:off+stride], data[y*stride:(y+1)*stride])
	}
}
