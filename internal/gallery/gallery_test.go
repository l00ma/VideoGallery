package gallery

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFrameTimes(t *testing.T) {
	got := FrameTimes(100)
	if got[0] != 10 || got[4] != 50 || got[8] != 90 {
		t.Errorf("FrameTimes(100) = %v, attendu 10, 50 et 90 aux extrémités et au centre", got)
	}
}

func whiteFrame(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	return img
}

func TestComposeSheet(t *testing.T) {
	const w, h = 320, 180
	frames := make([]image.Image, sheetFrames)
	for i := range frames {
		frames[i] = whiteFrame(w, h)
	}
	info := SheetInfo{Name: "vacances à hossegor ; vlog 1.mp4", Size: 52_913_499, Width: 1920, Height: 1080, Duration: 131.5}
	sheet := ComposeSheet(frames, FrameTimes(info.Duration), info)

	gridW, gridH := 3*w+2*sheetGap, 3*h+2*sheetGap
	if sheet.Bounds().Dx() != gridW {
		t.Fatalf("largeur = %d, attendu %d", sheet.Bounds().Dx(), gridW)
	}
	headerH := sheet.Bounds().Dy() - gridH
	if headerH < 30 || headerH > 90 {
		t.Fatalf("hauteur d'en-tête improbable: %d", headerH)
	}

	// L'en-tête contient du texte clair sur fond sombre.
	light := 0
	for y := 0; y < headerH; y++ {
		for x := 0; x < gridW; x++ {
			if sheet.RGBAAt(x, y).R > 200 {
				light++
			}
		}
	}
	if light < 200 {
		t.Errorf("l'en-tête ne semble pas contenir de texte (%d pixels clairs)", light)
	}

	// Chaque capture porte un horodatage en bas à gauche, et seulement là.
	for i := 0; i < sheetFrames; i++ {
		x0 := (i % 3) * (w + sheetGap)
		y0 := headerH + (i/3)*(h+sheetGap)
		if sheet.RGBAAt(x0+w-3, y0+3).R != 255 { // coin haut droit intact
			t.Errorf("capture %d: coin haut-droit modifié", i)
		}
		dark := 0
		for y := y0 + h - 40; y < y0+h; y++ {
			for x := x0; x < x0+80; x++ {
				if sheet.RGBAAt(x, y).R < 128 {
					dark++
				}
			}
		}
		if dark < 100 {
			t.Errorf("capture %d: pas d'horodatage en bas à gauche (%d pixels sombres)", i, dark)
		}
	}

	// Les captures d'origine ne sont pas modifiées (la miniature en est une).
	for i, f := range frames {
		if r, g, b, _ := f.At(10, h-10).RGBA(); r != 0xffff || g != 0xffff || b != 0xffff {
			t.Fatalf("la capture %d a été modifiée par la composition", i)
		}
	}
}

func TestComposeSheetNarrowDoesNotPanic(t *testing.T) {
	frames := make([]image.Image, sheetFrames)
	for i := range frames {
		frames[i] = whiteFrame(80, 45)
	}
	sheet := ComposeSheet(frames, FrameTimes(3700), SheetInfo{Name: strings.Repeat("très long nom ", 20) + ".mp4", Size: 3_200_000_000, Width: 3840, Height: 2160, Duration: 3700})
	if sheet.Bounds().Dx() != 3*80+2*sheetGap {
		t.Errorf("largeur inattendue: %d", sheet.Bounds().Dx())
	}
}

func TestFormats(t *testing.T) {
	clocks := map[float64]string{0: "0:00", 59.4: "0:59", 59.6: "1:00", 205.2: "3:25", 3599.6: "1:00:00", 3723: "1:02:03"}
	for in, want := range clocks {
		if got := formatClock(in); got != want {
			t.Errorf("formatClock(%v) = %q, attendu %q", in, got, want)
		}
	}
	sizes := map[int64]string{
		52_913_499:    "52,9 Mo",
		88_649_319:    "88,6 Mo",
		999_999_999:   "1000,0 Mo",
		3_200_000_000: "3,20 Go",
		42_000:        "42 Ko",
	}
	for in, want := range sizes {
		if got := formatSize(in); got != want {
			t.Errorf("formatSize(%d) = %q, attendu %q", in, got, want)
		}
	}
}

