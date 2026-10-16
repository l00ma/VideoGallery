package gallery

import (
	"embed"
	"fmt"
	htmltemplate "html/template"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	texttemplate "text/template"
	"time"
)

//go:embed templates/*.gohtml
var htmlFS embed.FS

//go:embed templates/style.css.tmpl
var cssSource string

//go:embed assets/favicon.ico
var faviconICO []byte

var pageTemplates = htmltemplate.Must(htmltemplate.ParseFS(htmlFS, "templates/*.gohtml"))
var cssTemplate = texttemplate.Must(texttemplate.New("style.css").Parse(cssSource))

func pageFile(n int) string {
	if n == 1 {
		return "index.html"
	}
	return fmt.Sprintf("page-%d.html", n)
}

func videoFile(n int) string { return fmt.Sprintf("video-%d.html", n) }

// urlPath encode chaque segment d'un chemin relatif ("a b/c#d.mp4").
func urlPath(p string) htmltemplate.URL {
	parts := strings.Split(filepath.ToSlash(p), "/")
	for i, s := range parts {
		if s != ".." && s != "." {
			parts[i] = url.PathEscape(s)
		}
	}
	return htmltemplate.URL(strings.Join(parts, "/"))
}

func plural(n int, one, many string) string {
	if n <= 1 {
		return one
	}
	return many
}

type cardView struct {
	Number         int
	Name           string
	Duration       string // « 12:34 », incrusté sur la miniature
	Details        string // « 245,3 Mo · 1920 × 1080 · 24/08/2024 » (date de tournage)
	Href           string
	Thumb          htmltemplate.URL
	ThumbW, ThumbH int
}

// cellView : lien posé sur une capture de la planche. Le petit script de la
// page vidéo lit alors la vidéo dans le lecteur de la page, à l'instant de la
// capture ; sans JavaScript, le lien ouvre la vidéo à cet instant dans un
// nouvel onglet (fragment de média « #t=secondes »). Sa position est en
// pourcentages de la planche, pour suivre le redimensionnement de l'image.
type cellView struct {
	Href  htmltemplate.URL
	At    string // instant en secondes (« 33.18 »), lu par le script de la page
	Style htmltemplate.CSS
	Clock string
}

type pageLink struct {
	Number  int
	Href    string
	Current bool
}

type indexView struct {
	Gallery    string
	TotalLabel string
	Cols       int
	Cards      []cardView
	Prev, Next string // vides quand inexistants
	Links      []pageLink
	Version    string
}

type videoView struct {
	Gallery        string
	Number, Total  int
	Name           string
	Source         htmltemplate.URL
	MIME           string
	Unsupported    string // codec que tous les navigateurs ne lisent pas (« HEVC (H.265) »), sinon vide
	Poster         htmltemplate.URL
	Ratio          float64 // largeur/hauteur du lecteur, réservé avant le chargement
	Sheet          htmltemplate.URL
	SheetW, SheetH int
	Cells          []cellView // zones cliquables posées sur les captures de la planche
	Prev, Next     string
	Back           string
	Version        string
}

