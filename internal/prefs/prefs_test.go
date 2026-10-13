package prefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingGivesDefaults(t *testing.T) {
	got := Load(filepath.Join(t.TempDir(), "absent.json"))
	if got != Defaults() {
		t.Errorf("Load(absent) = %+v", got)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sous", "dossier", "prefs.json")
	want := Defaults()
	want.VideoDir, want.Name, want.Cols, want.OpenBrowser = "/vidéos", "Été", 3, true
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	if got := Load(path); got != want {
		t.Errorf("Load = %+v, attendu %+v", got, want)
	}
}

func TestLoadCorruptedOrOutOfRange(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte("{pas du json"), 0o644)
	if got := Load(bad); got != Defaults() {
		t.Errorf("JSON invalide: %+v", got)
	}

	oor := filepath.Join(dir, "oor.json")
	os.WriteFile(oor, []byte(`{"cols":0,"rows":9999,"frameWidth":5,"background":"rouge","foreground":"#12345g","name":"ok"}`), 0o644)
	got := Load(oor)
	d := Defaults()
	if got.Cols != d.Cols || got.Rows != d.Rows || got.FrameWidth != d.FrameWidth ||
		got.Background != d.Background || got.Foreground != d.Foreground {
		t.Errorf("valeurs hors bornes non corrigées: %+v", got)
	}
	if got.Name != "ok" {
		t.Errorf("les champs valides doivent être conservés: %+v", got)
	}
}

func TestImportLegacy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mes_valeurs.sav")
	// format exact écrit par la version Perl (sans retour à la ligne final)
	os.WriteFile(path, []byte("/home/fred/videos|Vacances|4|5|1|280|235|0|186,175,145|51,51,51|1"), 0o644)
	p, err := ImportLegacy(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.VideoDir != "/home/fred/videos" || p.Name != "Vacances" || p.Cols != 4 || p.Rows != 5 ||
		!p.Sort || p.Background != "#baaf91" || p.Foreground != "#333333" || !p.OpenBrowser {
		t.Errorf("import incorrect: %+v", p)
	}
	// 280 est la largeur de miniature Perl : valide aussi comme largeur de capture
	if p.FrameWidth != 280 {
		t.Errorf("FrameWidth = %d", p.FrameWidth)
	}

	os.WriteFile(path, []byte("trop|court"), 0o644)
	if _, err := ImportLegacy(path); err == nil {
		t.Error("erreur attendue pour un fichier tronqué")
	}
}
