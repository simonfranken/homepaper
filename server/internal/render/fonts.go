package render

import (
	"fmt"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

// Fonts are the Go font family, embedded in the binary. Text is rasterized
// with full hinting and then hard-thresholded to 1 bit (docs/rendering.md);
// bitmap fonts can replace these later.
type fontID int

const (
	fontRegular fontID = iota
	fontMedium
	fontBold
)

var fontData = map[fontID][]byte{
	fontRegular: goregular.TTF,
	fontMedium:  gomedium.TTF,
	fontBold:    gobold.TTF,
}

var (
	parsed = map[fontID]*opentype.Font{}
	faces  = map[string]font.Face{}
)

func init() {
	for id, data := range fontData {
		f, err := opentype.Parse(data)
		if err != nil {
			panic(err)
		}
		parsed[id] = f
	}
}

// face returns a cached face at a pixel size. Faces are not safe for
// concurrent use; the renderer serializes all drawing.
func face(id fontID, px float64) font.Face {
	key := fmt.Sprintf("%d/%.0f", id, px)
	if f, ok := faces[key]; ok {
		return f
	}
	f, err := opentype.NewFace(parsed[id], &opentype.FaceOptions{Size: px, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err)
	}
	faces[key] = f
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
