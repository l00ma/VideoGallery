package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"videogallery/internal/gallery"
	"videogallery/internal/prefs"
	"videogallery/internal/update"
)

// App relie l'interface web au moteur de génération. Les méthodes exportées
// sont appelables depuis le JavaScript (window.go.main.App.*).
type App struct {
	ctx context.Context

	mu       sync.Mutex
	cancel   context.CancelFunc // non nil pendant une génération
	updating bool
	pending  *update.Release // dernière release proposée par CheckUpdate

	exe string // chemin réel de l'exécutable, relevé avant toute mise à jour
}

// Summary est le bilan d'une génération, envoyé à l'interface (événement « done »).
type Summary struct {
	Videos    int       `json:"videos"`
	Pages     int       `json:"pages"`
	Generated int       `json:"generated"`
	Cached    int       `json:"cached"`
	IndexPath string    `json:"indexPath"`
	Failed    []Failure `json:"failed"`
	Cancelled bool      `json:"cancelled"`
	Error     string    `json:"error"`
}

type Failure struct {
	Video string `json:"video"`
	Error string `json:"error"`
}

func NewApp() *App {
	exe, _ := update.SelfPath()
	if exe != "" {
		update.CleanOld(filepath.Dir(exe)) // restes d'une mise à jour précédente (Windows)
	}
	return &App{exe: exe}
}

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

// Version renvoie la version affichée dans la barre d'état.
func (a *App) Version() string { return gallery.Version }

// ToolsStatus renvoie une chaîne vide si ffmpeg est utilisable, sinon un
// message d'aide destiné à l'utilisateur.
func (a *App) ToolsStatus() string {
	if _, err := gallery.FindTools(); err != nil {
		return err.Error() + ". ffmpeg est requis (Ubuntu : sudo apt install ffmpeg ; Windows : placer ffmpeg.exe et ffprobe.exe à côté du programme)."
	}
	return ""
}

// LoadPrefs renvoie les préférences enregistrées (ou celles par défaut).
func (a *App) LoadPrefs() prefs.Prefs {
	path, err := prefs.Path()
	if err != nil {
		return prefs.Defaults()
	}
	return prefs.Load(path)
}

// DefaultPrefs sert au bouton « Effacer les valeurs ».
func (a *App) DefaultPrefs() prefs.Prefs { return prefs.Defaults() }

func (a *App) SavePrefs(p prefs.Prefs) error {
	path, err := prefs.Path()
	if err != nil {
		return err
	}
	return prefs.Save(path, p)
}

// ChooseDirectory ouvre le sélecteur de dossier natif. Renvoie "" si annulé.
func (a *App) ChooseDirectory(current string) (string, error) {
	opts := runtime.OpenDialogOptions{Title: "Répertoire des vidéos"}
	if st, err := os.Stat(current); err == nil && st.IsDir() {
		opts.DefaultDirectory = current
	}
	return runtime.OpenDirectoryDialog(a.ctx, opts)
}

// Generate lance la génération en arrière-plan et rend la main aussitôt.
// Progression : événements « log » (texte) et « progress » ({done,total}) ;
// fin : événement « done » (Summary).
func (a *App) Generate(p prefs.Prefs) error {
	a.mu.Lock()
	if a.cancel != nil {
		a.mu.Unlock()
		return errors.New("une génération est déjà en cours")
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancel = cancel
	a.mu.Unlock()

	tools, err := gallery.FindTools()
	if err != nil {
		a.finish()
		return err
	}

	opts := gallery.Options{
		VideoDir:   p.VideoDir,
		Name:       p.Name,
		Cols:       p.Cols,
		Rows:       p.Rows,
		FrameWidth: p.FrameWidth,
		NoSort:     !p.Sort,
		Background: p.Background,
		Foreground: p.Foreground,
		Tools:      tools,
		Log: func(format string, args ...any) {
			runtime.EventsEmit(a.ctx, "log", fmt.Sprintf(format, args...))
		},
		Progress: func(done, total int) {
			runtime.EventsEmit(a.ctx, "progress", map[string]int{"done": done, "total": total})
		},
	}

	go func() {
		defer a.finish()
		res, err := gallery.Run(ctx, opts)
		sum := Summary{}
		switch {
		case errors.Is(err, context.Canceled):
			sum.Cancelled = true
		case err != nil:
			sum.Error = err.Error()
		default:
			sum = Summary{
				Videos: res.Videos, Pages: res.Pages,
				Generated: res.Generated, Cached: res.Cached,
				IndexPath: filepath.Join(res.OutDir, "index.html"),
			}
			for _, f := range res.Failed {
				sum.Failed = append(sum.Failed, Failure{Video: f.Video, Error: f.Err.Error()})
			}
			// Mémorise les réglages d'une génération réussie.
			_ = a.SavePrefs(p)
		}
		runtime.EventsEmit(a.ctx, "done", sum)
	}()
	return nil
}

func (a *App) finish() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
}

// Cancel interrompt la génération en cours (sans effet sinon).
func (a *App) Cancel() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
	}
}

// OpenInBrowser ouvre une page de galerie dans le navigateur par défaut.
func (a *App) OpenInBrowser(indexPath string) error {
	if _, err := os.Stat(indexPath); err != nil {
		return err
	}
	return openFile(indexPath)
}

// Quit ferme l'application.
func (a *App) Quit() { runtime.Quit(a.ctx) }

// CheckUpdate cherche une release plus récente que la version courante.
// Renvoie nil si l'on est à jour. L'interface décide quand l'appeler (selon la
// préférence « Rechercher les mises à jour »).
func (a *App) CheckUpdate() (*update.Release, error) {
	rel, err := update.NewClient().Latest(a.ctx, gallery.Version)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.pending = rel
	a.mu.Unlock()
	return rel, nil
}

// ApplyUpdate installe la release trouvée par CheckUpdate (jamais une adresse
// fournie par l'interface), rafraîchit l'entrée de menu, puis relance
// l'application et ferme celle-ci. Progression : événement « updateProgress ».
func (a *App) ApplyUpdate() error {
	a.mu.Lock()
	switch {
	case a.cancel != nil:
		a.mu.Unlock()
		return errors.New("une génération est en cours : attendez la fin ou annulez-la")
	case a.updating:
		a.mu.Unlock()
		return errors.New("une mise à jour est déjà en cours")
	case a.pending == nil:
		a.mu.Unlock()
		return errors.New("aucune mise à jour à installer")
	case a.exe == "":
		a.mu.Unlock()
		return errors.New("emplacement du programme introuvable")
	}
	a.updating = true
	rel := a.pending
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.updating = false
		a.mu.Unlock()
	}()

	dir := filepath.Dir(a.exe)
	_, err := update.NewClient().Apply(a.ctx, rel, dir, func(done, total int64) {
		runtime.EventsEmit(a.ctx, "updateProgress", map[string]int64{"done": done, "total": total})
	})
	if err != nil {
		return err
	}
	if _, err := update.RefreshShortcuts(dir); err != nil {
		// La mise à jour elle-même a réussi : on le signale sans l'annuler.
		runtime.EventsEmit(a.ctx, "log", "Entrée de menu non rafraîchie : "+err.Error())
	}

	// Relance : le nouveau processus démarre avant que celui-ci ne se ferme.
	if err := exec.Command(a.exe).Start(); err != nil {
		return fmt.Errorf("mise à jour installée, mais le redémarrage a échoué (%v) : relancez le programme", err)
	}
	runtime.Quit(a.ctx)
	return nil
}
