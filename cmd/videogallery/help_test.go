package main

import (
	"flag"
	"strings"
	"testing"
	"unicode/utf8"

	"videogallery/internal/gallery"
)

// Toute option déclarée doit être documentée dans l'aide, chaque ordre de tri
// aussi, et aucun repère {…} ne doit rester sans valeur.
func TestHelpDocumentsEverything(t *testing.T) {
	fs := newFlagSet(&cli{})
	help := helpText(fs, "videogallery")
	fs.VisitAll(func(f *flag.Flag) {
		if !strings.Contains(help, "\n  -"+f.Name+" ") {
			t.Errorf("option -%s absente de l'aide", f.Name)
		}
	})
	for _, o := range gallery.Orders {
		if !strings.Contains(help, "\n                      "+o+" ") {
			t.Errorf("ordre %q absent de l'aide", o)
		}
	}
	if strings.Contains(help, "{") {
		t.Errorf("repère non remplacé dans l'aide:\n%s", help)
	}
	for i, line := range strings.Split(help, "\n") {
		if n := utf8.RuneCountInString(line); n > 80 {
			t.Errorf("ligne %d trop longue (%d caractères) : %s", i+1, n, line)
		}
	}
}

func TestFlagErrorsInFrench(t *testing.T) {
	cases := map[string]string{
		"-colonnes":                  "option inconnue : -colonnes",
		"-sort":                      "cette option attend une valeur : -sort",
		"-cols=trois":                `valeur invalide "trois" pour l'option -cols : nombre entier attendu`,
		"-open=peut-être":            `valeur invalide "peut-être" pour l'option -open : une option comme -open s'écrit seule (ou -open=false)`,
		"-rows=99999999999999999999": `valeur invalide "99999999999999999999" pour l'option -rows : nombre trop grand`,
	}
	for arg, want := range cases {
		err := newFlagSet(&cli{}).Parse([]string{arg})
		if err == nil {
			t.Errorf("%s accepté", arg)
			continue
		}
		if got := flagError(err); got != want {
			t.Errorf("%s : %q, attendu %q", arg, got, want)
		}
	}
}
