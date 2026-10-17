package gallery

import (
	"path/filepath"
	"syscall"
	"unsafe"
)

var getDriveType = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDriveTypeW")

const driveRemote = 4 // DRIVE_REMOTE

// networkFS indique si dir est sur un partage réseau (chemin \\serveur\partage
// ou lecteur réseau connecté).
func networkFS(dir string) (string, bool) {
	path, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	vol := filepath.VolumeName(path)
	if vol == "" {
		return "", false
	}
	root, err := syscall.UTF16PtrFromString(vol + `\`)
	if err != nil {
		return "", false
	}
	t, _, _ := getDriveType.Call(uintptr(unsafe.Pointer(root)))
	if t == driveRemote {
		return "partage réseau", true
	}
	return "", false
}
