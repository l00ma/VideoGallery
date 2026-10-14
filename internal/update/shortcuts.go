package update

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// RefreshShortcuts remet à jour l'entrée de menu Linux (.desktop + icône) si
// elle a été installée par install.sh pour ce dossier. Elle relance le
// install.sh qui vient d'être livré avec la mise à jour : le modèle .desktop et
// l'icône de la nouvelle version sont ainsi pris en compte. Sans entrée
// installée pour ce dossier (autre emplacement, entrée système), rien n'est
// touché et refreshed vaut false.
func RefreshShortcuts(dir string) (refreshed bool, err error) {
	if runtime.GOOS != "linux" {
		return false, nil // pas d'entrée de menu installée par nos soins ailleurs
	}
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		home, herr := os.UserHomeDir()
		if herr != nil {
			return false, nil
		}
		data = filepath.Join(home, ".local", "share")
	}
	desktop, err := os.ReadFile(filepath.Join(data, "applications", "videogallery.desktop"))
	if err != nil {
		return false, nil
	}
	// Même formulation que install.sh : chemin entre guillemets, « % » doublé.
	exec1 := `Exec="` + strings.ReplaceAll(filepath.Join(dir, "VideoGallery"), "%", "%%") + `"`
	if !strings.Contains(string(desktop), "\n"+exec1+"\n") {
		return false, nil
	}
	script := filepath.Join(dir, "install.sh")
	if _, err := os.Stat(script); err != nil {
		return false, fmt.Errorf("install.sh introuvable dans %s : relancez-le à la main pour rafraîchir le menu", dir)
	}
	if out, err := exec.Command("bash", script).CombinedOutput(); err != nil {
		return false, fmt.Errorf("install.sh : %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return true, nil
}

const checkInterval = 24 * time.Hour

type cacheFile struct {
	Checked time.Time `json:"checked"`
	Latest  string    `json:"latest"` // dernière version connue, "" si inconnue
}

// CachedLatest sert la vérification discrète de la ligne de commande : elle
// n'interroge GitHub qu'une fois par jour, le résultat étant gardé dans
// cachePath. Renvoie la version plus récente que current, ou "".
func (c *Client) CachedLatest(ctx context.Context, current, cachePath string) string {
	var cf cacheFile
	if data, err := os.ReadFile(cachePath); err == nil {
		_ = json.Unmarshal(data, &cf)
	}
	if time.Since(cf.Checked) >= checkInterval || cf.Checked.After(time.Now().Add(time.Hour)) {
		rel, err := c.Latest(ctx, current)
		if err == nil {
			cf.Latest = ""
			if rel != nil {
				cf.Latest = rel.Version
			}
		}
		// Même en cas d'échec (hors ligne) : on ne retente pas à chaque lancement.
		cf.Checked = time.Now()
		if data, err := json.Marshal(cf); err == nil {
			_ = os.MkdirAll(filepath.Dir(cachePath), 0o755)
			_ = os.WriteFile(cachePath, data, 0o644)
		}
	}
	if Newer(cf.Latest, current) {
		return cf.Latest
	}
	return ""
}

// SelfPath renvoie le chemin réel de l'exécutable en cours (liens symboliques
// résolus). À appeler au démarrage : après une mise à jour, Linux signale
// l'ancien fichier remplacé comme « (deleted) ».
func SelfPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}