// render écrit style.css, favicon.ico, les pages d'index et les pages vidéo.
// Renvoie le nombre de pages d'index.
func render(opts Options, items []item) (int, error) {
	perPage := opts.Cols * opts.Rows
	pages := (len(items) + perPage - 1) / perPage

	// Chemin relatif de sortie -> vidéos, pour les <source>.
	rel, err := filepath.Rel(opts.OutDir, opts.VideoDir)
	if err != nil {
		return 0, fmt.Errorf("chemin relatif vers les vidéos: %w", err)
	}

	if err := removeOldPages(opts.OutDir); err != nil {
		return 0, err
	}
	if err := writeCSS(opts); err != nil {
		return 0, err
	}
	if err := os.WriteFile(filepath.Join(opts.OutDir, "favicon.ico"), faviconICO, 0o644); err != nil {
		return 0, err
	}

	total := len(items)
	totalLabel := fmt.Sprintf("%d %s", total, plural(total, "vidéo", "vidéos"))

	for p := 1; p <= pages; p++ {
		v := indexView{Gallery: opts.Name, TotalLabel: totalLabel, Cols: opts.Cols, Version: Version}
		first := (p - 1) * perPage
		last := min(first+perPage, total)
		for i := first; i < last; i++ {
			it := items[i]
			v.Cards = append(v.Cards, cardView{
				Number: i + 1, Name: it.Name, Href: videoFile(i + 1),
				Duration: formatClock(it.Duration),
				Details: fmt.Sprintf("%s · %d × %d · %s", formatSize(it.Size), it.Width, it.Height,
					time.Unix(it.shotTime(), 0).Format("02/01/2006")),
				Thumb: urlPath(it.Thumb), ThumbW: it.ThumbW, ThumbH: it.ThumbH,
			})
		}
		if p > 1 {
			v.Prev = pageFile(p - 1)
		}
		if p < pages {
			v.Next = pageFile(p + 1)
		}
		if pages > 1 {
			for n := 1; n <= pages; n++ {
				v.Links = append(v.Links, pageLink{Number: n, Href: pageFile(n), Current: n == p})
			}
		}
		if err := writePage(opts.OutDir, pageFile(p), "index", v); err != nil {
			return 0, err
		}
	}

	for i, it := range items {
		n := i + 1
		var prev, next string // vides aux extrémités : pas de navigation circulaire
		if n > 1 {
			prev = videoFile(n - 1)
		}
		if n < total {
			next = videoFile(n + 1)
		}
		source := urlPath(filepath.ToSlash(filepath.Join(rel, it.Name)))
		v := videoView{
			Gallery: opts.Name, Number: n, Total: total, Name: it.Name,
			Source: source, MIME: it.MIME, Cells: sheetLinks(source, it),
			Unsupported: unsupported(it.Codec),
			Poster:      urlPath(it.Thumb), Ratio: float64(it.ThumbW) / float64(it.ThumbH), Sheet: urlPath(it.Sheet), SheetW: it.SheetW, SheetH: it.SheetH,
			Prev: prev, Next: next,
			Back: pageFile(i/perPage + 1), Version: Version,
		}
		if err := writePage(opts.OutDir, videoFile(n), "video", v); err != nil {
			return 0, err
		}
	}
	return pages, nil
}

func writePage(dir, file, tmpl string, data any) error {
	f, err := os.Create(filepath.Join(dir, file))
	if err != nil {
		return err
	}
	if err := pageTemplates.ExecuteTemplate(f, tmpl+".gohtml", data); err != nil {
		f.Close()
		return fmt.Errorf("rendu de %s: %w", file, err)
	}
	return f.Close()
}

func writeCSS(opts Options) error {
	f, err := os.Create(filepath.Join(opts.OutDir, "style.css"))
	if err != nil {
		return err
	}
	data := struct{ Background, Foreground string }{opts.Background, opts.Foreground}
	if err := cssTemplate.Execute(f, data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// removeOldPages supprime les pages d'une génération précédente (et elles
// seules : on ne touche pas aux autres fichiers du répertoire de sortie).
func removeOldPages(dir string) error {
	for _, pattern := range []string{"index.html", "page-*.html", "video-*.html"} {
		matches, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			return err
		}
		for _, m := range matches {
			if err := os.Remove(m); err != nil {
				return fmt.Errorf("suppression de %s: %w", m, err)
			}
		}
	}
	return nil
}

func unsupported(codec string) string {
	if webCodec(codec) {
		return ""
	}
	return codecLabel(codec)
}

func sheetLinks(source htmltemplate.URL, it item) []cellView {
	if it.Duration <= 0 || it.SheetW <= 0 || it.SheetH <= 0 {
		return nil
	}
	times := FrameTimes(it.Duration)
	pct := func(v, total int) float64 { return 100 * float64(v) / float64(total) }
	var cells []cellView
	for i, r := range sheetCells(it.SheetW, it.SheetH) {
		at := strconv.FormatFloat(times[i], 'f', 2, 64)
		cells = append(cells, cellView{
			Href: source + htmltemplate.URL("#t="+at),
			At:   at,
			Style: htmltemplate.CSS(fmt.Sprintf("left:%.3f%%;top:%.3f%%;width:%.3f%%;height:%.3f%%",
				pct(r.Min.X, it.SheetW), pct(r.Min.Y, it.SheetH), pct(r.Dx(), it.SheetW), pct(r.Dy(), it.SheetH))),
			Clock: formatClock(times[i]),
		})
	}
	return cells
}
