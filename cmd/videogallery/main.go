// Commande videogallery : génère une galerie HTML statique pour un
// répertoire de vidéos.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"videogallery/internal/gallery"
	"videogallery/internal/prefs"
	"videogallery/internal/update"
)

func main() { os.Exit(run()) }

func run() int {
	var opts gallery.Options
	fs := flag.NewFlagSet("videogallery", flag.ContinueOnError)
	fs.StringVar(&opts.Name, "name", "", "nom de la galerie (défaut : nom du répertoire)")
	fs.StringVar(&opts.OutDir, "out", "", "répertoire de sortie (défaut : <répertoire>/pages)")
	fs.IntVar(&opts.Cols, "cols", 4, "miniatures par ligne")
	fs.IntVar(&opts.Rows, "rows", 5, "miniatures par colonne")
	fs.StringVar(&opts.Background, "bg", "#baaf91", "couleur de fond (#rrggbb)")
	fs.StringVar(&opts.Foreground, "fg", "#333333", "couleur du texte (#rrggbb)")
	fs.IntVar(&opts.FrameWidth, "frame-width", 320, "largeur d'une capture en pixels")
	fs.IntVar(&opts.Jobs, "jobs", 0, "vidéos traitées en parallèle (défaut : nombre de CPU)")
	fs.StringVar(&opts.Order, "sort", gallery.OrderName, "ordre des vidéos : name, name-desc, date-desc (récentes d'abord), date,\n"+
		"duration-desc (longues d'abord), duration, size-desc (lourdes d'abord), size, none\n"+
		"(date = date de tournage si la vidéo l'indique, sinon date du fichier)")
	noSort := fs.Bool("no-sort", false, "équivaut à -sort none (conservé pour compatibilité)")
	fs.BoolVar(&opts.Force, "force", false, "régénérer les images même si elles sont à jour")
	showVersion := fs.Bool("version", false, "affiche la version")
	doUpdate := fs.Bool("update", false, "installe la dernière version publiée sur GitHub, puis quitte")
	noUpdateCheck := fs.Bool("no-update-check", false, "ne pas chercher de nouvelle version (vérification : au plus une fois par jour)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage : %s [options] <répertoire des vidéos> [options]\n\nOptions :\n", filepath.Base(os.Args[0]))
		fs.PrintDefaults()
	}

	// Accepte le répertoire avant ou après les options.
	var dir string
	args := os.Args[1:]
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			return 2
		}
		if fs.NArg() == 0 {
			break
		}
		if dir != "" {
			fmt.Fprintln(os.Stderr, "un seul répertoire est attendu")
			return 2
		}
		dir, args = fs.Arg(0), fs.Args()[1:]
	}

	if *showVersion {
		fmt.Println("The Video Gallery", gallery.Version)
		return 0
	}
	if self, err := update.SelfPath(); err == nil {
		update.CleanOld(filepath.Dir(self))
	}
	if *doUpdate {
		return selfUpdate()
	}
	if dir == "" {
		fs.Usage()
		return 2
	}
	opts.VideoDir = dir
	if *noSort {
		opts.Order = gallery.OrderNone
	}
	defer announceUpdate(*noUpdateCheck)()

	tools, err := gallery.FindTools()
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur:", err)
		fmt.Fprintln(os.Stderr, "ffmpeg est requis pour extraire les captures (Ubuntu : sudo apt install ffmpeg).")
		return 1
	}
	opts.Tools = tools
	opts.Log = func(format string, args ...any) { fmt.Printf(format+"\n", args...) }

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	res, err := gallery.Run(ctx, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur:", err)
		return 1
	}

	fmt.Printf("\n%d vidéo(s), %d page(s) d'index ; images : %d générée(s), %d déjà à jour\n",
		res.Videos, res.Pages, res.Generated, res.Cached)
	if len(res.Failed) > 0 {
		fmt.Printf("%d vidéo(s) ignorée(s) :\n", len(res.Failed))
		for _, f := range res.Failed {
			fmt.Printf("  %s : %v\n", f.Video, f.Err)
		}
	}
	fmt.Println("Galerie :", filepath.Join(res.OutDir, "index.html"))
	if len(res.Failed) > 0 {
		return 3
	}
	return 0
}

// selfUpdate installe la dernière release à la place de l'exécutable courant
// (et des fichiers livrés à côté), puis rafraîchit l'entrée de menu Linux.
func selfUpdate() int {
	self, err := update.SelfPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur:", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	c := update.NewClient()
	rel, err := c.Latest(ctx, gallery.Version)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur:", err)
		return 1
	}
	if rel == nil {
		fmt.Println("Déjà à jour (version", gallery.Version+").")
		return 0
	}
	fmt.Printf("Mise à jour %s -> %s dans %s\n", gallery.Version, rel.Version, filepath.Dir(self))
	done, err := c.Apply(ctx, rel, filepath.Dir(self), nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur:", err)
		return 1
	}
	fmt.Println("Fichiers remplacés :", done)
	if ok, err := update.RefreshShortcuts(filepath.Dir(self)); err != nil {
		fmt.Fprintln(os.Stderr, "attention :", err)
	} else if ok {
		fmt.Println("Entrée de menu mise à jour.")
	}
	fmt.Println("Version", rel.Version, "installée. Notes :", rel.URL)
	return 0
}

// announceUpdate lance en arrière-plan la vérification discrète d'une nouvelle
// version et renvoie la fonction qui affiche l'avis (sur stderr, et seulement
// dans un terminal : jamais dans un script ou un cron). Elle ne retarde la fin
// du programme que d'un instant.
func announceUpdate(disabled bool) func() {
	nop := func() {}
	if st, err := os.Stderr.Stat(); err != nil || st.Mode()&os.ModeCharDevice == 0 {
		return nop
	}
	if disabled {
		return nop
	}
	if path, err := prefs.Path(); err == nil && !prefs.Load(path).CheckUpdates {
		return nop
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return nop
	}
	cache := filepath.Join(dir, "VideoGallery", "update-check.json")

	result := make(chan string, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		result <- update.NewClient().CachedLatest(ctx, gallery.Version, cache)
	}()
	return func() {
		select {
		case v := <-result:
			if v != "" {
				fmt.Fprintf(os.Stderr, "\nUne nouvelle version est disponible : %s (vous avez %s). Pour l'installer : %s -update\n",
					v, gallery.Version, filepath.Base(os.Args[0]))
			}
		case <-time.After(1500 * time.Millisecond):
		}
	}
}
