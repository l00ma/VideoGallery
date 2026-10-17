// Package gallery génère une galerie HTML statique à partir d'un répertoire
// de vidéos : une planche de 9 captures par vidéo, des pages d'index
// paginées et une page par vidéo. Aucun serveur n'est nécessaire pour la
// consulter (ouverture en file://).
package gallery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	xdraw "golang.org/x/image/draw"
)

const Version = "1.2.0"

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// Pages que nous générons : page-12.html, video-3.html (numérotées, donc
// « page-contact.html » n'en fait pas partie).
var ourPage = regexp.MustCompile(`^(page|video)-[0-9]+\.html$`)

// Options configure la génération.
type Options struct {
	VideoDir   string // répertoire des vidéos (requis)
	OutDir     string // répertoire de sortie ; défaut : <VideoDir>/pages
	Name       string // nom de la galerie ; défaut : nom du répertoire
	Cols, Rows int    // miniatures par ligne / par colonne
	Order      string // ordre des vidéos (OrderName... ; défaut : OrderName)
	Force      bool   // régénérer les images même si elles sont à jour
	Background string // couleur de fond, #rrggbb
	Foreground string // couleur du texte, #rrggbb
	FrameWidth int    // largeur d'une capture, en pixels
	Jobs       int    // vidéos traitées en parallèle
	Tools      Tools
	Log        func(format string, args ...any)
	// Progress est appelé après chaque vidéo traitée (réussie ou non).
	// Les appels sont sérialisés, comme ceux de Log.
	Progress func(done, total int)
}

// Failure signale une vidéo qui n'a pas pu être traitée.
type Failure struct {
	Video string
	Err   error
}

// Result résume une génération.
type Result struct {
	OutDir    string
	Videos    int // vidéos présentes dans la galerie
	Pages     int // pages d'index
	Generated int // vidéos dont les images ont été (re)générées
	Cached    int // vidéos dont les images étaient à jour
	Failed    []Failure
}

// item est une vidéo prête à être affichée.
type item struct {
	Video
	Meta                  // durée et résolution (ffprobe ou cache meta.json)
	Sheet, Thumb   string // noms de fichiers relatifs à OutDir, ex. img/320-a.mp4-sheet.jpg
	SheetW, SheetH int
	ThumbW, ThumbH int
	cached         bool
}

func (o *Options) applyDefaults() error {
	if o.VideoDir == "" {
		return errors.New("répertoire des vidéos non précisé")
	}
	abs, err := filepath.Abs(o.VideoDir)
	if err != nil {
		return err
	}
	o.VideoDir = abs
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		return fmt.Errorf("%s n'est pas un répertoire", o.VideoDir)
	}
	if o.OutDir == "" {
		o.OutDir = filepath.Join(o.VideoDir, "pages")
	}
	if o.OutDir, err = filepath.Abs(o.OutDir); err != nil {
		return err
	}
	if o.Name == "" {
		o.Name = filepath.Base(o.VideoDir)
	}
	if o.Cols <= 0 || o.Rows <= 0 {
		return errors.New("le nombre de miniatures par ligne et par colonne doit être positif")
	}
	if o.Order == "" {
		o.Order = OrderName
	}
	if err := validOrder(o.Order); err != nil {
		return err
	}
	if o.FrameWidth < 80 || o.FrameWidth > 1920 {
		return errors.New("la largeur des captures doit être comprise entre 80 et 1920 pixels")
	}
	if o.Background == "" {
		o.Background = "#baaf91"
	}
	if o.Foreground == "" {
		o.Foreground = "#333333"
	}
	if !hexColor.MatchString(o.Background) || !hexColor.MatchString(o.Foreground) {
		return errors.New("les couleurs doivent être au format #rrggbb")
	}
	if o.Jobs <= 0 {
		o.Jobs = runtime.NumCPU()
	}
	if o.Log == nil {
		o.Log = func(string, ...any) {}
	}
	if o.Progress == nil {
		o.Progress = func(int, int) {}
	}
	return nil
}

