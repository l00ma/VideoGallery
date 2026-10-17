package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"2.0.1", "2.0.0", true},
		{"v2.1.0", "2.0.9", true},
		{"2.0.0", "2.0.0", false},
		{"2.0.0", "2.0.1", false},
		{"10.0.0", "9.9.9", true},
		{"2.0.0", "2.0.0-dev", false}, // le suffixe est ignoré
		{"abc", "2.0.0", false},
		{"2.0", "1.0.0", false},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, attendu %v", c.a, c.b, got, c.want)
		}
	}
}

// fakeRelease construit une archive (tar.gz ou zip) et un serveur GitHub factice.
func fakeRelease(t *testing.T, goos string, files map[string]string, corruptSum bool) (*Client, *httptest.Server) {
	t.Helper()
	top := "VideoGallery-9.9.9-" + goos + "-amd64"
	var archive bytes.Buffer
	name := top + ".tar.gz"
	if goos == "windows" {
		name = top + ".zip"
		zw := zip.NewWriter(&archive)
		for n, c := range files {
			w, _ := zw.Create(n)
			w.Write([]byte(c))
		}
		zw.Close()
	} else {
		gz := gzip.NewWriter(&archive)
		tw := tar.NewWriter(gz)
		for n, c := range files {
			tw.WriteHeader(&tar.Header{Name: n, Mode: 0o644, Size: int64(len(c)), Typeflag: tar.TypeReg})
			tw.Write([]byte(c))
		}
		tw.Close()
		gz.Close()
	}
	sum := sha256.Sum256(archive.Bytes())
	hexsum := hex.EncodeToString(sum[:])
	if corruptSum {
		hexsum = strings.Repeat("0", 64)
	}

	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/repos/"+Repo+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":"v9.9.9","name":"neuf","body":"notes","html_url":"https://example/r",
		 "assets":[{"name":%q,"browser_download_url":%q},{"name":"SHA256SUMS","browser_download_url":%q}]}`,
			name, srv.URL+"/dl/"+name, srv.URL+"/dl/SHA256SUMS")
	})
	mux.HandleFunc("/dl/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", hexsum, name)
	})
	mux.HandleFunc("/dl/"+name, func(w http.ResponseWriter, r *http.Request) { w.Write(archive.Bytes()) })
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := NewClient()
	c.APIURL, c.GOOS, c.GOARCH = srv.URL, goos, "amd64"
	return c, srv
}

func TestLatestNothingNewer(t *testing.T) {
	c, _ := fakeRelease(t, "linux", nil, false)
	rel, err := c.Latest(context.Background(), "9.9.9")
	if err != nil || rel != nil {
		t.Fatalf("Latest = %v, %v ; attendu nil, nil", rel, err)
	}
}

func TestApplyLinux(t *testing.T) {
	top := "VideoGallery-9.9.9-linux-amd64/"
	c, _ := fakeRelease(t, "linux", map[string]string{
		top + "VideoGallery":     "gui-new",
		top + "videogallery-cli": "cli-new",
		top + "LISEZMOI.txt":     "readme-new",
		top + "../evil":          "x",
		top + "sub/install.sh":   "nested",
		"autre/VideoGallery":     "wrong-dir",
	}, false)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "VideoGallery"), []byte("gui-old"), 0o755)
	os.WriteFile(filepath.Join(dir, "videogallery-cli"), []byte("cli-old"), 0o755)
	// LISEZMOI.txt volontairement absent : il ne doit pas être créé.

	rel, err := c.Latest(context.Background(), "2.0.1")
	if err != nil || rel == nil || rel.Version != "9.9.9" {
		t.Fatalf("Latest = %+v, %v", rel, err)
	}
	done, err := c.Apply(context.Background(), rel, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(done) != 2 {
		t.Errorf("remplacés : %v, attendu 2 fichiers", done)
	}
	for f, want := range map[string]string{"VideoGallery": "gui-new", "videogallery-cli": "cli-new"} {
		got, _ := os.ReadFile(filepath.Join(dir, f))
		if string(got) != want {
			t.Errorf("%s = %q, attendu %q", f, got, want)
		}
		if st, _ := os.Stat(filepath.Join(dir, f)); st.Mode()&0o111 == 0 {
			t.Errorf("%s n'est plus exécutable", f)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Errorf("le dossier contient des fichiers en trop : %v", entries)
	}
}

func TestApplyWindowsZip(t *testing.T) {
	top := "VideoGallery-9.9.9-windows-amd64/"
	c, _ := fakeRelease(t, "windows", map[string]string{top + "VideoGallery.exe": "new"}, false)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "VideoGallery.exe"), []byte("old"), 0o755)
	rel, err := c.Latest(context.Background(), "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Apply(context.Background(), rel, dir, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "VideoGallery.exe")); string(got) != "new" {
		t.Errorf("exe = %q", got)
	}
}

func TestApplyRejectsBadChecksum(t *testing.T) {
	top := "VideoGallery-9.9.9-linux-amd64/"
	c, _ := fakeRelease(t, "linux", map[string]string{top + "VideoGallery": "new"}, true)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "VideoGallery"), []byte("old"), 0o755)
	rel, _ := c.Latest(context.Background(), "1.0.0")
	if _, err := c.Apply(context.Background(), rel, dir, nil); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("erreur = %v, attendu un rejet d'empreinte", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "VideoGallery")); string(got) != "old" {
		t.Errorf("le fichier a été modifié malgré une empreinte incorrecte : %q", got)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("fichiers temporaires restants : %v", entries)
	}
}

func TestCachedLatest(t *testing.T) {
	c, srv := fakeRelease(t, "linux", nil, false)
	cache := filepath.Join(t.TempDir(), "sub", "c.json")
	if got := c.CachedLatest(context.Background(), "1.0.0", cache); got != "9.9.9" {
		t.Fatalf("1er appel = %q", got)
	}
	srv.Close() // les appels suivants doivent venir du cache
	if got := c.CachedLatest(context.Background(), "1.0.0", cache); got != "9.9.9" {
		t.Errorf("appel en cache = %q", got)
	}
	if got := c.CachedLatest(context.Background(), "9.9.9", cache); got != "" {
		t.Errorf("version à jour = %q, attendu vide", got)
	}
}

func TestRefreshShortcuts(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil || runtime.GOOS != "linux" {
		t.Skip("bash/linux requis")
	}
	install, err := os.ReadFile("../../packaging/linux/install.sh")
	if err != nil {
		t.Fatal(err)
	}
	tpl, _ := os.ReadFile("../../packaging/linux/videogallery.desktop.in")
	dir := filepath.Join(t.TempDir(), "mon dossier") // espace : cas réel
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "install.sh"), install, 0o755)
	os.WriteFile(filepath.Join(dir, "videogallery.desktop.in"), tpl, 0o644)
	os.WriteFile(filepath.Join(dir, "videogallery.png"), []byte("png"), 0o644)
	os.WriteFile(filepath.Join(dir, "VideoGallery"), []byte("x"), 0o755)

	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	desktop := filepath.Join(data, "applications", "videogallery.desktop")

	// Aucune entrée installée : rien à faire.
	if ok, err := RefreshShortcuts(dir); ok || err != nil {
		t.Fatalf("sans entrée : %v, %v", ok, err)
	}
	// Entrée installée pour ce dossier, puis modèle mis à jour par la release.
	if out, err := exec.Command("bash", filepath.Join(dir, "install.sh")).CombinedOutput(); err != nil {
		t.Fatalf("install.sh : %v %s", err, out)
	}
	os.WriteFile(filepath.Join(dir, "videogallery.desktop.in"), append(tpl, []byte("Keywords=nouveau\n")...), 0o644)
	if ok, err := RefreshShortcuts(dir); !ok || err != nil {
		t.Fatalf("rafraîchissement : %v, %v", ok, err)
	}
	if got, _ := os.ReadFile(desktop); !strings.Contains(string(got), "Keywords=nouveau") {
		t.Errorf("le .desktop n'a pas été régénéré :\n%s", got)
	}
	// Entrée qui pointe ailleurs : on n'y touche pas.
	os.WriteFile(desktop, []byte("[Desktop Entry]\nExec=\"/autre/VideoGallery\"\n"), 0o644)
	if ok, _ := RefreshShortcuts(dir); ok {
		t.Error("une entrée d'un autre dossier ne doit pas être modifiée")
	}
}

// Un « .old » encore verrouillé (ici simulé par un répertoire non vide, que
// os.Remove refuse) est supprimé dès qu'il se libère.
func TestCleanOldRetries(t *testing.T) {
	oldFor, oldEvery := oldRetryFor, oldRetryEvery
	oldRetryFor, oldRetryEvery = 5*time.Second, 10*time.Millisecond
	defer func() { oldRetryFor, oldRetryEvery = oldFor, oldEvery }()

	dir := t.TempDir()
	exe := "videogallery-cli"
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	free := filepath.Join(dir, "LICENSE.old")
	locked := filepath.Join(dir, exe+".old")
	if err := os.WriteFile(free, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(locked, "verrou"), 0o755); err != nil {
		t.Fatal(err)
	}

	CleanOld(dir)
	if _, err := os.Stat(free); !os.IsNotExist(err) {
		t.Fatalf("LICENSE.old devrait être supprimé tout de suite : %v", err)
	}
	if _, err := os.Stat(locked); err != nil {
		t.Fatalf("le .old verrouillé ne peut pas déjà avoir disparu : %v", err)
	}

	os.Remove(filepath.Join(locked, "verrou")) // l'ancien processus se ferme
	for start := time.Now(); time.Since(start) < 3*time.Second; time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(locked); os.IsNotExist(err) {
			return
		}
	}
	t.Fatal("le .old n'a pas été supprimé une fois libéré")
}

// Windows ignore la casse : deux fichiers gérés ne doivent pas s'y confondre
// (VideoGallery.exe écrasait videogallery.exe jusqu'à la 1.1.0), ni sous Linux
// sur une clé FAT/exFAT.
func TestManagedNamesDistinctIgnoringCase(t *testing.T) {
	for _, goos := range []string{"linux", "windows"} {
		seen := map[string]string{}
		for name := range managed(goos) {
			low := strings.ToLower(name)
			if prev, ok := seen[low]; ok {
				t.Errorf("%s : %q et %q se confondent", goos, prev, name)
			}
			seen[low] = name
		}
	}
}

// Une installation 1.1.0 (CLI nommé « videogallery ») reçoit le CLI sous son
// nouveau nom et perd l'ancien.
func TestApplyMigratesLegacyCLI(t *testing.T) {
	top := "VideoGallery-9.9.9-linux-amd64/"
	c, _ := fakeRelease(t, "linux", map[string]string{
		top + "VideoGallery":     "gui-new",
		top + "videogallery-cli": "cli-new",
	}, false)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "VideoGallery"), []byte("gui-old"), 0o755)
	os.WriteFile(filepath.Join(dir, "videogallery"), []byte("cli-old"), 0o755)

	rel, err := c.Latest(context.Background(), "1.1.0")
	if err != nil || rel == nil {
		t.Fatalf("Latest = %+v, %v", rel, err)
	}
	if _, err := c.Apply(context.Background(), rel, dir, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "videogallery-cli")); string(got) != "cli-new" {
		t.Errorf("videogallery-cli = %q, attendu cli-new", got)
	}
	if _, err := os.Lstat(filepath.Join(dir, "videogallery")); !os.IsNotExist(err) {
		t.Errorf("l'ancien CLI devrait avoir disparu : %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "VideoGallery")); string(got) != "gui-new" {
		t.Errorf("VideoGallery = %q, attendu gui-new", got)
	}
}

// Ancien nom confondu avec le GUI (Windows, casse ignorée ; simulé par un lien
// physique) : le GUI est gardé et le CLI réinstallé.
func TestMigrateCLIKeepsGUI(t *testing.T) {
	staged, dir := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(staged, "videogallery-cli.exe"), []byte("cli-new"), 0o755)
	gui := filepath.Join(dir, "VideoGallery.exe")
	os.WriteFile(gui, []byte("gui"), 0o755)
	if err := os.Link(gui, filepath.Join(dir, "videogallery.exe")); err != nil {
		t.Skip("liens physiques indisponibles :", err)
	}
	name, err := migrateCLI("windows", staged, dir)
	if err != nil || name != "videogallery-cli.exe" {
		t.Fatalf("migrateCLI = %q, %v", name, err)
	}
	if got, _ := os.ReadFile(gui); string(got) != "gui" {
		t.Errorf("le GUI a été touché : %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "videogallery.exe")); err != nil {
		t.Errorf("l'ancien nom (le GUI) ne devait pas être supprimé : %v", err)
	}
}
