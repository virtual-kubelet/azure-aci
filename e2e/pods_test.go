package e2e

import (
	"bytes"
	"testing"
	"time"
)

func TestPodLifecycle(t *testing.T) {
	spec, err := fixtures.ReadFile("fixtures/nginx.yml")
	if err != nil {
		t.Fatal(err)
	}

	cmd := kubectl("apply", "-f", "-")
	cmd.Stdin = bytes.NewReader(spec)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}

	deadline, ok := t.Deadline()
	timeout := time.Until(deadline)
	if !ok {
		timeout = 300 * time.Second
	}
	cmd = kubectl("wait", "--for=condition=available", "--timeout="+timeout.String(), "deploy/nginx")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}

	cmd = kubectl("delete", "deploy/nginx")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
}
