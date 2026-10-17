//go:build !windows

package gallery

import "os/exec"

// silence n'a d'effet que sous Windows (voir proc_windows.go).
func silence(cmd *exec.Cmd) *exec.Cmd { return cmd }
