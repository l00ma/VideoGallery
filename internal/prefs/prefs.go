// Package prefs gère les préférences de l'utilisateur (dernière galerie
// créée, couleurs, taille de grille...), stockées en JSON dans le répertoire
// de configuration de l'utilisateur.
package prefs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
}

// Defaults : valeurs de l'ancienne version (fond 186,175,145 ; texte 51,51,51).
func Defaults() Prefs {
	return Prefs{
		Cols:       4,
		Rows:       5,
		FrameWidth: 320,
		Sort:       true,
		Background: "#baaf91",
		Foreground: "#333333",
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

// ImportLegacy lit un fichier « mes_valeurs.sav » de la version Perl :
//
//	dir|nom|ligne|colonne|tri|largeur|hauteur|proportions|r,g,b|r,g,b|navigateur
func ImportLegacy(path string) (Prefs, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Prefs{}, err
	}
	f := strings.Split(strings.TrimSpace(string(data)), "|")
	if len(f) < 11 {
		return Prefs{}, fmt.Errorf("format inattendu : %d champs au lieu de 11", len(f))
	}
	p := Defaults()
	p.VideoDir, p.Name = f[0], f[1]
	if n, err := strconv.Atoi(f[2]); err == nil {
		p.Cols = n
	}
	if n, err := strconv.Atoi(f[3]); err == nil {
		p.Rows = n
	}
	p.Sort = f[4] == "1"
	if n, err := strconv.Atoi(f[5]); err == nil {
		p.FrameWidth = n
	}
	if c, err := rgbToHex(f[8]); err == nil {
		p.Background = c
	}
	if c, err := rgbToHex(f[9]); err == nil {
		p.Foreground = c
	}
	p.OpenBrowser = f[10] == "1"
	return p.sanitized(), nil
}

func rgbToHex(s string) (string, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 3 {
		return "", errors.New("couleur attendue au format r,g,b")
	}
	var v [3]int
	for i, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 0 || n > 255 {
			return "", errors.New("composante de couleur invalide")
		}
		v[i] = n
	}
	return fmt.Sprintf("#%02x%02x%02x", v[0], v[1], v[2]), nil
}
