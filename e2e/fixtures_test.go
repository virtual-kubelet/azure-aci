package e2e

import (
	"os"
	"os/exec"
	"regexp"
)

//delete invisible characters
func cleanString(toClean string) string {
	re := regexp.MustCompile("\\x1B\\[[0-9;]*[a-zA-Z]")
	return re.ReplaceAllString(toClean, "")
}

//execute kubectl command in terminal
func kubectl(args ...string) *exec.Cmd {
	cmd := exec.Command("kubectl", args...)
	cmd.Env = os.Environ()
	return cmd
}

//execute helm command in terminal
func helm(args ...string) *exec.Cmd {
	cmd := exec.Command("helm", args...)
	cmd.Env = os.Environ()
	return cmd
}

//execute az command in terminal
func az(args ...string) *exec.Cmd {
	cmd := exec.Command("az", args...)
	cmd.Env = os.Environ()
	return cmd
}
