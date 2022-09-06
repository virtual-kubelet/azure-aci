package e2e

import (
	"os"
	"os/exec"
	"regexp"
)

//execute kubectl command in terminal
func kubectl(args ...string) *exec.Cmd {
	cmd := exec.Command("kubectl", args...)
	cmd.Env = os.Environ()
	return cmd
}
