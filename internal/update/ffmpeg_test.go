package update

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// fakeFFmpeg imite la release « latest » de BtbN : plusieurs variantes et
// branches, dont une seule doit être choisie (lgpl-shared de la branche
// stable la plus récente, 9.0 ici), et une archive piégée.
func fakeFFmpeg(t *testing.T, files map[string]string, corruptSum bool) (*Client, *int) {
	t.Helper()
	const name = "ffmpeg-n9.0-latest-win64-lgpl-shared-9.0.zip"
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	for n, c := range files {
		w, _ := zw.Create(n)
		w.Write([]byte(c))
	}
	zw.Close()
	sum := sha256.Sum256(archive.Bytes())
	hexsum := hex.EncodeToString(sum[:])
	if corruptSum {
		hexsum = strings.Repeat("0", 64)
	}

	downloads := 0
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/repos/"+FFmpegRepo+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		var assets []string
		for _, a := range []string{
			"checksums.sha256",
			"ffmpeg-master-latest-win64-lgpl-shared.zip",
			"ffmpeg-n8.1-latest-win64-lgpl-shared-8.1.zip",
			name,
			"ffmpeg-n9.0-latest-win64-gpl-shared-9.0.zip",
			"ffmpeg-n9.0-latest-win64-lgpl-9.0.zip",
			"ffmpeg-n9.0-latest-linux64-lgpl-shared-9.0.tar.xz",
		} {
			assets = append(assets, fmt.Sprintf(`{"name":%q,"browser_download_url":%q,"size":%d}`, a, srv.URL+"/dl/"+a, archive.Len()))
		}
		fmt.Fprintf(w, `{"tag_name":"latest","assets":[%s]}`, strings.Join(assets, ","))
	})
	mux.HandleFunc("/dl/checksums.sha256", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  ffmpeg-n8.1-latest-win64-lgpl-shared-8.1.zip\n%s  %s\n", strings.Repeat("1", 64), hexsum, name)
	})
	mux.HandleFunc("/dl/"+name, func(w http.ResponseWriter, r *http.Request) {
		downloads++
		w.Write(archive.Bytes())
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := NewClient()
	c.APIURL, c.GOOS, c.GOARCH = srv.URL, "windows", "amd64"
	return c, &downloads
}

func listDir(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

func TestInstallFFmpeg(t *testing.T) {
	top := "ffmpeg-n9.0-latest-win64-lgpl-shared-9.0/"
	c, _ := fakeFFmpeg(t, map[string]string{
		top + "LICENSE.txt":            "LGPL",
		top + "bin/ffmpeg.exe":         "ffmpeg",
		top + "bin/ffprobe.exe":        "ffprobe",
		top + "bin/ffplay.exe":         "inutile",
		top + "bin/avcodec-63.dll":     "codec",
		top + "bin/avformat-63.dll":    "format",
		top + "bin/sous/piege.dll":     "non",
		top + "doc/ffmpeg.html":        "non",
		top + "bin/../../../evil.dll":  "non",
		"autre-dossier/bin/ffmpeg.exe": "non",
	}, false)

	b, err := c.LatestFFmpeg(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if b.Version != "9.0" || !strings.Contains(b.Name, "lgpl-shared-9.0") {
		t.Fatalf("archive choisie : %+v", b)
	}

	app := t.TempDir()
	// une installation précédente, dont une DLL qui n'existe plus en 9.0
	old := filepath.Join(app, FFmpegDir)
	os.MkdirAll(old, 0o755)
	os.WriteFile(filepath.Join(old, "avcodec-62.dll"), []byte("ancienne"), 0o644)

	var last int64
	if err := c.InstallFFmpeg(context.Background(), b, app, func(done, total int64) { last = done }); err != nil {
		t.Fatal(err)
	}
	if last == 0 {
		t.Error("progression jamais signalée")
	}
	if got, want := listDir(t, filepath.Join(app, FFmpegDir)), "LICENSE.txt,avcodec-63.dll,avformat-63.dll,ffmpeg.exe,ffprobe.exe"; got != want {
		t.Errorf("fichiers installés : %s, attendu %s", got, want)
	}
	if data, _ := os.ReadFile(filepath.Join(app, FFmpegDir, "ffmpeg.exe")); string(data) != "ffmpeg" {
		t.Errorf("ffmpeg.exe = %q", data)
	}
	// rien d'autre dans le dossier du programme (ni dossier de travail, ni evil.dll)
	if got := listDir(t, app); got != FFmpegDir {
		t.Errorf("dossier du programme : %s", got)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(app), "evil.dll")); err == nil {
		t.Error("traversée de chemin : evil.dll écrit hors du dossier")
	}
}

func TestInstallFFmpegRejectsBadChecksum(t *testing.T) {
	c, _ := fakeFFmpeg(t, map[string]string{"ffmpeg-n9.0-latest-win64-lgpl-shared-9.0/bin/ffmpeg.exe": "x"}, true)
	b, err := c.LatestFFmpeg(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	app := t.TempDir()
	err = c.InstallFFmpeg(context.Background(), b, app, nil)
	if err == nil || !strings.Contains(err.Error(), "empreinte") {
		t.Fatalf("empreinte fausse acceptée : %v", err)
	}
	if got := listDir(t, app); got != "" {
		t.Errorf("le dossier du programme a été modifié : %s", got)
	}
}

func TestInstallFFmpegRequiresBothTools(t *testing.T) {
	c, _ := fakeFFmpeg(t, map[string]string{"ffmpeg-n9.0-latest-win64-lgpl-shared-9.0/bin/ffmpeg.exe": "x"}, false)
	b, _ := c.LatestFFmpeg(context.Background())
	app := t.TempDir()
	if err := c.InstallFFmpeg(context.Background(), b, app, nil); err == nil || !strings.Contains(err.Error(), "ffprobe.exe") {
		t.Fatalf("archive sans ffprobe acceptée : %v", err)
	}
	if got := listDir(t, app); got != "" {
		t.Errorf("le dossier du programme a été modifié : %s", got)
	}
}

func TestFFmpegOnlyOnWindows(t *testing.T) {
	c, downloads := fakeFFmpeg(t, nil, false)
	c.GOOS = "linux"
	if c.FFmpegSupported() {
		t.Error("proposé sous Linux")
	}
	if _, err := c.LatestFFmpeg(context.Background()); err == nil {
		t.Error("LatestFFmpeg accepté sous Linux")
	}
	if *downloads != 0 {
		t.Error("téléchargement lancé")
	}
}