// Run génère la galerie.
func Run(ctx context.Context, opts Options) (*Result, error) {
	autoJobs := opts.Jobs <= 0
	if err := opts.applyDefaults(); err != nil {
		return nil, err
	}
	// Évite que deux goroutines n'écrivent en même temps dans le journal.
	var logMu sync.Mutex
	rawLog := opts.Log
	opts.Log = func(format string, args ...any) {
		logMu.Lock()
		defer logMu.Unlock()
		rawLog(format, args...)
	}
	rawProgress := opts.Progress
	opts.Progress = func(done, total int) {
		logMu.Lock()
		defer logMu.Unlock()
		rawProgress(done, total)
	}

	videos, err := Scan(opts.VideoDir, opts.Order != OrderNone)
	if err != nil {
		return nil, err
	}
	if len(videos) == 0 {
		return nil, fmt.Errorf("aucune vidéo (%s) dans %s", Formats, opts.VideoDir)
	}
	opts.Log("%d vidéo(s) trouvée(s) dans %s", len(videos), opts.VideoDir)
	if fs, ok := networkFS(opts.VideoDir); ok && autoJobs && opts.Jobs > netJobs {
		opts.Jobs = netJobs
		opts.Log("Vidéos sur un partage réseau (%s) : %d vidéos traitées à la fois pour ménager le réseau et le serveur (option --jobs du CLI pour changer)", fs, netJobs)
	}

	if err := checkOutDir(opts.OutDir); err != nil {
		return nil, err
	}
	imgDir := filepath.Join(opts.OutDir, "img")
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		return nil, fmt.Errorf("création de %s: %w", imgDir, err)
	}
	if err := os.WriteFile(filepath.Join(opts.OutDir, markerFile), []byte(markerContent), 0o644); err != nil {
		return nil, err
	}

	metas := loadMetaCache(imgDir)
	items, failures := buildImages(ctx, opts, videos, imgDir, metas)
	if err := ctx.Err(); err != nil {
		metas.save(imgDir, videoNames(videos)) // la prochaine génération en profitera
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("aucune vidéo exploitable (%d en échec)", len(failures))
	}

	res := &Result{OutDir: opts.OutDir, Videos: len(items), Failed: failures}
	for _, it := range items {
		if it.cached {
			res.Cached++
		} else {
			res.Generated++
		}
	}

	sortItems(items, opts.Order)
	for _, it := range items {
		if !webCodec(it.Codec) {
			opts.Log("Attention : %s est en %s, un format dont la lecture dans le navigateur dépend du système (sa page propose d'ouvrir le fichier avec le lecteur vidéo)",
				it.Name, codecLabel(it.Codec))
		}
	}
	pruneImages(imgDir, items)
	names := make([]string, len(items))
	for i, it := range items {
		names[i] = it.Name
	}
	if err := metas.save(imgDir, names); err != nil {
		opts.Log("Cache des métadonnées non enregistré : %v", err) // simple ralentissement au prochain passage
	}

	pages, err := render(opts, items)
	if err != nil {
		return nil, err
	}
	res.Pages = pages
	return res, nil
}

// netJobs : parallélisme par défaut quand les vidéos sont sur un partage
// réseau. Chaque capture est une lecture à un endroit différent du fichier ;
// une douzaine de vidéos lues à la fois saturent un serveur à disques durs
// (et le lien) sans aller plus vite.
const netJobs = 3

// markerFile signale un répertoire produit par ce programme : il autorise les
// générations suivantes à y réécrire (et à y supprimer les fichiers obsolètes).
const markerFile = ".videogallery"

const markerContent = "Répertoire généré par The Video Gallery.\nSon contenu est réécrit à chaque génération : n'y rangez rien d'autre.\n"

// ownedName reconnaît un fichier que nous produisons nous-mêmes, pour accepter
// les galeries créées avant l'introduction du marqueur.
func ownedName(name string, isDir bool) bool {
	if isDir {
		return name == "img"
	}
	switch name {
	case markerFile, "index.html", "style.css", "favicon.ico":
		return true
	}
	return ourPage.MatchString(name)
}

