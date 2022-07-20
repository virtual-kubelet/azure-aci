package e2e

import (
	"os"
	"os/exec"
)

func kubectl(args ...string) *exec.Cmd {
	cmd := exec.Command("kubectl", args...)
	cmd.Env = os.Environ()
	return cmd
}
