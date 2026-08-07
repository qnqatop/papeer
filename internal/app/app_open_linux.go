//go:build linux

package app

import "os/exec"

func openFileCommand(path string) *exec.Cmd {
	return exec.Command("xdg-open", path)
}
