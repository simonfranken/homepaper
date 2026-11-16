package render

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"image"
	"strconv"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// bdfFont is a bitmap font read from a BDF file. Its glyphs are drawn
// pixel for pixel, never scaled, so small text keeps the shapes its
// designer placed by hand instead of whatever a threshold leaves of an
// anti-aliased outline.
type bdfFont struct {
	px      int // PIXEL_SIZE
	ascent  int
	descent int
	capH    int
	xH      int
	glyphs  map[rune]*bdfGlyph
}

type bdfGlyph struct {
	advance int
	// Bounds relative to the dot (baseline origin), y down.
	bounds image.Rectangle
	mask   *image.Alpha
}

func parseBDF(data []byte) (*bdfFont, error) {
	f := &bdfFont{glyphs: map[rune]*bdfGlyph{}}
	sc := bufio.NewScanner(bytes.NewReader(data))
	var (
		g        *bdfGlyph
		enc      = -1
		w, h     int
		xo, yo   int
		row      int
		inBitmap bool
	)
	atoi := func(s string) int { n, _ := strconv.Atoi(s); return n }
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 {
			continue
		}
		if inBitmap && fields[0] != "ENDCHAR" {
			bits, err := hex.DecodeString(fields[0])
			if err != nil {
				return nil, fmt.Errorf("glyph %d: %w", enc, err)
			}
			for x := 0; x < w && x/8 < len(bits); x++ {
				if bits[x/8]&(0x80>>(x%8)) != 0 {
					g.mask.Pix[row*g.mask.Stride+x] = 0xff
				}
			}
			row++
			continue
		}
		switch fields[0] {
		case "PIXEL_SIZE":
			f.px = atoi(fields[1])
		case "FONT_ASCENT":
			f.ascent = atoi(fields[1])
		case "FONT_DESCENT":
			f.descent = atoi(fields[1])
		case "CAP_HEIGHT":
			f.capH = atoi(fields[1])
		case "X_HEIGHT":
			f.xH = atoi(fields[1])
		case "STARTCHAR":
			g, enc = &bdfGlyph{}, -1
		case "ENCODING":
			enc = atoi(fields[1])
		case "DWIDTH":
			g.advance = atoi(fields[1])
		case "BBX":
			w, h, xo, yo = atoi(fields[1]), atoi(fields[2]), atoi(fields[3]), atoi(fields[4])
			g.bounds = image.Rect(xo, -yo-h, xo+w, -yo)
			g.mask = image.NewAlpha(image.Rect(0, 0, w, h))
		case "BITMAP":
			inBitmap, row = true, 0
		case "ENDCHAR":
			inBitmap = false
			if enc >= 0 && g != nil {
				f.glyphs[rune(enc)] = g
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if f.px == 0 || len(f.glyphs) == 0 {
		return nil, fmt.Errorf("not a usable BDF font")
	}
	if f.capH == 0 {
		if g := f.glyphs['H']; g != nil {
			f.capH = -g.bounds.Min.Y
		}
	}
	return f, nil
}

// bitmapFace draws a bdfFont and borrows any glyph it lacks (e.g. "→")
// from a vector face of the same pixel size, on the same baseline. User
// entity names can contain anything, so a missing glyph must never drop
// the whole string out of the bitmap font.
type bitmapFace struct {
	f        *bdfFont
	fallback font.Face
}

var _ font.Face = (*bitmapFace)(nil)

func (b *bitmapFace) Close() error { return nil }

func (b *bitmapFace) Glyph(dot fixed.Point26_6, r rune) (image.Rectangle, image.Image, image.Point, fixed.Int26_6, bool) {
	// Bitmaps sit on whole pixels; the renderer keeps the dot whole too.
	x, y := dot.X.Round(), dot.Y.Round()
	g := b.f.glyphs[r]
	if g == nil {
		dr, mask, mp, adv, ok := b.fallback.Glyph(fixed.P(x, y), r)
		return dr, mask, mp, fixed.I(adv.Round()), ok
	}
	return g.bounds.Add(image.Pt(x, y)), g.mask, image.Point{}, fixed.I(g.advance), true
}

func (b *bitmapFace) GlyphBounds(r rune) (fixed.Rectangle26_6, fixed.Int26_6, bool) {
	g := b.f.glyphs[r]
	if g == nil {
		return b.fallback.GlyphBounds(r)
	}
	return fixed.R(g.bounds.Min.X, g.bounds.Min.Y, g.bounds.Max.X, g.bounds.Max.Y), fixed.I(g.advance), true
}

func (b *bitmapFace) GlyphAdvance(r rune) (fixed.Int26_6, bool) {
	g := b.f.glyphs[r]
	if g == nil {
		adv, ok := b.fallback.GlyphAdvance(r)
		// Whole pixels, so the bitmap glyphs after it stay on the grid.
		return fixed.I(adv.Round()), ok
	}
	return fixed.I(g.advance), true
}

func (b *bitmapFace) Kern(r0, r1 rune) fixed.Int26_6 { return 0 }

func (b *bitmapFace) Metrics() font.Metrics {
	return font.Metrics{
		Height:    fixed.I(b.f.ascent + b.f.descent),
		Ascent:    fixed.I(b.f.ascent),
		Descent:   fixed.I(b.f.descent),
		XHeight:   fixed.I(b.f.xH),
		CapHeight: fixed.I(b.f.capH),
	}
}

// covers reports whether the bitmap font has every rune of s.
func (f *bdfFont) covers(s string) []rune {
	var missing []rune
	for _, r := range s {
		if f.glyphs[r] == nil {
			missing = append(missing, r)
		}
	}
	return missing
}
