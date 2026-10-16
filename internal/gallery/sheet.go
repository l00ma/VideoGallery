package gallery

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"
)

const (
	sheetGrid   = 3 // planche de 3x3 captures
	sheetFrames = sheetGrid * sheetGrid
	sheetGap    = 2
	jpegQuality = 85
	// La capture utilisée comme miniature : celle du milieu de la planche.
	thumbFrame = sheetFrames / 2
)

// FrameTimes répartit les captures sur toute la durée, entre 10 % et 90 %
// (on évite ainsi les noirs de début et de fin).
func FrameTimes(duration float64) [sheetFrames]float64 {
	var t [sheetFrames]float64
	for i := range t {
		t[i] = duration * float64(i+1) / float64(sheetFrames+1)
	}
	return t
}

// SheetInfo décrit la vidéo pour l'en-tête de la planche.
type SheetInfo struct {
	Name          string
	Size          int64 // octets
	Width, Height int   // résolution d'origine
	Duration      float64
}

var (
	headerBG   = color.RGBA{0x1c, 0x1c, 0x1e, 0xff}
	headerName = color.RGBA{0xf2, 0xf2, 0xf2, 0xff}
	headerInfo = color.RGBA{0xc4, 0xc4, 0xc8, 0xff}
	stampBG    = color.NRGBA{0, 0, 0, 165} // fond translucide des horodatages
	stampFG    = color.RGBA{0xff, 0xff, 0xff, 0xff}
)

// sheetHeader calcule la mise en page de l'en-tête d'une planche de largeur
// gridW : taille de police, marge, hauteur de ligne et hauteur totale.
func sheetHeader(gridW int) (headPx float64, pad, lineH, headerH int) {
	headPx = clampF(float64(gridW)/58, 11, 40)
	face := newFace(true, headPx)
	defer face.Close()
	pad = int(math.Round(headPx * 0.6))
	lineH = lineHeight(face)
	return headPx, pad, lineH, 2*pad + 2*lineH
}

// sheetCells retrouve, d'après la seule taille d'une planche (y compris en
// cache), le rectangle de chacune de ses captures.
func sheetCells(sheetW, sheetH int) [sheetFrames]image.Rectangle {
	_, _, _, headerH := sheetHeader(sheetW)
	w := (sheetW - (sheetGrid-1)*sheetGap) / sheetGrid
	h := (sheetH - headerH - (sheetGrid-1)*sheetGap) / sheetGrid
	var cells [sheetFrames]image.Rectangle
	for i := range cells {
		x := (i % sheetGrid) * (w + sheetGap)
		y := headerH + (i/sheetGrid)*(h+sheetGap)
		cells[i] = image.Rect(x, y, x+w, y+h)
	}
	return cells
}

// ComposeSheet assemble les captures en grille 3x3 sous un en-tête (nom,
// taille, résolution, durée), avec l'instant de chaque capture inscrit en bas
// à gauche. Les captures elles-mêmes ne sont pas modifiées : la miniature de
// l'index en est une, et doit rester propre.
func ComposeSheet(frames []image.Image, times [sheetFrames]float64, info SheetInfo) *image.RGBA {
	b := frames[0].Bounds()
	w, h := b.Dx(), b.Dy()
	gridW := sheetGrid*w + (sheetGrid-1)*sheetGap
	gridH := sheetGrid*h + (sheetGrid-1)*sheetGap

	// En-tête : deux lignes, dont la taille suit la largeur de la planche.
	headPx, pad, lineH, headerH := sheetHeader(gridW)
	nameFace, infoFace := newFace(true, headPx), newFace(false, headPx)
	defer nameFace.Close()
	defer infoFace.Close()

	sheet := image.NewRGBA(image.Rect(0, 0, gridW, headerH+gridH))
	draw.Draw(sheet, sheet.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)
	draw.Draw(sheet, image.Rect(0, 0, gridW, headerH), image.NewUniform(headerBG), image.Point{}, draw.Src)

	avail := gridW - 2*pad
	size, res, dur := formatSize(info.Size), fmt.Sprintf("%d × %d", info.Width, info.Height), formatClock(info.Duration)
	line2 := fitText(infoFace, []string{
		fmt.Sprintf("Taille : %s     Résolution : %s     Durée : %s", size, res, dur),
		fmt.Sprintf("%s  ·  %s  ·  %s", size, res, dur),
	}, avail)
	drawText(sheet, nameFace, headerName, pad, pad+nameFace.Metrics().Ascent.Ceil(), ellipsize(nameFace, info.Name, avail))
	drawText(sheet, infoFace, headerInfo, pad, pad+lineH+infoFace.Metrics().Ascent.Ceil(), line2)

	// Grille de captures, chacune avec son horodatage.
	stampPx := clampF(float64(w)/22, 9, 28)
	stampFace := newFace(true, stampPx)
	defer stampFace.Close()
	margin := int(math.Round(stampPx * 0.35))
	padX, padY := int(math.Round(stampPx*0.35)), int(math.Round(stampPx*0.15))
	for i, f := range frames {
		x := (i % sheetGrid) * (w + sheetGap)
		y := headerH + (i/sheetGrid)*(h+sheetGap)
		draw.Draw(sheet, image.Rect(x, y, x+w, y+h), f, f.Bounds().Min, draw.Src)

		label := formatClock(times[i])
		boxW := textWidth(stampFace, label) + 2*padX
		boxH := lineHeight(stampFace) + 2*padY
		box := image.Rect(x+margin, y+h-margin-boxH, x+margin+boxW, y+h-margin)
		draw.Draw(sheet, box, image.NewUniform(stampBG), image.Point{}, draw.Over)
		drawText(sheet, stampFace, stampFG, box.Min.X+padX, box.Min.Y+padY+stampFace.Metrics().Ascent.Ceil(), label)
	}
	return sheet
}

// writeJPEG écrit l'image via un fichier temporaire, pour ne jamais laisser
// un JPEG tronqué (et donc pris pour du cache valide) en cas d'interruption.
func writeJPEG(path string, img image.Image) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*.jpg")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := jpeg.Encode(tmp, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// CreateTemp crée en 0600 : on aligne sur les autres fichiers générés.
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func imageSize(path string) (w, h int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	return cfg.Width, cfg.Height, err
}
