package gallery

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Les polices Go (BSD, dessinées pour être lisibles à l'écran) sont
// embarquées : le rendu du texte ne dépend d'aucune police du système.
var fonts struct {
	once          sync.Once
	regular, bold *opentype.Font
}

func loadFonts() {
	fonts.once.Do(func() {
		var err error
		if fonts.regular, err = opentype.Parse(goregular.TTF); err != nil {
			panic(err) // police embarquée : ne peut échouer qu'à la compilation
		}
		if fonts.bold, err = opentype.Parse(gobold.TTF); err != nil {
			panic(err)
		}
	})
}

// newFace crée une police à la taille voulue (en pixels). Un font.Face n'est
// pas utilisable en parallèle : chaque planche crée les siens.
func newFace(bold bool, px float64) font.Face {
	loadFonts()
	f := fonts.regular
	if bold {
		f = fonts.bold
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: px, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err)
	}
	return face
}

func textWidth(face font.Face, s string) int {
	return font.MeasureString(face, s).Ceil()
}

// drawText écrit s à partir de (x, baseline).
func drawText(dst draw.Image, face font.Face, col color.Color, x, baseline int, s string) {
	d := font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  fixed.P(x, baseline),
	}
	d.DrawString(s)
}

func lineHeight(face font.Face) int { return face.Metrics().Height.Ceil() }

// ellipsize raccourcit s avec « … » pour tenir dans maxW pixels.
func ellipsize(face font.Face, s string, maxW int) string {
	if textWidth(face, s) <= maxW {
		return s
	}
	r := []rune(s)
	for len(r) > 1 {
		r = r[:len(r)-1]
		if cand := strings.TrimRight(string(r), " ") + "…"; textWidth(face, cand) <= maxW {
			return cand
		}
	}
	return "…"
}

// fitText renvoie le premier candidat qui tient dans maxW, sinon le dernier
// raccourci avec « … ».
func fitText(face font.Face, candidates []string, maxW int) string {
	for _, c := range candidates {
		if textWidth(face, c) <= maxW {
			return c
		}
	}
	return ellipsize(face, candidates[len(candidates)-1], maxW)
}

func clampF(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

// formatClock : « 3:25 » ou « 1:02:03 ».
func formatClock(seconds float64) string {
	s := int(math.Round(seconds))
	if s < 0 {
		s = 0
	}
	if h := s / 3600; h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, s%3600/60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// formatSize : taille en Mo (1 Mo = 10^6 octets, comme l'explorateur de
// fichiers d'Ubuntu), virgule décimale française ; en Go au-delà de 1000 Mo.
func formatSize(bytes int64) string {
	var s string
	switch {
	case bytes >= 1_000_000_000:
		s = fmt.Sprintf("%.2f Go", float64(bytes)/1e9)
	case bytes >= 100_000:
		s = fmt.Sprintf("%.1f Mo", float64(bytes)/1e6)
	default:
		s = fmt.Sprintf("%.0f Ko", float64(bytes)/1e3)
	}
	return strings.Replace(s, ".", ",", 1)
}
