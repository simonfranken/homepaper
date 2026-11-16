package render

import (
	"embed"
	"fmt"
	"sort"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

// Text comes in two kinds (docs/rendering.md, P-013):
//
//   - Up to 34 px it is set in the Lucida Sans bitmap fonts, whose pixels
//     were placed by hand for screens. A thresholded outline can't match
//     that at these sizes: stems come out one pixel here and two there.
//   - Above that, the Go fonts (also by Bigelow & Holmes) are rasterized
//     and hard-thresholded; at that size the threshold loses nothing.
//
// The bitmap fonts are the X.Org bh-75dpi / bh-100dpi BDF files, embedded
// unmodified, as their license requires. Its notice, which must appear in
// code comments and user documentation:
//
//	(c) Copyright 1989 Sun Microsystems, Inc. Sun design patents
//	pending in the U.S. and foreign countries. OPEN LOOK is a
//	trademark of AT&T. Used by written permission of the owners.
//
//	(c) Copyright Bigelow & Holmes 1986, 1985. Lucida is a registered
//	trademark of Bigelow & Holmes. Permission to use the Lucida
//	trademark is hereby granted only in association with the images
//	and fonts described in this file.
//
// The full text is in fonts/lucida/COPYING.
type fontID int

const (
	fontRegular fontID = iota
	fontMedium
	fontBold
)

var vectorData = map[fontID][]byte{
	fontRegular: goregular.TTF,
	fontMedium:  gomedium.TTF,
	fontBold:    gobold.TTF,
}

//go:embed fonts/lucida
var lucidaFS embed.FS

// Bitmap files per weight. Pixel sizes: 12, 14, 17, 20, 25, 34.
var bitmapFiles = map[string][]string{
	"regular": {"75dpi/luRS12", "75dpi/luRS14", "100dpi/luRS12", "100dpi/luRS14", "100dpi/luRS18", "100dpi/luRS24"},
	"bold":    {"75dpi/luBS12", "75dpi/luBS14", "100dpi/luBS12", "100dpi/luBS14", "100dpi/luBS18", "100dpi/luBS24"},
}

// Medium text (labels) uses the regular bitmaps: Lucida Sans "medium" is
// the regular weight, and its two-pixel stems are already sturdy.
var bitmapWeight = map[fontID]string{fontRegular: "regular", fontMedium: "regular", fontBold: "bold"}

var (
	vectors = map[fontID]*opentype.Font{}
	bitmaps = map[string]map[int]*bdfFont{} // weight → pixel size → font
	// bitmapSizes lists the pixel sizes with a bitmap, ascending.
	bitmapSizes []int
	faces       = map[string]font.Face{}
)

// maxBitmapPx is the largest bitmap size; bigger text is vector.
const maxBitmapPx = 34

func init() {
	for id, data := range vectorData {
		f, err := opentype.Parse(data)
		if err != nil {
			panic(err)
		}
		vectors[id] = f
	}
	for weight, files := range bitmapFiles {
		bitmaps[weight] = map[int]*bdfFont{}
		for _, name := range files {
			data, err := lucidaFS.ReadFile("fonts/lucida/" + name + ".bdf")
			if err != nil {
				panic(err)
			}
			f, err := parseBDF(data)
			if err != nil {
				panic(fmt.Sprintf("%s: %v", name, err))
			}
			bitmaps[weight][f.px] = f
		}
	}
	for px := range bitmaps["regular"] {
		bitmapSizes = append(bitmapSizes, px)
	}
	sort.Ints(bitmapSizes)
}

// sizeSteps are the pixel sizes text may be set at, ascending: the bitmap
// sizes up to 25 px, where hand-placed pixels matter most, then every whole
// pixel size (vector, except 34, which has a bitmap).
func sizeSteps(maxPx float64) []int {
	var out []int
	for _, px := range bitmapSizes {
		if px <= 25 && float64(px) <= maxPx {
			out = append(out, px)
		}
	}
	for px := 26; float64(px) <= maxPx; px++ {
		out = append(out, px)
	}
	if len(out) == 0 {
		out = append(out, bitmapSizes[0])
	}
	return out
}

// face returns a cached face at a pixel size: the bitmap font when there is
// one of exactly that size, the vector font otherwise. Faces are not safe
// for concurrent use; the renderer serializes all drawing.
func face(id fontID, px int) font.Face {
	key := fmt.Sprintf("%d/%d", id, px)
	if f, ok := faces[key]; ok {
		return f
	}
	var f font.Face
	vec := vectorFace(id, px)
	if b := bitmaps[bitmapWeight[id]][px]; b != nil {
		f = &bitmapFace{f: b, fallback: vec}
	} else {
		f = vec
	}
	faces[key] = f
	return f
}

func vectorFace(id fontID, px int) font.Face {
	f, err := opentype.NewFace(vectors[id], &opentype.FaceOptions{Size: float64(px), DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err)
	}
	return f
}

// FontFile returns an embedded font as TTF, so the web UI can set its text
// in the same typeface the panel uses without shipping a second copy.
func FontFile(name string) ([]byte, bool) {
	switch name {
	case "go-regular.ttf":
		return goregular.TTF, true
	case "go-medium.ttf":
		return gomedium.TTF, true
	case "go-bold.ttf":
		return gobold.TTF, true
	case "go-mono.ttf":
		return gomono.TTF, true
	}
	return nil, false
}
