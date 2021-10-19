package e2e

import (
	"embed"
	"os"
	"os/exec"
)

//go:embed fixtures/*
var fixtures embed.FS

func kubectl(args ...string) *exec.Cmd {
	cmd := exec.Command("kubectl", args...)
	cmd.Env = os.Environ()
	return cmd
}
