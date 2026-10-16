package gallery

import (
	"image"
	"image/color"
	"image/draw"
	"testing"
)

// Les zones cliquables, recalculées d'après la seule taille de la planche,
// doivent tomber exactement sur les captures dessinées par ComposeSheet, pour
// plusieurs largeurs et ratios (paysage, portrait).
func TestSheetCellsMatchComposeSheet(t *testing.T) {
	for _, size := range [][2]int{{120, 68}, {320, 180}, {280, 158}, {180, 320}, {1920, 1080}} {
		w, h := size[0], size[1]
		frames := make([]image.Image, sheetFrames)
		for i := range frames {
			f := image.NewRGBA(image.Rect(0, 0, w, h))
			// une teinte par capture, pour reconnaître où chacune a été posée
			draw.Draw(f, f.Bounds(), image.NewUniform(color.RGBA{uint8(20 * (i + 1)), 0, 0, 255}), image.Point{}, draw.Src)
			frames[i] = f
		}
		sheet := ComposeSheet(frames, FrameTimes(90), SheetInfo{Name: "x.mp4", Duration: 90})
		b := sheet.Bounds()
		for i, r := range sheetCells(b.Dx(), b.Dy()) {
			if r.Dx() != w || r.Dy() != h {
				t.Fatalf("%dx%d case %d : %v, attendu %dx%d", w, h, i, r, w, h)
			}
			// coin haut droit : hors de l'horodatage, donc teinte d'origine
			if got := sheet.RGBAAt(r.Max.X-1, r.Min.Y).R; got != uint8(20*(i+1)) {
				t.Errorf("%dx%d case %d mal placée (%v) : teinte %d", w, h, i, r, got)
			}
			if r.Max.X > b.Dx() || r.Max.Y > b.Dy() {
				t.Errorf("%dx%d case %d hors de la planche : %v", w, h, i, r)
			}
		}
	}
}
