package gallery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// metaFile, dans img/, mémorise les métadonnées lues par ffprobe (durée,
// résolution) : l'index et le tri en ont besoin, y compris pour les vidéos
// dont les images sont en cache, et un ffprobe coûte ~100 ms par vidéo.
const metaFile = "meta.json"

type metaEntry struct {
	Size    int64 `json:"size"`
	ModTime int64 `json:"mtime"` // nanosecondes Unix
	Meta
}

// metaCache est partagé par les goroutines de buildImages.
type metaCache struct {
	mu      sync.Mutex
	entries map[string]metaEntry
}

func loadMetaCache(imgDir string) *metaCache {
	c := &metaCache{entries: map[string]metaEntry{}}
	if data, err := os.ReadFile(filepath.Join(imgDir, metaFile)); err == nil {
		_ = json.Unmarshal(data, &c.entries) // cache illisible : on repart de zéro
	}
	return c
}

// get renvoie les métadonnées de v si elles correspondent encore au fichier
// (même taille, même date de modification).
func (c *metaCache) get(v Video) (Meta, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[v.Name]
	if !ok || e.Size != v.Size || e.ModTime != v.ModTime.UnixNano() || e.Duration <= 0 {
		return Meta{}, false
	}
	return e.Meta, true
}

func (c *metaCache) put(v Video, m Meta) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[v.Name] = metaEntry{Size: v.Size, ModTime: v.ModTime.UnixNano(), Meta: m}
}

// save n'écrit que les vidéos de la galerie (les disparues sont oubliées),
// via un fichier temporaire comme les images.
func (c *metaCache) save(imgDir string, items []item) error {
	keep := make(map[string]metaEntry, len(items))
	c.mu.Lock()
	for _, it := range items {
		if e, ok := c.entries[it.Name]; ok {
			keep[it.Name] = e
		}
	}
	c.mu.Unlock()
	data, err := json.MarshalIndent(keep, "", " ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(imgDir, ".tmp-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(imgDir, metaFile))
}
