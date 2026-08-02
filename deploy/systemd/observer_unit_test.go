package systemd_test

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

func TestObserverUnitPreservesReadOnlyLeastPrivilegeContract(t *testing.T) {
	path := "pve-storage-guard-observer.service"
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()

	values := make(map[string][]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("invalid unit line %q", line)
		}
		values[key] = append(values[key], value)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}

	requireExact := map[string]string{
		"ConditionFileIsExecutable": "/usr/local/bin/pve-storage-guard",
		"Type":                      "simple",
		"User":                      "pve-storage-guard",
		"Group":                     "pve-storage-guard",
		"UMask":                     "0077",
		"ExecStartPre":              "/usr/local/bin/pve-storage-guard agent inventory --config /etc/pve-storage-guard/agent.json",
		"ExecStart":                 "/usr/local/bin/pve-storage-guard agent watch --config /etc/pve-storage-guard/agent.json --period 10s",
		"NoNewPrivileges":           "yes",
		"CapabilityBoundingSet":     "",
		"AmbientCapabilities":       "",
		"ProtectSystem":             "strict",
		"ProtectHome":               "yes",
		"PrivateNetwork":            "yes",
		"RestrictAddressFamilies":   "AF_UNIX",
		"IPAddressDeny":             "any",
		"DevicePolicy":              "closed",
		"DeviceAllow":               "/dev/zfs rw",
		"ProtectProc":               "invisible",
		"RestrictNamespaces":        "yes",
		"SystemCallArchitectures":   "native",
		"SystemCallFilter":          "@system-service",
		"Restart":                   "on-failure",
		"KillSignal":                "SIGTERM",
		"TimeoutStopSec":            "10s",
	}
	for key, expected := range requireExact {
		got := values[key]
		if len(got) != 1 || got[0] != expected {
			t.Errorf("%s=%q, want exactly %q", key, got, expected)
		}
	}

	for _, forbidden := range []string{"RootDirectory", "RootImage", "ReadWritePaths", "BindPaths", "NetworkNamespacePath"} {
		if _, exists := values[forbidden]; exists {
			t.Errorf("observer unit must not declare %s", forbidden)
		}
	}
	for _, directive := range append(values["ExecStartPre"], values["ExecStart"]...) {
		for _, forbidden := range []string{"/bin/sh", "/bin/bash", "sudo", "actuator", " apply "} {
			if strings.Contains(directive, forbidden) {
				t.Errorf("execution directive %q contains forbidden surface %q", directive, forbidden)
			}
		}
	}
}
