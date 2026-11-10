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
	h, data, err := Decode(buf)
	if err != nil {
		t.Fatal(err)
	}
	if h.Kind != KindPartial || h.Seq != 42 || h.Region != r || len(data) != 6 {
		t.Fatalf("decoded %+v len %d", h, len(data))
	}
}
