package frame

import (
	"image"
	"testing"
)

func TestFromGrayPolarityAndPacking(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 16, 2))
	for i := range img.Pix {
		img.Pix[i] = 0xFF
	}
	img.Pix[0] = 0        // (0,0) black -> MSB of byte 0 cleared
	img.Pix[16+9] = 0x10  // (9,1) dark  -> bit 6 of byte 3 cleared
	img.Pix[16+10] = 0xC0 // (10,1) light -> stays white
	b := FromGray(img, 128)
	want := []byte{0x7F, 0xFF, 0xFF, 0xBF}
	for i := range want {
		if b.Pix[i] != want[i] {
			t.Fatalf("byte %d = %#x, want %#x", i, b.Pix[i], want[i])
		}
	}
}

func TestDiffNoChange(t *testing.T) {
	a := New(800, 480)
	if r := Diff(a, a.Clone()); len(r) != 0 {
		t.Fatalf("expected no regions, got %v", r)
	}
}

func TestDiffRegionsAreAlignedAndCover(t *testing.T) {
	prev := New(800, 480)
	next := prev.Clone()
	set := func(x, y int) { next.Pix[y*next.Stride()+x/8] &^= 0x80 >> (x % 8) }
	set(13, 10)
	set(100, 12)
	set(700, 400)

	regions := Diff(prev, next)
	if len(regions) != 2 {
		t.Fatalf("expected 2 bands, got %v", regions)
	}
	if regions[0] != (Region{X: 8, Y: 10, W: 96, H: 3}) {
		t.Fatalf("first region %v", regions[0])
	}
	if regions[1] != (Region{X: 696, Y: 400, W: 8, H: 1}) {
		t.Fatalf("second region %v", regions[1])
	}

	// Applying the regions to prev must reproduce next.
	got := prev.Clone()
	for _, r := range regions {
		got.Apply(r, next.Extract(r))
	}
	if got.CRC32() != next.CRC32() {
		t.Fatal("applied regions do not reproduce next frame")
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	b := New(800, 480)
	r := Region{X: 16, Y: 5, W: 24, H: 2}
	buf := Encode(KindPartial, 42, r, b.Extract(r))
	if len(buf) != HeaderSize+6 {
		t.Fatalf("len %d", len(buf))
	}
	if buf[0] != 'H' || buf[1] != 'P' {
		t.Fatalf("magic bytes %q", buf[:2])
	}
	h, data, err := Decode(buf, 1)
	if err != nil {
		t.Fatal(err)
	}
	if h.Kind != KindPartial || h.Seq != 42 || h.Region != r || len(data) != 6 {
		t.Fatalf("decoded %+v len %d", h, len(data))
	}
}

func TestFromLevelsPacking(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 8, 1))
	copy(img.Pix, []byte{0, 0x55, 0xAA, 0xFF, 30, 100, 180, 250})
	b := FromLevels(img)
	// 00 01 10 11 | 00 01 10 11
	want := []byte{0x1B, 0x1B}
	if b.Depth != 2 || len(b.Pix) != 2 || b.Pix[0] != want[0] || b.Pix[1] != want[1] {
		t.Fatalf("got depth %d pix %#v, want %#v", b.Depth, b.Pix, want)
	}
	if g := b.ToGray(); g.Pix[1] != 0x55 || g.Pix[2] != 0xAA || g.Pix[3] != 0xFF {
		t.Fatalf("ToGray %v", g.Pix)
	}
	if !b.HasGrey() || NewDepth(8, 1, 2).HasGrey() {
		t.Fatal("HasGrey")
	}
}

func TestDiff2bppAlignsTo8Pixels(t *testing.T) {
	prev := NewDepth(800, 480, 2)
	next := prev.Clone()
	next.setLevel(13, 10, 0) // byte column 3 (odd): pixels 12..15
	next.setLevel(17, 11, 1) // byte column 4: pixels 16..19
	regions := Diff(prev, next)
	if len(regions) != 1 || regions[0] != (Region{X: 8, Y: 10, W: 16, H: 2}) {
		t.Fatalf("regions %v", regions)
	}
	got := prev.Clone()
	for _, r := range regions {
		data := next.Extract(r)
		if len(data) != RegionBytes(r, 2) {
			t.Fatalf("extract len %d", len(data))
		}
		got.Apply(r, data)
	}
	if got.CRC32() != next.CRC32() {
		t.Fatal("applied regions do not reproduce next frame")
	}
	if !ChangedGrey(prev, next, regions[0]) {
		t.Fatal("a pixel turned dark grey: ChangedGrey must be true")
	}
}

func TestChangedGreyIgnoresUnchangedGrey(t *testing.T) {
	prev := NewDepth(64, 8, 2)
	for x := 0; x < 64; x++ {
		prev.setLevel(x, 4, 2) // a light grey rule across the band
	}
	next := prev.Clone()
	next.setLevel(10, 3, 0) // black text changes above it
	next.setLevel(10, 5, 0)
	r := Diff(prev, next)
	if len(r) != 1 || ChangedGrey(prev, next, r[0]) {
		t.Fatalf("B/W change inside a band with unchanged grey must stay partial: %v", r)
	}
	next.setLevel(20, 4, 0) // the rule itself changes
	if !ChangedGrey(prev, next, Diff(prev, next)[0]) {
		t.Fatal("grey to black must count")
	}
}

func TestDecodeChecksDepth(t *testing.T) {
	b := NewDepth(800, 480, 2)
	r := Region{X: 8, Y: 0, W: 16, H: 3}
	buf := Encode(KindPartial, 1, r, b.Extract(r))
	if _, data, err := Decode(buf, 2); err != nil || len(data) != 12 {
		t.Fatalf("2bpp decode: %v len %d", err, len(data))
	}
	if _, _, err := Decode(buf, 1); err == nil {
		t.Fatal("a 2bpp payload must not pass as 1bpp")
	}
}
