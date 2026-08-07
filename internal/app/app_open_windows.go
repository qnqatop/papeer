//go:build windows

package app

import "os/exec"

func openFileCommand(path string) *exec.Cmd {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
}
