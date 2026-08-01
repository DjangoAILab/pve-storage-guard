package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestShadowCommandEmitsNonActuatingProposal(t *testing.T) {
	root := filepath.Join("..", "..")
	now := time.Now().UTC()
	input := fmt.Sprintf(`{"schemaVersion":"guard.storage-slo.io/v1alpha1","id":"observation-cli-1","observedAt":%q,"domainKey":"reference-bulk-pool","writeWaitP95Milliseconds":120,"waitValid":true,"emergency":false,"managementPlaneHealthy":true}`+"\n", now.Format(time.RFC3339Nano))
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"shadow",
		"--policy", filepath.Join(root, "configs", "examples", "reference-shadow-policy.json"),
		"--enrollment", filepath.Join(root, "configs", "examples", "reference-enrollment.json"),
	}, strings.NewReader(input), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"actuationAllowed":false`) || !strings.Contains(stdout.String(), `"reason":"emergency"`) {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestShadowCommandRejectsMissingEnrollment(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"shadow", "--policy", "unused.json"}, strings.NewReader(""), &stdout, &stderr)
	if code == 0 || !strings.Contains(stderr.String(), "at least one --enrollment") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}
