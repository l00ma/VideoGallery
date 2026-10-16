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

// mimeByExt donne le type annoncé dans <source>. Un .mov (téléphones, appareils
// photo) est annoncé en video/mp4, que tous les navigateurs connaissent :
// « video/quicktime » n'est pas reconnu partout (Firefox 157 l'accepte, mais
// un navigateur qui ne le connaît pas écarte la source sans l'essayer), alors
// que le contenu, du H.264 ou du HEVC dans un conteneur MP4/QuickTime, est le
// même (voir webCodec pour le HEVC).
var mimeByExt = map[string]string{
	".mp4":  "video/mp4",
	".m4v":  "video/mp4",
	".mov":  "video/mp4",
	".webm": "video/webm",
	".ogg":  "video/ogg",
}

// Formats énumère les extensions acceptées, pour les messages.
const Formats = "mp4, m4v, mov, webm, ogg"

// Scan liste les vidéos (voir Formats) d'un répertoire, sans récursivité.
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
