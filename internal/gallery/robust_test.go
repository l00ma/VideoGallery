package gallery

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fakeTool écrit un script shell qui tient lieu de ffmpeg.
func fakeTool(t *testing.T, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("script shell")
	}
	path := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func shortTimeouts(t *testing.T, call, grace time.Duration) {
	oldCall, oldGrace := callTimeout, killGrace
	callTimeout, killGrace = call, grace
	t.Cleanup(func() { callTimeout, killGrace = oldCall, oldGrace })
}

// Un ffmpeg qui ne répond plus (lecture réseau bloquée) ne doit pas figer la
// génération : l'appel échoue au bout de callTimeout.
func TestFrameTimesOut(t *testing.T) {
	shortTimeouts(t, 300*time.Millisecond, time.Second)
	tools := Tools{FFmpeg: fakeTool(t, "exec sleep 30")}
	start := time.Now()
	_, err := tools.Frame(context.Background(), "video.mp4", 1, 120)
	if err == nil || !strings.Contains(err.Error(), "aucune réponse") {
		t.Fatalf("erreur attendue « aucune réponse », obtenu %v", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("délai non respecté : %v", d)
	}
}

// Un processus qui survit à SIGKILL (bloqué dans le noyau sur une lecture
// réseau) est abandonné. Simulé ici par un processus fils qui garde la sortie
// standard ouverte : Wait attendrait sa fin.
func TestFrameAbandonsUnkillable(t *testing.T) {
	shortTimeouts(t, 200*time.Millisecond, 300*time.Millisecond)
	tools := Tools{FFmpeg: fakeTool(t, "sleep 5 &\nwait")}
	start := time.Now()
	if _, err := tools.Frame(context.Background(), "video.mp4", 1, 120); err == nil {
		t.Fatal("erreur attendue")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("processus non abandonné : %v", d)
	}
}

// Une annulation n'est pas rapportée comme un délai dépassé.
func TestFrameCancel(t *testing.T) {
	tools := Tools{FFmpeg: fakeTool(t, "exec sleep 30")}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := tools.Frame(ctx, "video.mp4", 1, 120); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("annulation attendue, obtenu %v", err)
	}
}

// La capture centrale sert de miniature : 9 appels à ffmpeg par vidéo, pas
// 10, et des images aux bonnes dimensions dans les deux sens (miniature plus
// large ou plus étroite que les captures).
func TestOneCallPerCapture(t *testing.T) {
	tools, err := FindTools()
	if err != nil {
		t.Skip("ffmpeg indisponible")
	}
	dir := t.TempDir()
	cmd := exec.Command(tools.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=10:duration=4",
		filepath.Join(dir, "v.mp4"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	calls := filepath.Join(t.TempDir(), "calls")
	counted := tools
	counted.FFmpeg = fakeTool(t, fmt.Sprintf("echo >> %q\nexec %q \"$@\"", calls, tools.FFmpeg))

	for _, c := range []struct{ cols, frameW int }{{2, 120}, {4, 320}} {
		os.Remove(calls)
		opts := Options{VideoDir: dir, Cols: c.cols, Rows: 1, FrameWidth: c.frameW, Tools: counted, Force: true}
		if _, err := Run(context.Background(), opts); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(calls)
		if n := strings.Count(string(data), "\n"); n != sheetFrames {
			t.Errorf("cols=%d : %d appels à ffmpeg, attendu %d", c.cols, n, sheetFrames)
		}
		sheetName, thumbName := imageNames(opts, Video{Name: "v.mp4"})
		tw, th, err := imageSize(filepath.Join(dir, "pages", "img", thumbName))
		if err != nil {
			t.Fatal(err)
		}
		wantW := thumbWidth(c.cols)
		if wantH := evenHeight(320, 180, wantW); tw != wantW || th != wantH {
			t.Errorf("cols=%d : miniature %d×%d, attendu %d×%d", c.cols, tw, th, wantW, wantH)
		}
		sw, _, err := imageSize(filepath.Join(dir, "pages", "img", sheetName))
		if err != nil {
			t.Fatal(err)
		}
		if want := sheetGrid*c.frameW + (sheetGrid-1)*sheetGap; sw != want {
			t.Errorf("cols=%d : planche de %d px de large, attendu %d", c.cols, sw, want)
		}
	}
}

// meta.json est réécrit en cours de génération, pas seulement à la fin.
func TestMetaCheckpoint(t *testing.T) {
	dir := t.TempDir()
	c := loadMetaCache(dir)
	var names []string
	for i := range metaCheckpoint {
		v := Video{Name: fmt.Sprintf("v%d.mp4", i), Size: 1, ModTime: time.Unix(1, 0)}
		names = append(names, v.Name)
		c.put(v, Meta{Duration: 1})
		c.checkpoint(dir, names)
		_, err := os.Stat(filepath.Join(dir, metaFile))
		if done := i+1 == metaCheckpoint; done == errors.Is(err, os.ErrNotExist) {
			t.Fatalf("après %d entrées : meta.json présent = %v", i+1, err == nil)
		}
	}
	if got := len(loadMetaCache(dir).entries); got != metaCheckpoint {
		t.Errorf("%d entrées relues, attendu %d", got, metaCheckpoint)
	}
}
