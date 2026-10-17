//go:build !linux && !windows

package gallery

// networkFS : détection non prise en charge sur ce système.
func networkFS(string) (string, bool) { return "", false }
