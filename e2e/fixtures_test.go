package e2e

import (
	"os"
	"os/exec"
	"regexp"
	"testing"
	"time"
)

//delete invisible characters
func cleanString(toClean string) string {
	re := regexp.MustCompile("\\x1B\\[[0-9;]*[a-zA-Z]")
	return re.ReplaceAllString(toClean, "")
}

func kubectl(args ...string) *exec.Cmd {
	cmd := exec.Command("kubectl", args...)
	cmd.Env = os.Environ()
	return cmd
}

func helm(args ...string) *exec.Cmd {
	cmd := exec.Command("helm", args...)
	cmd.Env = os.Environ()
	return cmd
}

func az(args ...string) *exec.Cmd {
	cmd := exec.Command("az", args...)
	cmd.Env = os.Environ()
	return cmd
}

//create the pod 'podName' with the pod specs on 'podDir'
func CreatePodFromKubectl(t *testing.T, podName string, podDir string, namespace string) {
	cmd := kubectl("apply", "-f", podDir, "--namespace="+namespace)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}
	t.Log(string(out))

	deadline, ok := t.Deadline()
	timeout := time.Until(deadline)
	if !ok {
		timeout = 300 * time.Second
	}
	cmd = kubectl("wait", "--for=condition=ready", "--timeout="+timeout.String(), "pod/"+podName, "--namespace="+namespace)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	t.Log("success create pod")
}

//delete pod
func DeletePodFromKubectl(t *testing.T, podName string, namespace string) {
	t.Log("clean up pod")
	cmd := kubectl("delete", "pod/"+podName, "--namespace="+namespace)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
}
