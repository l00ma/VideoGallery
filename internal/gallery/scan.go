package gallery

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Video décrit un fichier vidéo trouvé dans le répertoire source.
type Video struct {
	Name    string // nom de fichier, sans chemin
	Path    string // chemin complet
	MIME    string
	Size    int64 // octets
	ModTime time.Time
}

var mimeByExt = map[string]string{
	".mp4":  "video/mp4",
	".webm": "video/webm",
	".ogg":  "video/ogg",
}

// Scan liste les vidéos (mp4, webm, ogg) d'un répertoire, sans récursivité.
func Scan(dir string, sorted bool) ([]Video, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("lecture du répertoire %s: %w", dir, err)
	}
	var videos []Video
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		mime, ok := mimeByExt[strings.ToLower(filepath.Ext(e.Name()))]
		if !ok {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		videos = append(videos, Video{
			Name:    e.Name(),
			Path:    filepath.Join(dir, e.Name()),
			MIME:    mime,
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}
	if sorted {
		sort.SliceStable(videos, func(i, j int) bool {
			a, b := strings.ToLower(videos[i].Name), strings.ToLower(videos[j].Name)
			if a != b {
				return a < b
			}
			return videos[i].Name < videos[j].Name
		})
	}
	return videos, nil
}
