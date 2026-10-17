// Package update vérifie si une nouvelle version est publiée sur GitHub et
// l'installe à la place de l'ancienne (bibliothèque standard uniquement).
//
// Intégrité : l'archive téléchargée est comparée à l'empreinte SHA-256 du
// fichier SHA256SUMS de la même release. Cela détecte un téléchargement
// corrompu ou tronqué, pas une release malveillante : les archives ne sont pas
// signées, la confiance repose sur le compte GitHub du projet.
package update

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Repo est le dépôt GitHub qui publie les releases.
const Repo = "l00ma/VideoGallery"

const (
	maxFileSize    = 100 << 20 // un exécutable décompressé
	maxArchiveSize = 200 << 20 // l'archive téléchargée
)

// Release décrit la dernière version publiée.
type Release struct {
	Version string `json:"version"` // sans « v » : « 2.0.1 »
	Name    string `json:"name"`
	Notes   string `json:"notes"`
	URL     string `json:"url"` // page de la release

	archiveURL, sumsURL string
	archiveName         string
}

// Client interroge GitHub. Les champs exportés servent aux tests.
type Client struct {
	APIURL string // défaut : https://api.github.com
	HTTP   *http.Client
	GOOS   string // défaut : runtime.GOOS
	GOARCH string // défaut : runtime.GOARCH
}

// NewClient renvoie un client prêt à l'emploi.
func NewClient() *Client {
	return &Client{
		APIURL: "https://api.github.com",
		HTTP: &http.Client{CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			if req.URL.Scheme != "https" && !isLoopback(req.URL) {
				return fmt.Errorf("redirection refusée vers %s", req.URL.Redacted())
			}
			return nil
		}},
		GOOS:   runtime.GOOS,
		GOARCH: runtime.GOARCH,
	}
}

func isLoopback(u *url.URL) bool {
	h := u.Hostname()
	return h == "127.0.0.1" || h == "localhost" || h == "::1"
}

type ghRelease struct {
	TagName    string `json:"tag_name"`
	Name       string `json:"name"`
	Body       string `json:"body"`
	HTMLURL    string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

// Latest renvoie la dernière release si elle est plus récente que current,
// sinon nil. Une release sans archive pour la plateforme courante est
// signalée par une erreur : on ne propose pas une mise à jour impossible.
func (c *Client) Latest(ctx context.Context, current string) (*Release, error) {
	gr, err := c.latestRelease(ctx, Repo, "VideoGallery/"+current)
	if err != nil {
		return nil, err
	}
	if gr.Draft || gr.Prerelease {
		return nil, nil
	}
	latest := strings.TrimPrefix(gr.TagName, "v")
	if !Newer(latest, current) {
		return nil, nil
	}

	rel := &Release{Version: latest, Name: gr.Name, Notes: gr.Body, URL: gr.HTMLURL}
	ext := ".tar.gz"
	if c.GOOS == "windows" {
		ext = ".zip"
	}
	rel.archiveName = fmt.Sprintf("VideoGallery-%s-%s-%s%s", latest, c.GOOS, c.GOARCH, ext)
	for _, a := range gr.Assets {
		switch a.Name {
		case rel.archiveName:
			rel.archiveURL = a.URL
		case "SHA256SUMS":
			rel.sumsURL = a.URL
		}
	}
	if rel.archiveURL == "" || rel.sumsURL == "" {
		return nil, fmt.Errorf("la version %s n'a pas d'archive %s/%s", latest, c.GOOS, c.GOARCH)
	}
	return rel, nil
}

// latestRelease interroge l'API GitHub (sans authentification) sur la
// dernière release du dépôt « propriétaire/nom ».
func (c *Client) latestRelease(ctx context.Context, repo, userAgent string) (*ghRelease, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.APIURL+"/repos/"+repo+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub a répondu %s", resp.Status)
	}
	var gr ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&gr); err != nil {
		return nil, fmt.Errorf("réponse GitHub illisible : %w", err)
	}
	return &gr, nil
}

