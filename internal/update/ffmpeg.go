package update

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Téléchargement de ffmpeg pour Windows, où il n'est pas installé d'office :
// build « lgpl-shared » de BtbN (ffmpeg.exe, ffprobe.exe et leurs DLL), de la
// dernière branche stable, installé dans le sous-dossier FFmpegDir du
// programme (gallery.FindTools y regarde). LGPL suffit : le programme ne fait
// que décoder ; « shared » pèse 77 Mo à télécharger contre 172 pour un build
// statique. Comme pour nos propres mises à jour, l'archive est comparée à
// l'empreinte publiée dans la même release (intégrité, pas de signature : la
// confiance repose sur le compte GitHub de BtbN).

// FFmpegRepo est le dépôt GitHub des builds de ffmpeg.
const FFmpegRepo = "BtbN/FFmpeg-Builds"

// FFmpegDir est le sous-dossier du programme où ffmpeg est installé.
const FFmpegDir = "ffmpeg"

// maxFFmpegFile borne un fichier extrait (avcodec-*.dll fait ~90 Mo).
const maxFFmpegFile = 300 << 20

// « ffmpeg-n9.0-latest-win64-lgpl-shared-9.0.zip » : branche stable 9.0.
var ffmpegAsset = regexp.MustCompile(`^ffmpeg-n(\d+)\.(\d+)-latest-win64-lgpl-shared-\d+\.\d+\.zip$`)

// FFmpegBuild décrit l'archive de ffmpeg à installer.
type FFmpegBuild struct {
	Version string `json:"version"` // branche : « 9.0 »
	Size    int64  `json:"size"`    // octets à télécharger
	Name    string `json:"name"`    // nom de l'archive

	url, sumsURL string
}

// FFmpegSupported indique si le téléchargement de ffmpeg est proposé sur la
// plateforme du client (Windows 64 bits ; ailleurs, le gestionnaire de
// paquets du système s'en charge).
func (c *Client) FFmpegSupported() bool {
	return c.GOOS == "windows" && c.GOARCH == "amd64"
}

// LatestFFmpeg choisit, dans la dernière release de BtbN, l'archive
// lgpl-shared de la branche stable la plus récente.
func (c *Client) LatestFFmpeg(ctx context.Context) (*FFmpegBuild, error) {
	if !c.FFmpegSupported() {
		return nil, fmt.Errorf("le téléchargement de ffmpeg n'est proposé que sous Windows 64 bits (ici %s/%s)", c.GOOS, c.GOARCH)
	}
	gr, err := c.latestRelease(ctx, FFmpegRepo, "VideoGallery-ffmpeg")
	if err != nil {
		return nil, err
	}
	var best *FFmpegBuild
	var bestMajor, bestMinor int
	sums := ""
	for _, a := range gr.Assets {
		if a.Name == "checksums.sha256" {
			sums = a.URL
			continue
		}
		m := ffmpegAsset.FindStringSubmatch(a.Name)
		if m == nil {
			continue
		}
		major, _ := strconv.Atoi(m[1])
		minor, _ := strconv.Atoi(m[2])
		if best == nil || major > bestMajor || (major == bestMajor && minor > bestMinor) {
			best = &FFmpegBuild{Version: m[1] + "." + m[2], Size: a.Size, Name: a.Name, url: a.URL}
			bestMajor, bestMinor = major, minor
		}
	}
	if best == nil {
		return nil, fmt.Errorf("aucune archive ffmpeg pour Windows dans la dernière release de %s", FFmpegRepo)
	}
	if sums == "" {
		return nil, fmt.Errorf("la release de %s ne publie pas d'empreintes (checksums.sha256)", FFmpegRepo)
	}
	best.sumsURL = sums
	return best, nil
}