// checkOutDir refuse d'écrire dans un répertoire existant qui contient autre
// chose qu'une galerie : la génération y écrase index.html et y supprime les
// pages et images obsolètes.
func checkOutDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil // il sera créé
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !ownedName(e.Name(), e.IsDir()) {
			return fmt.Errorf("%s contient déjà des fichiers étrangers à une galerie (par exemple %q) ;\n"+
				"la génération y écraserait des pages et y supprimerait des images.\n"+
				"Choisissez un répertoire de sortie vide ou dédié", dir, e.Name())
		}
	}
	return nil
}

// buildImages produit planche et miniature de chaque vidéo, en parallèle.
// Les vidéos en échec sont écartées de la galerie et rapportées à part.
func buildImages(ctx context.Context, opts Options, videos []Video, imgDir string, metas *metaCache) ([]item, []Failure) {
	results := make([]*item, len(videos))
	errs := make([]error, len(videos))

	names := videoNames(videos)
	sem := make(chan struct{}, opts.Jobs)
	var wg sync.WaitGroup
	var done atomic.Int32
	for i, v := range videos {
		if ctx.Err() != nil {
			break // annulation : on ne lance plus rien
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() { opts.Progress(int(done.Add(1)), len(videos)) }()
			it, err := buildItem(ctx, opts, v, imgDir, metas)
			metas.checkpoint(imgDir, names)
			if err != nil {
				errs[i] = err
				if ctx.Err() == nil {
					opts.Log("ÉCHEC %s: %v", v.Name, err)
				}
				return
			}
			results[i] = it
		}()
	}
	wg.Wait()

	var items []item
	var failures []Failure
	for i, it := range results {
		if it != nil {
			items = append(items, *it)
		} else if errs[i] != nil && ctx.Err() == nil {
			failures = append(failures, Failure{Video: videos[i].Name, Err: errs[i]})
		}
	}
	return items, failures
}

func videoNames(videos []Video) []string {
	names := make([]string, len(videos))
	for i, v := range videos {
		names[i] = v.Name
	}
	return names
}

// sheetLayout identifie la mise en page de la planche (en-tête, horodatages).
// Elle fait partie du nom de fichier : en la changeant, les planches en cache
// d'une ancienne mise en page sont régénérées puis purgées.
const sheetLayout = 2

// thumbWidth choisit la largeur de la miniature d'index selon le nombre de
// colonnes : la page étire les miniatures sur toute sa largeur, il faut donc
// plus de pixels quand il y a peu de colonnes (calé sur un écran de 1920 px,
// en demi-résolution pour garder des fichiers légers).
func thumbWidth(cols int) int {
	w := (900/cols + 39) / 40 * 40
	return min(max(w, 160), 640)
}

// maxBaseName borne les noms de fichiers générés : la plupart des systèmes de
// fichiers (ext4, NTFS, APFS) limitent un nom à 255 octets, et un nom de vidéo
// peut à lui seul en faire 250.
const maxBaseName = 200

// shorten raccourcit le nom de la vidéo si le nom d'image dépasse la limite,
// en gardant un suffixe issu du nom complet pour éviter toute collision.
func shorten(prefix, name, suffix string) string {
	full := prefix + name + suffix
	if len(full) <= maxBaseName {
		return full
	}
	sum := sha256.Sum256([]byte(name))
	tag := "~" + hex.EncodeToString(sum[:4])
	keep := maxBaseName - len(prefix) - len(suffix) - len(tag)
	if keep < 0 {
		keep = 0
	}
	// coupe sur une frontière de caractère pour rester en UTF-8 valide
	for keep > 0 && !utf8.RuneStart(name[keep]) {
		keep--
	}
	return prefix + name[:keep] + tag + suffix
}

func imageNames(opts Options, v Video) (sheet, thumb string) {
	return shorten(fmt.Sprintf("%d-v%d-", opts.FrameWidth, sheetLayout), v.Name, ".sheet.jpg"),
		shorten(fmt.Sprintf("t%d-", thumbWidth(opts.Cols)), v.Name, ".thumb.jpg")
}

