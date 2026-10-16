package gallery

import (
	"slices"
	"testing"
	"time"
)

func TestSortItems(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2024, 8, d, 12, 0, 0, 0, time.UTC) }
	mk := func(name string, size int64, dur float64, mtime time.Time, created int64) item {
		return item{Video: Video{Name: name, Size: size, ModTime: mtime}, Meta: Meta{Duration: dur, Created: created}}
	}
	// Ordre d'arrivée = alphabétique, comme après Scan. « c » a été copiée
	// récemment (date de fichier le 30) mais tournée le 1er : la date de
	// tournage doit primer. « b » et « d » ont la même durée.
	base := []item{
		mk("a", 300, 10, day(5), 0),
		mk("b", 100, 30, day(10), 0),
		mk("c", 200, 20, day(30), day(1).Unix()),
		mk("d", 400, 30, day(20), 0),
	}
	cases := map[string]string{
		OrderName:         "abcd",
		OrderNameDesc:     "dcba",
		OrderDate:         "cabd",
		OrderDateDesc:     "dbac",
		OrderDuration:     "acbd",
		OrderDurationDesc: "bdca", // égalité b/d : ordre alphabétique conservé
		OrderSize:         "bcad",
		OrderSizeDesc:     "dacb",
		OrderNone:         "abcd",
	}
	if len(cases) != len(Orders) {
		t.Fatalf("%d cas pour %d ordres", len(cases), len(Orders))
	}
	for order, want := range cases {
		items := slices.Clone(base)
		sortItems(items, order)
		got := ""
		for _, it := range items {
			got += it.Name
		}
		if got != want {
			t.Errorf("%s : %s, attendu %s", order, got, want)
		}
	}
	if err := validOrder("taille"); err == nil {
		t.Error("ordre inconnu accepté")
	}
}

func TestParseCreated(t *testing.T) {
	cases := map[string]int64{
		"2024-08-24T10:11:12.000000Z": time.Date(2024, 8, 24, 10, 11, 12, 0, time.UTC).Unix(),
		"2024-08-24T10:11:12Z":        time.Date(2024, 8, 24, 10, 11, 12, 0, time.UTC).Unix(),
		"1970-01-01T00:00:00.000000Z": 0, // date nulle de certains appareils
		"1904-01-01T00:00:00Z":        0,
		"":                            0,
		"hier":                        0,
	}
	for in, want := range cases {
		if got := parseCreated(in); got != want {
			t.Errorf("parseCreated(%q) = %d, attendu %d", in, got, want)
		}
	}
}