// Newer indique si la version a est strictement plus récente que b
// (« X.Y.Z », préfixe « v » et suffixe « -dev » ignorés). Une version
// illisible n'est jamais considérée comme plus récente.
func Newer(a, b string) bool {
	pa, okA := parseVersion(a)
	pb, okB := parseVersion(b)
	if !okA || !okB {
		return false
	}
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// GUIName et CLIName donnent le nom des exécutables livrés pour goos. Le
// suffixe -cli évite que les deux noms se confondent là où la casse est
// ignorée (Windows, clés FAT/exFAT) : jusqu'à la 1.1.0, « videogallery.exe »
// et « VideoGallery.exe » s'écrasaient l'un l'autre.
func GUIName(goos string) string { return "VideoGallery" + exeExt(goos) }

func CLIName(goos string) string { return "videogallery-cli" + exeExt(goos) }

// legacyCLIName est l'ancien nom du CLI (jusqu'à la 1.1.0).
func legacyCLIName(goos string) string { return "videogallery" + exeExt(goos) }

func exeExt(goos string) string {
	if goos == "windows" {
		return ".exe"
	}
	return ""
}

// managed liste les fichiers d'une archive que la mise à jour peut remplacer,
// avec leur mode. Seuls ceux déjà présents dans le dossier d'installation sont
// touchés : quelqu'un qui n'a copié que l'exécutable n'obtient que lui.
func managed(goos string) map[string]os.FileMode {
	return map[string]os.FileMode{
		GUIName(goos):              0o755,
		CLIName(goos):              0o755,
		"LICENSE":                  0o644,
		"LISEZMOI.txt":             0o644,
		"THIRD_PARTY_LICENSES.txt": 0o644,
		"install.sh":               0o755,
		"videogallery.desktop.in":  0o644,
		"videogallery.png":         0o644,
	}
}

// Apply télécharge la release, vérifie son empreinte puis remplace, dans dir,
// les fichiers qui y figurent déjà. Elle renvoie les noms remplacés. Rien n'est
// touché tant que le téléchargement n'est pas complet et vérifié.
func (c *Client) Apply(ctx context.Context, rel *Release, dir string, progress func(done, total int64)) ([]string, error) {
	if rel == nil || rel.archiveURL == "" {
		return nil, errors.New("aucune mise à jour à installer")
	}
	if err := checkWritable(dir); err != nil {
		return nil, fmt.Errorf("le dossier %s n'est pas modifiable (%v) : téléchargez la mise à jour sur %s", dir, err, rel.URL)
	}

	// Dossier de travail dans dir : les renommages restent sur le même système de fichiers.
	work, err := os.MkdirTemp(dir, ".videogallery-update-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)

	sums, err := c.fetchSums(ctx, rel.sumsURL)
	if err != nil {
		return nil, err
	}
	want, ok := sums[rel.archiveName]
	if !ok {
		return nil, fmt.Errorf("%s absent de SHA256SUMS", rel.archiveName)
	}

	archive := filepath.Join(work, "archive")
	got, err := c.download(ctx, rel.archiveURL, archive, progress)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(got, want) {
		return nil, fmt.Errorf("empreinte SHA-256 incorrecte pour %s (téléchargement corrompu ?)", rel.archiveName)
	}

	top := strings.TrimSuffix(strings.TrimSuffix(rel.archiveName, ".zip"), ".tar.gz")
	staged := filepath.Join(work, "new")
	if err := os.Mkdir(staged, 0o700); err != nil {
		return nil, err
	}
	allowed := managed(c.GOOS)
	if strings.HasSuffix(rel.archiveName, ".zip") {
		err = extractZip(archive, top, staged, allowed)
	} else {
		err = extractTarGz(archive, top, staged, allowed)
	}
	if err != nil {
		return nil, err
	}

	var done []string
	for name := range allowed {
		src := filepath.Join(staged, name)
		if _, err := os.Stat(src); err != nil {
			continue // absent de l'archive
		}
		dst := filepath.Join(dir, name)
		if _, err := os.Lstat(dst); err != nil {
			continue // pas installé ici
		}
		if err := replaceFile(src, dst); err != nil {
			return done, fmt.Errorf("remplacement de %s : %w", name, err)
		}
		done = append(done, name)
	}
	if name, err := migrateCLI(c.GOOS, staged, dir); err != nil {
		return done, err
	} else if name != "" {
		done = append(done, name)
	}
	if len(done) == 0 {
		return nil, errors.New("aucun fichier à mettre à jour dans " + dir)
	}
	return done, nil
}

// migrateCLI installe le CLI sous son nouveau nom là où l'ancien
// (legacyCLIName) est présent, puis retire l'ancien. Sous Windows, l'ancien nom
// désigne souvent le GUI lui-même (casse ignorée, le CLI avait été écrasé) : il
// est alors gardé, et le CLI disparu est réinstallé. Renvoie le nom installé.
func migrateCLI(goos, staged, dir string) (string, error) {
	cli := CLIName(goos)
	src := filepath.Join(staged, cli)
	legacy := filepath.Join(dir, legacyCLIName(goos))
	if _, err := os.Stat(src); err != nil {
		return "", nil // absent de l'archive
	}
	if _, err := os.Lstat(filepath.Join(dir, cli)); err == nil {
		return "", nil // déjà migré
	}
	legacyInfo, err := os.Lstat(legacy)
	if err != nil {
		return "", nil // pas de CLI installé ici
	}
	if err := os.Rename(src, filepath.Join(dir, cli)); err != nil {
		return "", fmt.Errorf("installation de %s : %w", cli, err)
	}
	if gui, err := os.Lstat(filepath.Join(dir, GUIName(goos))); err == nil && os.SameFile(gui, legacyInfo) {
		return cli, nil
	}
	// Sous Windows un exécutable en cours d'exécution ne peut pas être
	// supprimé, seulement renommé : CleanOld finira le travail.
	if err := os.Remove(legacy); err != nil {
		_ = os.Rename(legacy, legacy+".old")
	}
	return cli, nil
}

func checkWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".videogallery-write-")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

// replaceFile remplace dst par src. Sous Linux un renommage par-dessus un
// exécutable en cours d'utilisation est permis ; sous Windows il est refusé :
// on écarte alors l'ancien fichier en « .old » (autorisé même en cours
// d'exécution), que CleanOld supprime dès que le nouveau processus démarre.
func replaceFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	old := dst + ".old"
	_ = os.Remove(old)
	if err := os.Rename(dst, old); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err != nil {
		_ = os.Rename(old, dst) // retour arrière
		return err
	}
	return nil
}

