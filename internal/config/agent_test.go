package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadPVEAgentConfigRejectsInjectionAndUnknownFields(t *testing.T) {
	valid := `{"apiVersion":"guard.storage-slo.io/v1alpha1","kind":"PVEAgentConfig","spec":{"domainKey":"reference-pool","node":"node-a","storage":"storage-a","zpool":"pool-a","sampleIntervalSeconds":1,"commandTimeoutSeconds":5,"emergencyWaitMilliseconds":100,"resources":[{"resourceKey":"resource-a","kernelDevice":"sdb","root":false,"critical":false}]}}`
	for name, payload := range map[string]string{
		"unknown":          strings.Replace(valid, `"domainKey"`, `"unknown":1,"domainKey"`, 1),
		"node injection":   strings.Replace(valid, `"node-a"`, `"node-a;touch"`, 1),
		"path segment":     strings.Replace(valid, `"node-a"`, `".."`, 1),
		"device path":      strings.Replace(valid, `"sdb"`, `"../../sdb"`, 1),
		"opaque uppercase": strings.Replace(valid, `"reference-pool"`, `"ReferencePool"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "agent.json")
			if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := ReadPVEAgentConfig(path); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestReadPVEAgentConfigRejectsReadableOrNonRegularFile(t *testing.T) {
	payload := []byte(`{"apiVersion":"guard.storage-slo.io/v1alpha1","kind":"PVEAgentConfig","spec":{"domainKey":"reference-pool","node":"node-a","storage":"storage-a","zpool":"pool-a","sampleIntervalSeconds":1,"commandTimeoutSeconds":5,"emergencyWaitMilliseconds":100,"resources":[{"resourceKey":"resource-a","kernelDevice":"sdb","root":false,"critical":false}]}}`)
	directory := t.TempDir()
	readable := filepath.Join(directory, "readable.json")
	if err := os.WriteFile(readable, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPVEAgentConfig(readable); err == nil {
		t.Fatal("expected group/other-readable config rejection")
	}
	link := filepath.Join(directory, "agent-link.json")
	private := filepath.Join(directory, "private.json")
	if err := os.WriteFile(private, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(private, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPVEAgentConfig(link); err == nil {
		t.Fatal("expected symlink config rejection")
	}
}

func TestReadPVECanaryPreflightConfigRequiresPrivateExplicitNonCriticalDataDisk(t *testing.T) {
	valid := `{"apiVersion":"guard.storage-slo.io/v1alpha1","kind":"PVECanaryPreflightConfig","spec":{"domainKey":"reference-pool","node":"node-a","storage":"storage-a","zpool":"pool-a","workloadKind":"qemu","workloadId":"101","diskKey":"scsi1","requiredTags":["non-critical","pve-storage-guard"],"commandTimeoutSeconds":5,"envelope":{"minimumMiBPS":16,"maximumMiBPS":128,"rollbackMiBPS":32}}}`
	path := filepath.Join(t.TempDir(), "canary.json")
	if err := os.WriteFile(path, []byte(valid), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPVECanaryPreflightConfig(path); err != nil {
		t.Fatalf("read valid config: %v", err)
	}
	for name, payload := range map[string]string{
		"missing tag": strings.Replace(valid, `,"pve-storage-guard"`, "", 1),
		"boot-like disk is still syntactically allowed": strings.Replace(valid, `"scsi1"`, `"scsi0"`, 1),
		"workload injection":                            strings.Replace(valid, `"101"`, `"101;touch"`, 1),
		"rollback below floor":                          strings.Replace(valid, `"rollbackMiBPS":32`, `"rollbackMiBPS":8`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			candidate := filepath.Join(t.TempDir(), "canary.json")
			if err := os.WriteFile(candidate, []byte(payload), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := ReadPVECanaryPreflightConfig(candidate)
			if name == "boot-like disk is still syntactically allowed" {
				if err != nil {
					t.Fatalf("boot role requires live evidence, not key inference: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