func TestThumbWidth(t *testing.T) {
	// moins de colonnes => miniatures plus larges, bornées à [160, 640]
	want := map[int]int{1: 640, 2: 480, 3: 320, 4: 240, 5: 200, 6: 160, 8: 160, 20: 160}
	for cols, w := range want {
		if got := thumbWidth(cols); got != w {
			t.Errorf("thumbWidth(%d) = %d, attendu %d", cols, got, w)
		}
	}
}

func TestScan(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"b.mp4", "A.WEBM", "c.ogg", "d.avi", "e.mp4.jpg", "notes.txt", "f.MOV", "g.m4v", "h.mkv"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub.mp4"), 0o755); err != nil {
		t.Fatal(err)
	}
	vids, err := Scan(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, v := range vids {
		names = append(names, v.Name)
	}
	if got := strings.Join(names, ","); got != "A.WEBM,b.mp4,c.ogg,f.MOV,g.m4v" {
		t.Errorf("Scan = %s", got)
	}
	if vids[0].MIME != "video/webm" {
		t.Errorf("MIME = %s", vids[0].MIME)
	}
	// un .mov annoncé en video/quicktime serait écarté par Firefox et Chromium
	if vids[3].MIME != "video/mp4" || vids[4].MIME != "video/mp4" {
		t.Errorf("MIME .mov/.m4v = %s, %s", vids[3].MIME, vids[4].MIME)
	}
}

// Une vidéo HEVC (iPhone) dans un .mov : elle entre dans la galerie, mais sa
// page prévient qu'elle peut ne pas être lue et propose d'ouvrir le fichier ;
// une vidéo H.264 n'a pas d'avertissement.
func TestUnsupportedCodecWarning(t *testing.T) {
	tools, err := FindTools()
	if err != nil {
		t.Skip("ffmpeg indisponible")
	}
	dir := t.TempDir()
	for n, codec := range map[string]string{"iphone.mov": "libx265", "camera.mov": "libx264"} {
		cmd := exec.Command(tools.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=10:duration=2",
			"-c:v", codec, "-pix_fmt", "yuv420p", filepath.Join(dir, n))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("encodeur %s indisponible: %v: %s", codec, err, out)
		}
	}
	var logs []string
	opts := Options{VideoDir: dir, Cols: 2, Rows: 2, FrameWidth: 120, Tools: tools,
		Log: func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) }}
	if res, err := Run(context.Background(), opts); err != nil || res.Videos != 2 {
		t.Fatalf("Run: %+v, %v", res, err)
	}
	camera, _ := os.ReadFile(filepath.Join(dir, "pages", "video-1.html"))
	iphone, _ := os.ReadFile(filepath.Join(dir, "pages", "video-2.html"))
	if strings.Contains(string(camera), `class="warn"`) {
		t.Error("avertissement sur une vidéo H.264")
	}
	for _, want := range []string{`class="warn"`, "HEVC (H.265)", `<a href="../iphone.mov">`, `type="video/mp4"`} {
		if !strings.Contains(string(iphone), want) {
			t.Errorf("page HEVC sans %q:\n%s", want, iphone)
		}
	}
	if !strings.Contains(strings.Join(logs, "\n"), "iphone.mov est en HEVC") {
		t.Errorf("pas d'avertissement dans le journal:\n%s", strings.Join(logs, "\n"))
	}
}

func TestURLPath(t *testing.T) {
	cases := map[string]string{
		"../clip A.mp4":   "../clip%20A.mp4",
		"../l'été #2.mp4": "../l%27%C3%A9t%C3%A9%20%232.mp4",
		"img/a?b.jpg":     "img/a%3Fb.jpg",
	}
	for in, want := range cases {
		if got := string(urlPath(in)); got != want {
			t.Errorf("urlPath(%q) = %q, attendu %q", in, got, want)
		}
	}
}

