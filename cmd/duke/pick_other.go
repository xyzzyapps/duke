//go:build !windows && !darwin

package main

import (
	"os/exec"
	"strings"
)

// pickSavePath shows a GTK/KDE save dialog when available (zenity or
// kdialog). With neither installed it falls back to the given default
// (working directory), so Save still saves the tab to a file.
func pickSavePath(def string) (string, bool) {
	candidates := [][]string{
		{"zenity", "--file-selection", "--save", "--confirm-overwrite", "--filename", def},
		{"kdialog", "--getsavefilename", def},
	}
	for _, argv := range candidates {
		if _, err := exec.LookPath(argv[0]); err != nil {
			continue // tool not installed: try the next
		}
		out, err := exec.Command(argv[0], argv[1:]...).Output()
		if err != nil {
			return "", false // ran but was cancelled
		}
		p := strings.TrimSpace(string(out))
		if p == "" {
			return "", false
		}
		return p, true
	}
	return def, true // no dialog available: save next to the working dir
}

// pickOpenPath shows a GTK/KDE file picker when available (zenity or
// kdialog). With neither installed the open is cancelled.
func pickOpenPath(def string) (string, bool) {
	candidates := [][]string{
		{"zenity", "--file-selection", "--filename", def},
		{"kdialog", "--getopenfilename", def},
	}
	for _, argv := range candidates {
		if _, err := exec.LookPath(argv[0]); err != nil {
			continue // tool not installed: try the next
		}
		out, err := exec.Command(argv[0], argv[1:]...).Output()
		if err != nil {
			return "", false // ran but was cancelled
		}
		p := strings.TrimSpace(string(out))
		if p == "" {
			return "", false
		}
		return p, true
	}
	return "", false // no dialog available
}
