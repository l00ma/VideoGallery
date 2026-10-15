// Package prefs gère les préférences de l'utilisateur (dernière galerie
// créée, couleurs, taille de grille...), stockées en JSON dans le répertoire
// de configuration de l'utilisateur.
package prefs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
)

// Prefs reprend les champs de l'ancienne fenêtre Perl.
type Prefs struct {
	VideoDir    string `json:"videoDir"`
	Name        string `json:"name"`
	Cols        int    `json:"cols"`
	Rows        int    `json:"rows"`
	FrameWidth  int    `json:"frameWidth"`
	Sort        bool   `json:"sort"`
	Background  string `json:"background"` // #rrggbb
	Foreground  string `json:"foreground"` // #rrggbb
	OpenBrowser bool   `json:"openBrowser"`
	// CheckUpdates : chercher une nouvelle version au démarrage (GUI) et au
	// plus une fois par jour (CLI).
	CheckUpdates bool `json:"checkUpdates"`
}

// Defaults : valeurs de l'ancienne version (fond 186,175,145 ; texte 51,51,51).
func Defaults() Prefs {
	return Prefs{
		Cols:         4,
		Rows:         5,
		FrameWidth:   320,
		Sort:         true,
		CheckUpdates: true,
		Background:   "#baaf91",
		Foreground:   "#333333",
	}
}

// Path renvoie l'emplacement du fichier de préférences.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "VideoGallery", "prefs.json"), nil
}

// Load lit les préférences. Un fichier absent ou illisible donne les valeurs
// par défaut : des préférences abîmées ne doivent jamais empêcher de démarrer.
// Les champs manquants gardent leur valeur par défaut.
func Load(path string) Prefs {
	p := Defaults()
	data, err := os.ReadFile(path)
	if err != nil {
		return p
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return Defaults()
	}
	return p.sanitized()
}

// sanitized remplace les valeurs hors bornes par celles par défaut.
func (p Prefs) sanitized() Prefs {
	d := Defaults()
	if p.Cols < 2 || p.Cols > 5 {
		p.Cols = d.Cols
	}
	if p.Rows < 2 || p.Rows > 6 {
		p.Rows = d.Rows
	}
	if p.FrameWidth < 80 || p.FrameWidth > 1920 {
		p.FrameWidth = d.FrameWidth
	}
	if !validColor(p.Background) {
		p.Background = d.Background
	}
	if !validColor(p.Foreground) {
		p.Foreground = d.Foreground
	}
	return p
}

// Save écrit les préférences via un fichier temporaire.
func Save(path string, p Prefs) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func validColor(c string) bool {
	if len(c) != 7 || c[0] != '#' {
		return false
	}
	_, err := strconv.ParseUint(c[1:], 16, 32)
	return err == nil
}
