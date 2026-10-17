// Commande videogallery : génère une galerie HTML statique pour un
// répertoire de vidéos.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"videogallery/internal/gallery"
	"videogallery/internal/opener"
	"videogallery/internal/prefs"
	"videogallery/internal/update"
)

func main() { os.Exit(run()) }

// cli regroupe les valeurs des options de la ligne de commande.
type cli struct {
	opts          gallery.Options
	noSort        bool
	open          bool
	showVersion   bool
	doUpdate      bool
	noUpdateCheck bool
}

// newFlagSet déclare les options. Les textes courts servent aux messages
// d'erreur ; l'aide complète (-h) est dans help.go.
func newFlagSet(c *cli) *flag.FlagSet {
	fs := flag.NewFlagSet("videogallery", flag.ContinueOnError)
	fs.StringVar(&c.opts.Name, "name", "", "titre de la galerie")
	fs.StringVar(&c.opts.OutDir, "out", "", "dossier de sortie")
	fs.IntVar(&c.opts.Cols, "cols", 4, "miniatures par ligne")
	fs.IntVar(&c.opts.Rows, "rows", 5, "lignes de miniatures par page")
	fs.StringVar(&c.opts.Background, "bg", "#baaf91", "couleur de fond (#rrggbb)")
	fs.StringVar(&c.opts.Foreground, "fg", "#333333", "couleur du texte (#rrggbb)")
	fs.IntVar(&c.opts.FrameWidth, "frame-width", 320, "largeur d'une capture en pixels")
	fs.IntVar(&c.opts.Jobs, "jobs", 0, "vidéos traitées en parallèle (0 : nombre de processeurs)")
	fs.StringVar(&c.opts.Order, "sort", gallery.OrderName, "ordre des vidéos")
	fs.BoolVar(&c.noSort, "no-sort", false, "équivaut à -sort none")
	fs.BoolVar(&c.open, "open", false, "ouvrir la galerie dans le navigateur à la fin")
	fs.BoolVar(&c.opts.Force, "force", false, "régénérer toutes les images")
	fs.BoolVar(&c.showVersion, "version", false, "afficher la version")
	fs.BoolVar(&c.doUpdate, "update", false, "installer la dernière version, puis quitter")
	fs.BoolVar(&c.noUpdateCheck, "no-update-check", false, "ne pas chercher de nouvelle version")
	// Les messages de flag (en anglais) sont tus : run affiche l'erreur en
	// français suivie d'un rappel court, et l'aide complète sur --help.
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	return fs
}

// usageHint rappelle la syntaxe après une erreur d'utilisation.
func usageHint() {
	fmt.Fprintf(os.Stderr, "Usage : %s [--option valeur]... <dossier des vidéos>\n"+
		"Aide complète : %[1]s --help\n", filepath.Base(os.Args[0]))
}

// flagError traduit les erreurs du paquet flag, en nommant les options sous
// la forme --nom (seules les formes connues ; une autre reste en anglais
// plutôt que d'être mal traduite).
func flagError(err error) string {
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "flag provided but not defined: -"):
		return "option inconnue : --" + strings.TrimPrefix(msg, "flag provided but not defined: -")
	case strings.HasPrefix(msg, "flag needs an argument: -"):
		return "cette option attend une valeur : --" + strings.TrimPrefix(msg, "flag needs an argument: -")
	case strings.HasPrefix(msg, "invalid boolean value "):
		// invalid boolean value "x" for -open: parse error
		msg = strings.TrimPrefix(msg, "invalid boolean value ")
		msg = strings.Replace(msg, " for -", " pour l'option --", 1)
		return "valeur invalide " + strings.Replace(msg, ": parse error", " : cette option s'écrit seule, sans valeur", 1)
	case strings.HasPrefix(msg, "invalid value "):
		// invalid value "x" for flag -cols: parse error
		msg = strings.TrimPrefix(msg, "invalid value ")
		msg = strings.Replace(msg, " for flag -", " pour l'option --", 1)
		msg = strings.Replace(msg, ": value out of range", " : nombre trop grand", 1)
		return "valeur invalide " + strings.Replace(msg, ": parse error", " : nombre entier attendu", 1)
	}
	return msg
}

