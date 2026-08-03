package pve

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestBoundedCommandHelper(_ *testing.T) {
	separator := -1
	for index, argument := range os.Args {
		if argument == "--" {
			separator = index
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		return
	}
	switch os.Args[separator+1] {
	case "sleep":
		time.Sleep(30 * time.Second)
	case "stdout":
		chunk := strings.Repeat("x", 64*1024)
		for written := 0; written <= maxCommandOutput; written += len(chunk) {
			_, _ = fmt.Fprint(os.Stdout, chunk)
		}
	case "stderr":
		_, _ = fmt.Fprintln(os.Stderr, "private-node private-pool")
		os.Exit(23)
	default:
		os.Exit(24)
	}
}

func TestRunBoundedCommandCancelsAndReapsChild(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	started := time.Now()
	_, err := runBoundedCommand(ctx, 5*time.Second, opClusterStatus, os.Args[0], helperArgs("sleep"))
	if err == nil || !strings.Contains(err.Error(), "cluster-status cancelled") {
		t.Fatalf("unexpected cancellation error: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("cancelled child took %v to reap", elapsed)
	}
}

func TestRunBoundedCommandTimesOutAndReapsChild(t *testing.T) {
	started := time.Now()
	_, err := runBoundedCommand(context.Background(), 50*time.Millisecond, opStorageStatus, os.Args[0], helperArgs("sleep"))
	if err == nil || !strings.Contains(err.Error(), "storage-status timed out") {
		t.Fatalf("unexpected timeout error: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("timed-out child took %v to reap", elapsed)
	}
}

func TestRunBoundedCommandRejectsOutputOverflow(t *testing.T) {
	_, err := runBoundedCommand(context.Background(), 5*time.Second, opZFSWaitHistogram, os.Args[0], helperArgs("stdout"))
	if err == nil || err.Error() != "zfs-wait-histogram output exceeded safety limit" {
		t.Fatalf("unexpected overflow error: %v", err)
	}
}

func TestRunBoundedCommandDoesNotLeakFailedCommandOutput(t *testing.T) {
	_, err := runBoundedCommand(context.Background(), 5*time.Second, opStorageConfig, os.Args[0], helperArgs("stderr"))
	if err == nil || err.Error() != "storage-config failed" || strings.Contains(err.Error(), "private") {
		t.Fatalf("unexpected command error: %v", err)
	}
}

func TestQEMUConfigCommandIsExactAndRejectsInjection(t *testing.T) {
	path, args, err := commandSpec(opQEMUConfig, commandRequest{Node: "node-a", WorkloadID: "101"})
	if err != nil || path != "/usr/bin/pvesh" || strings.Join(args, " ") != "get /nodes/node-a/qemu/101/config --output-format json" {
		t.Fatalf("path=%q args=%v err=%v", path, args, err)
	}
	for _, workloadID := range []string{"", "0;touch", "../101", "1234567890"} {
		if _, _, err := commandSpec(opQEMUConfig, commandRequest{Node: "node-a", WorkloadID: workloadID}); err == nil {
			t.Fatalf("accepted unsafe workload id %q", workloadID)
		}
	}
}

func helperArgs(mode string) []string {
	return []string{"-test.run=^TestBoundedCommandHelper$", "--", mode}
}
