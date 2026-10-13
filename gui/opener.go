package main

import (
	"os/exec"
	goruntime "runtime"
)

// openFile ouvre un fichier avec l'application par défaut du système. Les
// arguments sont passés en liste, jamais par un shell : un chemin contenant
// des guillemets ou des « $ » ne peut rien exécuter.
func openFile(path string) error {
	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() // évite un processus zombie
	return nil
}
