//go:build darwin

package main

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// pickSavePath shows the macOS save dialog via osascript. Returns ok=false
// when the user cancels.
func pickSavePath(def string) (string, bool) {
	script := `POSIX path of (choose file name with default "` +
		filepath.Base(def) + `")`
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return "", false // user cancelled
	}
	p := strings.TrimSpace(string(out))
	if p == "" {
		return "", false
	}
	return p, true
}

// pickOpenPath shows the macOS open dialog via osascript. Returns ok=false
// when the user cancels.
func pickOpenPath(def string) (string, bool) {
	out, err := exec.Command("osascript", "-e", `POSIX path of (choose file)`).Output()
	if err != nil {
		return "", false // user cancelled
	}
	p := strings.TrimSpace(string(out))
	if p == "" {
		return "", false
	}
	return p, true
}
