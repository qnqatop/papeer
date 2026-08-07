//go:build darwin

package app

import "os/exec"

func openFileCommand(path string) *exec.Cmd {
	return exec.Command("open", path)
}
