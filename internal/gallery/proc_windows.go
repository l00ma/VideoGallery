package gallery

import (
	"os/exec"
	"syscall"
)

// createNoWindow (CREATE_NO_WINDOW) : le processus n'a pas de console. Sans
// cela, chaque ffmpeg/ffprobe lancé depuis la GUI (qui n'a pas de console)
// ouvrirait sa propre fenêtre de terminal le temps de son exécution.
const createNoWindow = 0x08000000

// silence empêche ffmpeg/ffprobe d'ouvrir une fenêtre de console. Leurs
// sorties restent capturées par des tubes.
func silence(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return cmd
}