// legacySyntax repère les écritures d'options qui seront refusées en 2.0.0
// (un seul tiret, valeur collée par =) et propose pour chacune la forme
// --option valeur. -h reste admis (convention universelle).
func legacySyntax(fs *flag.FlagSet, args []string) [][2]string {
	var out [][2]string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break // la suite n'est pas faite d'options
		}
		if len(a) < 2 || a[0] != '-' {
			continue // dossier des vidéos
		}
		name, value, hasEq := strings.Cut(strings.TrimLeft(a, "-"), "=")
		f := fs.Lookup(name)
		if f == nil {
			continue // option inconnue (ou -h) : traitée par fs.Parse
		}
		isBool := false
		if b, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && b.IsBoolFlag() {
			isBool = true
		}
		written := a
		if !hasEq && !isBool && i+1 < len(args) {
			i++
			value = args[i]
			written += " " + args[i]
		}
		if !strings.HasPrefix(a, "--") || hasEq {
			good := "--" + name
			switch {
			case isBool && hasEq && value == "false":
				good = "(retirez l'option : elle est inactive par défaut)"
			case !isBool:
				good += " " + shellQuote(value)
			}
			out = append(out, [2]string{written, good})
		}
	}
	return out
}

// shellQuote met entre guillemets doubles une valeur que le terminal
// couperait (espace, #...) : ils marchent sous Linux comme sous Windows.
func shellQuote(v string) string {
	if v != "" && !strings.ContainsAny(v, " \t#'\"$`&|;<>()*?!~") {
		return v
	}
	return `"` + v + `"`
}

func run() int {
	var c cli
	fs := newFlagSet(&c)
	opts := &c.opts

	// Accepte le répertoire avant ou après les options.
	var dir string
	args := os.Args[1:]
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				fmt.Print(helpText(fs, filepath.Base(os.Args[0])))
				return 0
			}
			fmt.Fprintln(os.Stderr, "erreur:", flagError(err))
			usageHint()
			return 2
		}
		if fs.NArg() == 0 {
			break
		}
		if dir != "" {
			fmt.Fprintln(os.Stderr, "erreur: un seul dossier de vidéos est attendu")
			usageHint()
			return 2
		}
		dir, args = fs.Arg(0), fs.Args()[1:]
	}

	if legacy := legacySyntax(fs, os.Args[1:]); len(legacy) > 0 {
		fmt.Fprintln(os.Stderr, "attention : écriture d'option ancienne, refusée à partir de la version 2.0.0.")
		fmt.Fprintln(os.Stderr, "Écrivez à la place :")
		width := 0
		for _, l := range legacy {
			width = max(width, utf8.RuneCountInString(l[0]))
		}
		for _, l := range legacy {
			pad := strings.Repeat(" ", width-utf8.RuneCountInString(l[0]))
			fmt.Fprintf(os.Stderr, "  %s%s  →  %s\n", l[0], pad, l[1])
		}
		fmt.Fprintln(os.Stderr)
	}
	if c.showVersion {
		fmt.Println("The Video Gallery", gallery.Version)
		return 0
	}
	if self, err := update.SelfPath(); err == nil {
		update.CleanOld(filepath.Dir(self))
	}
	if c.doUpdate {
		return selfUpdate()
	}
	if dir == "" {
		fmt.Fprintln(os.Stderr, "erreur: dossier des vidéos non précisé")
		usageHint()
		return 2
	}
	opts.VideoDir = dir
	if c.noSort {
		opts.Order = gallery.OrderNone
	}
	defer announceUpdate(c.noUpdateCheck)()

	tools, err := gallery.FindTools()
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur:", err)
		if update.NewClient().FFmpegSupported() {
			fmt.Fprintln(os.Stderr, "ffmpeg est requis pour extraire les captures. Pour l'installer : lancez VideoGallery.exe\n"+
				"et cliquez sur « Télécharger ffmpeg », ou placez ffmpeg.exe et ffprobe.exe à côté du programme.")
		} else {
			fmt.Fprintln(os.Stderr, "ffmpeg est requis pour extraire les captures (Ubuntu : sudo apt install ffmpeg).")
		}
		return 1
	}
	opts.Tools = tools
	opts.Log = func(format string, args ...any) { fmt.Printf(format+"\n", args...) }

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	res, err := gallery.Run(ctx, *opts)
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
	index := filepath.Join(res.OutDir, "index.html")
	fmt.Println("Galerie :", index)
	if c.open {
		if err := opener.Open(index); err != nil {
			// la galerie est faite : on le signale sans changer le code retour
			fmt.Fprintln(os.Stderr, "ouverture dans le navigateur impossible :", err)
		}
	}
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
	if cli := update.CLIName(runtime.GOOS); filepath.Base(self) != cli && slices.Contains(done, cli) {
		fmt.Printf("La ligne de commande s'appelle désormais %s (l'ancien nom est retiré).\n", cli)
	}
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
				fmt.Fprintf(os.Stderr, "\nUne nouvelle version est disponible : %s (vous avez %s). Pour l'installer : %s --update\n",
					v, gallery.Version, filepath.Base(os.Args[0]))
			}
		case <-time.After(1500 * time.Millisecond):
		}
	}
}