// InstallFFmpeg télécharge b, vérifie son empreinte, puis installe ffmpeg.exe,
// ffprobe.exe, leurs DLL et la licence dans appDir/FFmpegDir, en remplaçant
// une installation précédente. Rien n'est touché tant que le téléchargement
// n'est pas complet et vérifié.
func (c *Client) InstallFFmpeg(ctx context.Context, b *FFmpegBuild, appDir string, progress func(done, total int64)) error {
	if b == nil || b.url == "" {
		return errors.New("aucune archive ffmpeg à installer")
	}
	if err := checkWritable(appDir); err != nil {
		return fmt.Errorf("le dossier du programme %s n'est pas modifiable (%v) : déplacez VideoGallery dans un dossier à vous (Documents, Bureau...)", appDir, err)
	}
	// Dossier de travail dans appDir : les renommages restent sur le même disque.
	work, err := os.MkdirTemp(appDir, ".videogallery-ffmpeg-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	sums, err := c.fetchSums(ctx, b.sumsURL)
	if err != nil {
		return err
	}
	want, ok := sums[b.Name]
	if !ok {
		return fmt.Errorf("%s absent des empreintes publiées", b.Name)
	}
	archive := filepath.Join(work, "ffmpeg.zip")
	got, err := c.download(ctx, b.url, archive, progress)
	if err != nil {
		return err
	}
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("empreinte SHA-256 incorrecte pour %s (téléchargement corrompu ?)", b.Name)
	}

	staged := filepath.Join(work, FFmpegDir)
	if err := os.Mkdir(staged, 0o755); err != nil {
		return err
	}
	top := strings.TrimSuffix(b.Name, ".zip")
	if err := extractFFmpeg(archive, top, staged); err != nil {
		return err
	}
	for _, need := range []string{"ffmpeg.exe", "ffprobe.exe"} {
		if _, err := os.Stat(filepath.Join(staged, need)); err != nil {
			return fmt.Errorf("%s absent de l'archive %s", need, b.Name)
		}
	}

	// L'ancienne installation est écartée dans le dossier de travail (supprimé
	// à la fin), la nouvelle prend sa place.
	dst := filepath.Join(appDir, FFmpegDir)
	if _, err := os.Lstat(dst); err == nil {
		if err := os.Rename(dst, filepath.Join(work, "ancien")); err != nil {
			return fmt.Errorf("remplacement de %s : %w (ffmpeg est-il en cours d'utilisation ?)", dst, err)
		}
	}
	return os.Rename(staged, dst)
}

// ffmpegFile dit si une entrée de l'archive est à installer, et sous quel
// nom : « <top>/bin/ffmpeg.exe », « <top>/bin/ffprobe.exe »,
// « <top>/bin/<bibliothèque>.dll » et « <top>/LICENSE.txt ». ffplay.exe et
// le reste sont ignorés, de même que tout chemin inattendu (« .. »,
// sous-dossier).
func ffmpegFile(entry, top string) string {
	entry = path.Clean(strings.ReplaceAll(entry, "\\", "/"))
	dir, name := path.Split(entry)
	dir = strings.TrimSuffix(dir, "/")
	switch {
	case dir == top && name == "LICENSE.txt":
		return name
	case dir != top+"/bin":
		return ""
	case name == "ffmpeg.exe", name == "ffprobe.exe":
		return name
	case strings.HasSuffix(strings.ToLower(name), ".dll") && !strings.ContainsAny(name, `/\:`):
		return name
	}
	return ""
}

func extractFFmpeg(archive, top, dest string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, zf := range zr.File {
		if !zf.Mode().IsRegular() {
			continue
		}
		name := ffmpegFile(zf.Name, top)
		if name == "" {
			continue
		}
		if err := extractOne(zf, filepath.Join(dest, name)); err != nil {
			return fmt.Errorf("extraction de %s : %w", name, err)
		}
	}
	return nil
}

func extractOne(zf *zip.File, dst string) error {
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(rc, maxFFmpegFile+1))
	if err == nil && n > maxFFmpegFile {
		err = errors.New("fichier trop volumineux")
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}
