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

// Avant la 1.1.0, le tri était une case à cocher ("sort": bool).
func TestLoadLegacySort(t *testing.T) {
	dir := t.TempDir()
	cases := []struct{ json, want string }{
		{`{"sort": false}`, "none"},
		{`{"sort": true}`, "name"},
		{`{"sort": false, "order": "size"}`, "size"}, // le nouveau champ prime
		{`{"order": "n'importe quoi"}`, "name"},
		{`{"order": "date-desc"}`, "date-desc"},
	}
	for i, c := range cases {
		path := filepath.Join(dir, "p.json")
		if err := os.WriteFile(path, []byte(c.json), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := Load(path).Order; got != c.want {
			t.Errorf("cas %d %s : Order = %q, attendu %q", i, c.json, got, c.want)
		}
	}
}