func TestOptionsValidation(t *testing.T) {
	dir := t.TempDir()
	base := Options{VideoDir: dir, Cols: 4, Rows: 5, FrameWidth: 320}
	for name, mutate := range map[string]func(*Options){
		"couleur invalide":    func(o *Options) { o.Background = "red" },
		"injection css":       func(o *Options) { o.Foreground = "#fff;}body{display:none" },
		"grille nulle":        func(o *Options) { o.Cols = 0 },
		"largeur hors bornes": func(o *Options) { o.FrameWidth = 10 },
		"répertoire inconnu":  func(o *Options) { o.VideoDir = filepath.Join(dir, "absent") },
	} {
		o := base
		mutate(&o)
		if err := o.applyDefaults(); err == nil {
			t.Errorf("%s: erreur attendue", name)
		}
	}
	o := base
	if err := o.applyDefaults(); err != nil {
		t.Fatalf("options valides refusées: %v", err)
	}
}

// Test d'intégration : nécessite ffmpeg (ignoré sinon).
func TestRunEndToEnd(t *testing.T) {
	tools, err := FindTools()
	if err != nil {
		t.Skip("ffmpeg indisponible")
	}
	dir := t.TempDir()
	// dates de tournage : « trois » est la plus ancienne, « un » n'en a pas
	created := map[string]string{"d'eux #2.mp4": "2021-05-01T10:00:00Z", "trois.mp4": "2019-05-01T10:00:00Z"}
	for _, n := range []string{"un.mp4", "d'eux #2.mp4", "trois.mp4"} {
		args := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=10:duration=4"}
		if c := created[n]; c != "" {
			args = append(args, "-metadata", "creation_time="+c)
		}
		cmd := exec.Command(tools.FFmpeg, append(args, filepath.Join(dir, n))...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("génération de %s: %v: %s", n, err, out)
		}
	}
	// vidéo corrompue : ne doit pas faire échouer le reste
	if err := os.WriteFile(filepath.Join(dir, "casse.mp4"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := Options{VideoDir: dir, Cols: 2, Rows: 1, FrameWidth: 120, Tools: tools}
	res, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Videos != 3 || res.Pages != 2 || res.Generated != 3 || len(res.Failed) != 1 {
		t.Fatalf("résultat inattendu: %+v", res)
	}
	for _, f := range []string{"index.html", "page-2.html", "video-3.html", "style.css", "favicon.ico"} {
		if _, err := os.Stat(filepath.Join(dir, "pages", f)); err != nil {
			t.Errorf("fichier manquant: %s", f)
		}
	}
	page, _ := os.ReadFile(filepath.Join(dir, "pages", "video-1.html"))
	if !strings.Contains(string(page), `src="../d%27eux%20%232.mp4"`) {
		t.Errorf("source vidéo mal encodée:\n%s", page)
	}
	// 9 captures cliquables, qui ouvrent la vidéo à l'instant de la capture
	// (vidéo de 4 s : la 1re capture est à 0,4 s)
	if n := strings.Count(string(page), `class="cell"`); n != 9 {
		t.Errorf("%d zones cliquables sur la planche, attendu 9", n)
	}
	for _, want := range []string{`href="../d%27eux%20%232.mp4#t=0.40"`, `data-t="0.40"`, `video.currentTime = t;`} {
		if !strings.Contains(string(page), want) {
			t.Errorf("page vidéo sans %q (lecture à l'instant d'une capture):\n%s", want, page)
		}
	}

	index, _ := os.ReadFile(filepath.Join(dir, "pages", "index.html"))
	for _, want := range []string{`<span class="duration">0:04</span>`, `160 × 90`} {
		if !strings.Contains(string(index), want) {
			t.Errorf("index sans %q:\n%s", want, index)
		}
	}

	// 2e passage : tout vient du cache, métadonnées comprises (un ffprobe
	// introuvable ferait échouer toute vidéo qu'on tenterait d'analyser)
	cachedOpts := opts
	cachedOpts.Tools.FFprobe = filepath.Join(dir, "ffprobe-absent")
	res, err = Run(context.Background(), cachedOpts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Cached != 3 || res.Generated != 0 || len(res.Failed) != 1 {
		t.Errorf("cache non utilisé: %+v", res)
	}

	// galerie antérieure au cache de métadonnées : seul ffprobe est relancé
	if err := os.Remove(filepath.Join(dir, "pages", "img", metaFile)); err != nil {
		t.Fatal(err)
	}
	if res, err = Run(context.Background(), opts); err != nil || res.Cached != 3 {
		t.Fatalf("images en cache non réutilisées sans meta.json: %+v, %v", res, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "pages", "img", metaFile)); err != nil {
		t.Errorf("meta.json non recréé: %v", err)
	}

	// tri par date : la date de tournage prime, « un » (fichier du jour) est la plus récente
	dateOpts := opts
	dateOpts.Order = OrderDate
	if _, err := Run(context.Background(), dateOpts); err != nil {
		t.Fatal(err)
	}
	for n, want := range []string{"trois.mp4", "d%27eux%20%232.mp4", "un.mp4"} {
		page, _ := os.ReadFile(filepath.Join(dir, "pages", videoFile(n+1)))
		if !strings.Contains(string(page), `src="../`+want+`"`) {
			t.Errorf("tri par date : vidéo %d devrait être %s", n+1, want)
		}
	}
	index, _ = os.ReadFile(filepath.Join(dir, "pages", "index.html"))
	if !strings.Contains(string(index), "01/05/2019") {
		t.Errorf("date de tournage absente de l'index:\n%s", index)
	}

	// changement de largeur : les anciennes images sont purgées
	opts.FrameWidth = 100
	if _, err = Run(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	old, _ := filepath.Glob(filepath.Join(dir, "pages", "img", "120-*"))
	if len(old) != 0 {
		t.Errorf("images obsolètes non purgées: %v", old)
	}
}

// Une vidéo filmée en portrait (rotation dans les métadonnées) doit être
// annoncée dans la résolution que voit le spectateur.
func TestProbeRotation(t *testing.T) {
	tools, err := FindTools()
	if err != nil {
		t.Skip("ffmpeg indisponible")
	}
	dir := t.TempDir()
	base, rot := filepath.Join(dir, "base.mp4"), filepath.Join(dir, "rot.mp4")
	run := func(args ...string) {
		if out, err := exec.Command(tools.FFmpeg, append([]string{"-v", "error", "-y"}, args...)...).CombinedOutput(); err != nil {
			t.Skipf("ffmpeg ne sait pas générer ce cas (%v): %s", err, out)
		}
	}
	run("-f", "lavfi", "-i", "testsrc2=size=640x360:rate=10:duration=2", "-c:v", "libx264", "-pix_fmt", "yuv420p", base)
	run("-display_rotation", "90", "-i", base, "-c", "copy", rot)

	m, err := tools.Probe(context.Background(), base)
	if err != nil || m.Width != 640 || m.Height != 360 {
		t.Errorf("paysage: %+v, %v", m, err)
	}
	m, err = tools.Probe(context.Background(), rot)
	if err != nil || m.Width != 360 || m.Height != 640 {
		t.Errorf("portrait (rotation 90°): %+v, %v — largeur et hauteur doivent être échangées", m, err)
	}
	if m.Duration < 1.5 || m.Duration > 2.5 {
		t.Errorf("durée = %v", m.Duration)
	}
}

func TestShortenKeepsNamesWithinFilesystemLimit(t *testing.T) {
	long := strings.Repeat("é", 200) + ".mp4" // 400 octets + extension
	opts := Options{FrameWidth: 320, Cols: 4}
	sheet, thumb := imageNames(opts, Video{Name: long})
	for _, n := range []string{sheet, thumb} {
		if len(n) > maxBaseName {
			t.Errorf("nom de %d octets, limite %d : %s", len(n), maxBaseName, n)
		}
		if !utf8.ValidString(n) {
			t.Errorf("nom tronqué au milieu d'un caractère : %q", n)
		}
	}
	if sheet == thumb {
		t.Error("planche et miniature doivent avoir des noms distincts")
	}
	// deux noms longs différents ne doivent pas produire le même fichier
	other, _ := imageNames(opts, Video{Name: strings.Repeat("é", 199) + "x.mp4"})
	if other == sheet {
		t.Error("collision entre deux noms longs différents")
	}
	// un nom court n'est pas modifié
	s, _ := imageNames(opts, Video{Name: "court.mp4"})
	if s != "320-v2-court.mp4.sheet.jpg" {
		t.Errorf("nom court altéré : %s", s)
	}
}

func TestCheckOutDirProtectsForeignFiles(t *testing.T) {
	// répertoire inexistant : accepté (il sera créé)
	if err := checkOutDir(filepath.Join(t.TempDir(), "absent")); err != nil {
		t.Errorf("répertoire absent refusé : %v", err)
	}
	// répertoire vide : accepté
	if err := checkOutDir(t.TempDir()); err != nil {
		t.Errorf("répertoire vide refusé : %v", err)
	}
	// galerie créée avant l'introduction du marqueur : acceptée
	old := t.TempDir()
	for _, n := range []string{"index.html", "page-2.html", "video-1.html", "style.css", "favicon.ico"} {
		os.WriteFile(filepath.Join(old, n), nil, 0o644)
	}
	os.Mkdir(filepath.Join(old, "img"), 0o755)
	if err := checkOutDir(old); err != nil {
		t.Errorf("galerie existante refusée : %v", err)
	}
	// répertoire contenant autre chose : refusé
	foreign := t.TempDir()
	os.WriteFile(filepath.Join(foreign, "notes.txt"), []byte("x"), 0o644)
	if err := checkOutDir(foreign); err == nil {
		t.Error("un répertoire contenant des fichiers étrangers doit être refusé")
	}
}

// Un répertoire de sortie contenant des fichiers personnels ne doit jamais
// être écrasé, et la purge ne doit pas toucher aux images qui ne sont pas les
// nôtres.
func TestRunRefusesForeignOutDirAndPrunePreservesOtherImages(t *testing.T) {
	tools, err := FindTools()
	if err != nil {
		t.Skip("ffmpeg indisponible")
	}
	videoDir := t.TempDir()
	cmd := exec.Command(tools.FFmpeg, "-v", "error", "-f", "lavfi",
		"-i", "testsrc2=size=160x90:rate=10:duration=2", filepath.Join(videoDir, "a.mp4"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, out)
	}

	site := t.TempDir()
	home := filepath.Join(site, "index.html")
	os.WriteFile(home, []byte("<h1>mon site</h1>"), 0o644)
	os.WriteFile(filepath.Join(site, "page-contact.html"), []byte("contact"), 0o644)

	opts := Options{VideoDir: videoDir, OutDir: site, Cols: 2, Rows: 2, FrameWidth: 120, Tools: tools}
	if _, err := Run(context.Background(), opts); err == nil {
		t.Fatal("Run doit refuser un répertoire de sortie contenant des fichiers étrangers")
	}
	if b, _ := os.ReadFile(home); string(b) != "<h1>mon site</h1>" {
		t.Error("le index.html existant a été écrasé")
	}
	if _, err := os.Stat(filepath.Join(site, "page-contact.html")); err != nil {
		t.Error("page-contact.html a été supprimé")
	}

	// Dans une vraie galerie, une image étrangère déposée dans img/ est conservée.
	out := filepath.Join(t.TempDir(), "galerie")
	opts.OutDir = out
	if _, err := Run(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	photo := filepath.Join(out, "img", "photo-vacances.jpg")
	os.WriteFile(photo, []byte("photo"), 0o644)
	opts.Force = true
	if _, err := Run(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(photo); err != nil {
		t.Error("une image étrangère de img/ a été supprimée par la purge")
	}
}
