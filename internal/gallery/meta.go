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

// metaVersion change quand Meta s'enrichit : un cache d'une autre version est
// ignoré (ffprobe est relancé, les images, elles, restent en cache).
const metaVersion = 3

type metaJSON struct {
	Version int                  `json:"version"`
	Videos  map[string]metaEntry `json:"videos"`
}

type metaEntry struct {
	Size    int64 `json:"size"`
	ModTime int64 `json:"mtime"` // nanosecondes Unix
	Meta
}

// metaCheckpoint : nombre de vidéos lues par ffprobe au-delà duquel meta.json
// est réécrit en cours de génération. Une génération interrompue (coupure
// réseau, programme tué) ne perd ainsi que les dernières lectures.
const metaCheckpoint = 20

// metaCache est partagé par les goroutines de buildImages.
type metaCache struct {
	mu      sync.Mutex
	entries map[string]metaEntry
	dirty   int        // entrées ajoutées depuis le dernier enregistrement
	saveMu  sync.Mutex // un seul enregistrement à la fois
}

func loadMetaCache(imgDir string) *metaCache {
	c := &metaCache{entries: map[string]metaEntry{}}
	var f metaJSON
	if data, err := os.ReadFile(filepath.Join(imgDir, metaFile)); err == nil &&
		json.Unmarshal(data, &f) == nil && f.Version == metaVersion && f.Videos != nil {
		c.entries = f.Videos // sinon (absent, illisible, ancien) : on repart de zéro
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
	c.dirty++
}

// checkpoint enregistre le cache si assez d'entrées ont été ajoutées depuis
// le dernier enregistrement. Un échec n'est pas grave (simple ralentissement
// au prochain passage) : il est ignoré.
func (c *metaCache) checkpoint(imgDir string, names []string) {
	c.mu.Lock()
	due := c.dirty >= metaCheckpoint
	c.mu.Unlock()
	if due {
		c.save(imgDir, names)
	}
}

// save n'écrit que les vidéos nommées (les disparues sont oubliées), via un
// fichier temporaire comme les images.
func (c *metaCache) save(imgDir string, names []string) error {
	c.saveMu.Lock()
	defer c.saveMu.Unlock()
	keep := make(map[string]metaEntry, len(names))
	c.mu.Lock()
	for _, name := range names {
		if e, ok := c.entries[name]; ok {
			keep[name] = e
		}
	}
	c.dirty = 0
	c.mu.Unlock()
	data, err := json.MarshalIndent(metaJSON{Version: metaVersion, Videos: keep}, "", " ")
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
