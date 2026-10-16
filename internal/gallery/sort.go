package gallery

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// Ordres de tri des vidéos (Options.Order). Le suffixe « -desc » inverse le
// sens ; à égalité, l'ordre alphabétique départage.
const (
	OrderName         = "name"          // nom, A → Z (défaut)
	OrderNameDesc     = "name-desc"     // nom, Z → A
	OrderDateDesc     = "date-desc"     // plus récentes d'abord
	OrderDate         = "date"          // plus anciennes d'abord
	OrderDurationDesc = "duration-desc" // plus longues d'abord
	OrderDuration     = "duration"      // plus courtes d'abord
	OrderSizeDesc     = "size-desc"     // plus lourdes d'abord
	OrderSize         = "size"          // plus légères d'abord
	OrderNone         = "none"          // ordre du système de fichiers
)

// Orders liste les ordres acceptés, dans l'ordre où les proposer.
var Orders = []string{
	OrderName, OrderNameDesc, OrderDateDesc, OrderDate,
	OrderDurationDesc, OrderDuration, OrderSizeDesc, OrderSize, OrderNone,
}

func validOrder(o string) error {
	if slices.Contains(Orders, o) {
		return nil
	}
	return fmt.Errorf("ordre de tri inconnu %q (attendu : %s)", o, strings.Join(Orders, ", "))
}

// shotTime : date de tournage si la vidéo l'indique, sinon date du fichier
// (qui change à chaque copie ou téléchargement, d'où la préférence).
func (it item) shotTime() int64 {
	if it.Created != 0 {
		return it.Created
	}
	return it.ModTime.Unix()
}

// sortItems trie sur place. Les items arrivent dans l'ordre alphabétique de
// Scan (sauf OrderNone) : un tri stable garde cet ordre à égalité.
func sortItems(items []item, order string) {
	var key func(a, b item) int
	switch strings.TrimSuffix(order, "-desc") {
	case OrderName:
		if order == OrderNameDesc {
			slices.Reverse(items)
		}
		return
	case OrderDate:
		key = func(a, b item) int { return cmp.Compare(a.shotTime(), b.shotTime()) }
	case OrderDuration:
		key = func(a, b item) int { return cmp.Compare(a.Duration, b.Duration) }
	case OrderSize:
		key = func(a, b item) int { return cmp.Compare(a.Size, b.Size) }
	default: // OrderNone
		return
	}
	if strings.HasSuffix(order, "-desc") {
		asc := key
		key = func(a, b item) int { return asc(b, a) }
	}
	slices.SortStableFunc(items, key)
}