// CleanOld supprime les « .old » laissés par une mise à jour précédente.
//
// Juste après une mise à jour, le nouveau processus démarre avant que l'ancien
// ne soit fermé : sous Windows, son exécutable (devenu « .old ») ne peut pas
// encore être supprimé. Ce qui résiste est donc retenté en arrière-plan
// pendant oldRetryFor, le temps que l'ancien processus se termine.
func CleanOld(dir string) {
	rest := removeOld(dir, nil)
	if len(rest) == 0 {
		return
	}
	deadline, every := time.Now().Add(oldRetryFor), oldRetryEvery
	go func() {
		for len(rest) > 0 && time.Now().Before(deadline) {
			time.Sleep(every)
			rest = removeOld(dir, rest)
		}
	}()
}

var (
	oldRetryFor   = time.Minute
	oldRetryEvery = 500 * time.Millisecond
)

// removeOld supprime les « .old » de names (tous ceux de la liste fermée si
// names est nil) et renvoie ceux qui existent encore.
func removeOld(dir string, names []string) []string {
	if names == nil {
		for name := range managed(runtime.GOOS) {
			names = append(names, name)
		}
		names = append(names, legacyCLIName(runtime.GOOS))
	}
	var rest []string
	for _, name := range names {
		err := os.Remove(filepath.Join(dir, name+".old"))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			rest = append(rest, name)
		}
	}
	return rest
}

func (c *Client) get(ctx context.Context, u string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "VideoGallery-updater")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("téléchargement refusé (%s)", resp.Status)
	}
	return resp, nil
}

func (c *Client) fetchSums(ctx context.Context, u string) (map[string]string, error) {
	resp, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	sums := map[string]string{}
	sc := bufio.NewScanner(io.LimitReader(resp.Body, 1<<20))
	for sc.Scan() {
		f := strings.Fields(sc.Text()) // « <sha256>  <fichier> » (format de sha256sum)
		if len(f) == 2 {
			sums[strings.TrimPrefix(f[1], "*")] = f[0]
		}
	}
	return sums, sc.Err()
}

// download écrit l'URL dans dest et renvoie son SHA-256 hexadécimal.
func (c *Client) download(ctx context.Context, u, dest string, progress func(done, total int64)) (string, error) {
	resp, err := c.get(ctx, u)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	src := io.LimitReader(resp.Body, maxArchiveSize+1)
	var n int64
	buf := make([]byte, 64<<10)
	for {
		k, rerr := src.Read(buf)
		if k > 0 {
			h.Write(buf[:k])
			if _, err := f.Write(buf[:k]); err != nil {
				return "", err
			}
			n += int64(k)
			if n > maxArchiveSize {
				return "", errors.New("archive trop volumineuse")
			}
			if progress != nil {
				progress(n, resp.ContentLength)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return "", rerr
		}
	}
	return hex.EncodeToString(h.Sum(nil)), f.Close()
}

// stagedName valide le chemin d'une entrée d'archive : exactement
// « <top>/<fichier autorisé> », rien d'autre (pas de « .. », pas de
// sous-dossier, pas de lien). Renvoie "" pour une entrée à ignorer.
func stagedName(entry, top string, allowed map[string]os.FileMode) string {
	entry = strings.TrimPrefix(path.Clean(strings.ReplaceAll(entry, "\\", "/")), "./")
	dir, name := path.Split(entry)
	if strings.TrimSuffix(dir, "/") != top {
		return ""
	}
	if _, ok := allowed[name]; !ok {
		return ""
	}
	return name
}

func writeStaged(dir, name string, mode os.FileMode, r io.Reader) error {
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(r, maxFileSize+1))
	if err == nil && n > maxFileSize {
		err = fmt.Errorf("%s trop volumineux", name)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

func extractTarGz(archive, top, dest string, allowed map[string]os.FileMode) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		if name := stagedName(h.Name, top, allowed); name != "" {
			if err := writeStaged(dest, name, allowed[name], tr); err != nil {
				return err
			}
		}
	}
}

func extractZip(archive, top, dest string, allowed map[string]os.FileMode) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, zf := range zr.File {
		if !zf.Mode().IsRegular() {
			continue
		}
		name := stagedName(zf.Name, top, allowed)
		if name == "" {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		err = writeStaged(dest, name, allowed[name], rc)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
