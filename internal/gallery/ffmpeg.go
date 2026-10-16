package gallery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Tools regroupe les chemins des exécutables ffmpeg et ffprobe.
type Tools struct {
	FFmpeg  string
	FFprobe string
}

// FindTools cherche ffmpeg et ffprobe à côté de l'exécutable (livraison
// Windows en zip), puis dans le PATH.
func FindTools() (Tools, error) {
	ffmpeg, err := findTool("ffmpeg")
	if err != nil {
		return Tools{}, err
	}
	ffprobe, err := findTool("ffprobe")
	if err != nil {
		return Tools{}, err
	}
	return Tools{FFmpeg: ffmpeg, FFprobe: ffprobe}, nil
}

func findTool(name string) (string, error) {
	file := name
	if runtime.GOOS == "windows" {
		file += ".exe"
	}
	if self, err := os.Executable(); err == nil {
		local := filepath.Join(filepath.Dir(self), file)
		if st, err := os.Stat(local); err == nil && !st.IsDir() {
			return local, nil
		}
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s introuvable (ni à côté de l'exécutable, ni dans le PATH)", name)
	}
	return path, nil
}

// lastLine garde la dernière ligne non vide de la sortie d'erreur d'un outil,
// la plus parlante (« Invalid data found when processing input »).
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// Meta regroupe les métadonnées d'une vidéo utiles à la planche.
type Meta struct {
	Duration float64 `json:"duration"` // secondes
	Width    int     `json:"width"`    // résolution telle qu'affichée (rotation appliquée)
	Height   int     `json:"height"`
	// Created : date de tournage (creation_time du conteneur), en secondes
	// Unix ; 0 si absente ou invraisemblable.
	Created int64  `json:"created,omitempty"`
	Codec   string `json:"codec"` // codec vidéo selon ffprobe : h264, hevc, vp9...
}

// webCodec : codecs que les navigateurs courants lisent partout (Firefox,
// Chromium, Safari, Edge). Le HEVC des iPhone n'en fait pas partie : sa
// lecture dépend du système (décodeur matériel, extension HEVC sous Windows,
// ffmpeg du système pour Firefox sous Linux). Il est souvent lu, pas toujours.
func webCodec(codec string) bool {
	switch codec {
	case "h264", "vp8", "vp9", "av1", "theora":
		return true
	}
	return false
}

// codecLabel : nom lisible d'un codec pour les messages.
func codecLabel(codec string) string {
	switch codec {
	case "hevc":
		return "HEVC (H.265)"
	case "mpeg4":
		return "MPEG-4 Part 2"
	case "":
		return "inconnu"
	}
	return strings.ToUpper(codec)
}

type probeOutput struct {
	Streams []struct {
		CodecName    string `json:"codec_name"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		Tags         map[string]string
		SideDataList []struct {
			Rotation float64 `json:"rotation"`
		} `json:"side_data_list"`
	} `json:"streams"`
	Format struct {
		Duration string            `json:"duration"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
}

// Probe lit la durée et la résolution de la vidéo. Pour une vidéo filmée en
// portrait (rotation de 90° ou 270° dans les métadonnées), largeur et hauteur
// sont échangées : c'est la résolution que voit le spectateur.
func (t Tools) Probe(ctx context.Context, path string) (Meta, error) {
	cmd := exec.CommandContext(ctx, t.FFprobe,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_name,width,height:stream_tags=rotate:stream_side_data=rotation:format=duration:format_tags=creation_time",
		"-of", "json",
		path)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return Meta{}, fmt.Errorf("ffprobe: %w: %s", err, lastLine(stderr.String()))
	}
	var po probeOutput
	if err := json.Unmarshal(out, &po); err != nil {
		return Meta{}, fmt.Errorf("lecture de la sortie ffprobe: %w", err)
	}

	d, err := strconv.ParseFloat(strings.TrimSpace(po.Format.Duration), 64)
	if err != nil || math.IsNaN(d) || math.IsInf(d, 0) || d <= 0 {
		return Meta{}, errors.New("durée de la vidéo introuvable")
	}
	m := Meta{Duration: d, Created: parseCreated(po.Format.Tags["creation_time"])}
	if len(po.Streams) == 0 {
		return Meta{}, errors.New("aucun flux vidéo")
	}
	st := po.Streams[0]
	m.Width, m.Height, m.Codec = st.Width, st.Height, st.CodecName

	rotation := 0.0
	for _, sd := range st.SideDataList {
		if sd.Rotation != 0 {
			rotation = sd.Rotation
		}
	}
	if r, err := strconv.ParseFloat(st.Tags["rotate"], 64); err == nil && rotation == 0 {
		rotation = r
	}
	if int(math.Abs(rotation))%180 == 90 {
		m.Width, m.Height = m.Height, m.Width
	}
	return m, nil
}

// Frame extrait l'image située à l'instant « at » (secondes), mise à la
// largeur donnée. La hauteur suit le ratio d'origine (arrondie au pair).
func (t Tools) Frame(ctx context.Context, path string, at float64, width int) (image.Image, error) {
	cmd := exec.CommandContext(ctx, t.FFmpeg,
		"-v", "error",
		"-threads", "1", // le parallélisme se fait entre vidéos
		"-ss", strconv.FormatFloat(at, 'f', 3, 64),
		"-i", path,
		"-frames:v", "1",
		"-vf", fmt.Sprintf("scale=%d:-2", width),
		"-c:v", "mjpeg", "-q:v", "2",
		"-f", "image2pipe", "-")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg: %w: %s", err, lastLine(stderr.String()))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("aucune image extraite à %.1fs", at)
	}
	img, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		return nil, fmt.Errorf("décodage de l'image extraite: %w", err)
	}
	return img, nil
}

// parseCreated lit le creation_time de ffprobe (« 2024-08-24T12:34:56.000000Z »).
// Certains appareils écrivent une date nulle (1904, 1970) : elle est ignorée.
func parseCreated(s string) int64 {
	t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(s))
	if err != nil || t.Year() < 1990 {
		return 0
	}
	return t.Unix()
}
