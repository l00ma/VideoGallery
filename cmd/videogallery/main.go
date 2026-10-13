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

	"videogallery/internal/gallery"
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
	fs.BoolVar(&opts.NoSort, "no-sort", false, "ne pas trier les vidéos par nom")
	fs.BoolVar(&opts.Force, "force", false, "régénérer les images même si elles sont à jour")
	showVersion := fs.Bool("version", false, "affiche la version")
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
	if dir == "" {
		fs.Usage()
		return 2
	}
	opts.VideoDir = dir

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