func buildItem(ctx context.Context, opts Options, v Video, imgDir string, metas *metaCache) (*item, error) {
	sheetName, thumbName := imageNames(opts, v)
	sheetPath := filepath.Join(imgDir, sheetName)
	thumbPath := filepath.Join(imgDir, thumbName)
	it := &item{Video: v, Sheet: "img/" + sheetName, Thumb: "img/" + thumbName}

	meta, known := metas.get(v)
	if !known || opts.Force {
		var err error
		if meta, err = opts.Tools.Probe(ctx, v.Path); err != nil {
			return nil, err
		}
		metas.put(v, meta)
	}
	it.Meta = meta

	if !opts.Force && upToDate(v, sheetPath) && upToDate(v, thumbPath) {
		var err1, err2 error
		it.SheetW, it.SheetH, err1 = imageSize(sheetPath)
		it.ThumbW, it.ThumbH, err2 = imageSize(thumbPath)
		if err1 == nil && err2 == nil {
			it.cached = true
			return it, nil
		}
		// image en cache illisible : on la régénère
	}

	opts.Log("Génération de la planche de %s", v.Name)
	var err error
	tw := thumbWidth(opts.Cols)
	times := FrameTimes(meta.Duration)
	frames := make([]image.Image, sheetFrames)
	for i, t := range times {
		w := opts.FrameWidth
		if i == thumbFrame {
			w = max(w, tw) // sert aussi de miniature : une seule extraction
		}
		if frames[i], err = opts.Tools.Frame(ctx, v.Path, t, w); err != nil {
			return nil, err
		}
	}

	// La miniature est la capture centrale (sans horodatage), à sa propre
	// largeur ; elle est extraite une seule fois, à la plus grande des deux
	// largeurs, puis réduite ici pour l'autre usage (un appel à ffmpeg de
	// moins par vidéo, soit une lecture de moins sur un partage réseau).
	thumb := frames[thumbFrame]
	if size := frames[0].Bounds().Size(); thumb.Bounds().Size() != size {
		frames[thumbFrame] = resize(thumb, size.X, size.Y)
	}
	if b := thumb.Bounds(); tw < b.Dx() {
		thumb = resize(thumb, tw, evenHeight(b.Dx(), b.Dy(), tw))
	}
	sheet := ComposeSheet(frames, times, SheetInfo{
		Name: v.Name, Size: v.Size, Width: meta.Width, Height: meta.Height, Duration: meta.Duration,
	})
	if err := writeJPEG(sheetPath, sheet); err != nil {
		return nil, err
	}

	if err := writeJPEG(thumbPath, thumb); err != nil {
		return nil, err
	}
	it.SheetW, it.SheetH = sheet.Bounds().Dx(), sheet.Bounds().Dy()
	tb := thumb.Bounds()
	it.ThumbW, it.ThumbH = tb.Dx(), tb.Dy()
	return it, nil
}

// resize réduit une image aux dimensions données (Catmull-Rom, comme le
// filtre bicubique de ffmpeg).
func resize(src image.Image, w, h int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Src, nil)
	return dst
}

// evenHeight : hauteur, arrondie au pair, d'une image w×h ramenée à la largeur
// width (comme « scale=width:-2 » de ffmpeg).
func evenHeight(w, h, width int) int {
	return max(2, int(math.Round(float64(h)*float64(width)/float64(w)/2))*2)
}

// upToDate: le fichier existe et n'est pas plus ancien que la vidéo.
func upToDate(v Video, path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.ModTime().Before(v.ModTime)
}

// pruneImages supprime les images de img/ qui ne servent plus (vidéo
// supprimée, autre largeur de capture, autre mise en page). Seuls les fichiers
// portant nos suffixes (.sheet.jpg, .thumb.jpg) sont touchés.
func pruneImages(imgDir string, items []item) {
	keep := make(map[string]bool, 2*len(items))
	for _, it := range items {
		keep[filepath.Base(it.Sheet)] = true
		keep[filepath.Base(it.Thumb)] = true
	}
	entries, err := os.ReadDir(imgDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		n := e.Name()
		ours := strings.HasSuffix(n, ".sheet.jpg") || strings.HasSuffix(n, ".thumb.jpg")
		if !e.IsDir() && ours && !keep[n] {
			os.Remove(filepath.Join(imgDir, n))
		}
	}
}
