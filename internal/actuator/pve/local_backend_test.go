package pve

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

type recordingPveshExecutor struct {
	calls       [][]string
	responses   [][]byte
	err         error
	hadDeadline bool
}

type deadlineIgnoringExecutor struct{ delay time.Duration }

func (e deadlineIgnoringExecutor) run(context.Context, []string) ([]byte, error) {
	time.Sleep(e.delay)
	return qemuPayload(digestA, "bps_wr=33554432"), nil
}

func (e *recordingPveshExecutor) run(ctx context.Context, args []string) ([]byte, error) {
	_, e.hadDeadline = ctx.Deadline()
	e.calls = append(e.calls, slices.Clone(args))
	if e.err != nil {
		return nil, e.err
	}
	if len(e.responses) == 0 {
		return nil, nil
	}
	payload := slices.Clone(e.responses[0])
	e.responses = e.responses[1:]
	return payload, nil
}

func TestLocalBackendConstructionRunsNoCommand(t *testing.T) {
	backend, err := newLocalBackend(validActuatorConfig())
	if err != nil || backend == nil {
		t.Fatalf("backend=%v err=%v", backend, err)
	}
	if _, ok := backend.executor.(osPveshExecutor); !ok {
		t.Fatalf("unexpected executor %T", backend.executor)
	}

	document := validActuatorConfig()
	document.Spec.DiskKey = "unsafe"
	if _, err := newLocalBackend(document); err == nil {
		t.Fatal("expected invalid enrollment rejection")
	}
	if _, err := newLocalBackendWithExecutor(validActuatorConfig(), nil); err == nil {
		t.Fatal("expected nil executor rejection")
	}
}

func TestLocalBackendUsesExactReadAndUpdateCommands(t *testing.T) {
	executor := &recordingPveshExecutor{responses: [][]byte{qemuPayload(digestA, "bps_wr=33554432"), nil}}
	backend, err := newLocalBackendWithExecutor(validActuatorConfig(), executor)
	if err != nil {
		t.Fatal(err)
	}

	payload, err := backend.ReadQEMUConfig(context.Background(), "private-node", "101")
	if err != nil || string(payload) != string(qemuPayload(digestA, "bps_wr=33554432")) {
		t.Fatalf("payload=%q err=%v", payload, err)
	}
	diskValue := "private-storage:disk,cache=none,bps_wr=67108864,discard=on"
	if err := backend.UpdateQEMUDisk(
		context.Background(), "private-node", "101", "scsi1", diskValue, digestA,
	); err != nil {
		t.Fatal(err)
	}

	want := [][]string{
		{"get", "/nodes/private-node/qemu/101/config", "--output-format", "json"},
		{"set", "/nodes/private-node/qemu/101/config", "--digest", digestA, "--scsi1", diskValue},
	}
	if !slices.EqualFunc(executor.calls, want, slices.Equal[[]string]) {
		t.Fatalf("calls=%v want=%v", executor.calls, want)
	}
	if !executor.hadDeadline {
		t.Fatal("expected owner-configured command deadline")
	}
}

func TestLocalBackendCompletesActuatorApplyWithoutRuntimeWiring(t *testing.T) {
	executor := &recordingPveshExecutor{responses: [][]byte{
		qemuPayload(digestA, "cache=none,bps_wr=33554432"),
		nil,
		qemuPayload(digestB, "cache=none,bps_wr=67108864"),
	}}
	backend, err := newLocalBackendWithExecutor(validActuatorConfig(), executor)
	if err != nil {
		t.Fatal(err)
	}
	actuator, err := NewActuator(validActuatorConfig(), backend)
	if err != nil {
		t.Fatal(err)
	}
	effective, err := actuator.ApplyApproved(context.Background(), validApplyRequest(64))
	if err != nil || effective.ResourceKey != "resource-a" || effective.WriteLimitMiBPS != 64 {
		t.Fatalf("effective=%+v err=%v", effective, err)
	}
	if len(executor.calls) != 3 || executor.calls[0][0] != "get" || executor.calls[1][0] != "set" || executor.calls[2][0] != "get" {
		t.Fatalf("unexpected apply sequence: %v", executor.calls)
	}
}

func TestLocalBackendRejectsBindingAndDiskDriftBeforeCommand(t *testing.T) {
	tests := map[string]func(*localBackend) error{
		"read node": func(b *localBackend) error {
			_, err := b.ReadQEMUConfig(context.Background(), "other-node", "101")
			return err
		},
		"read workload": func(b *localBackend) error {
			_, err := b.ReadQEMUConfig(context.Background(), "private-node", "102")
			return err
		},
		"update node": func(b *localBackend) error {
			return b.UpdateQEMUDisk(context.Background(), "other-node", "101", "scsi1", validBackendDisk(32), digestA)
		},
		"update workload": func(b *localBackend) error {
			return b.UpdateQEMUDisk(context.Background(), "private-node", "102", "scsi1", validBackendDisk(32), digestA)
		},
		"update disk": func(b *localBackend) error {
			return b.UpdateQEMUDisk(context.Background(), "private-node", "101", "virtio1", validBackendDisk(32), digestA)
		},
		"update digest": func(b *localBackend) error {
			return b.UpdateQEMUDisk(context.Background(), "private-node", "101", "scsi1", validBackendDisk(32), "not-a-digest")
		},
		"wrong storage": func(b *localBackend) error {
			return b.UpdateQEMUDisk(context.Background(), "private-node", "101", "scsi1", "other-storage:disk,bps_wr=33554432", digestA)
		},
		"conflicting limiter": func(b *localBackend) error {
			return b.UpdateQEMUDisk(context.Background(), "private-node", "101", "scsi1", validBackendDisk(32)+",iops_wr=100", digestA)
		},
		"below envelope": func(b *localBackend) error {
			return b.UpdateQEMUDisk(context.Background(), "private-node", "101", "scsi1", validBackendDisk(8), digestA)
		},
		"above envelope": func(b *localBackend) error {
			return b.UpdateQEMUDisk(context.Background(), "private-node", "101", "scsi1", validBackendDisk(256), digestA)
		},
	}
	for name, invoke := range tests {
		t.Run(name, func(t *testing.T) {
			executor := &recordingPveshExecutor{}
			backend, err := newLocalBackendWithExecutor(validActuatorConfig(), executor)
			if err != nil {
				t.Fatal(err)
			}
			if err := invoke(backend); err == nil {
				t.Fatal("expected rejection")
			}
			if len(executor.calls) != 0 {
				t.Fatalf("invalid request executed command: %v", executor.calls)
			}
		})
	}
}

func TestLocalBackendDoesNotRetryOrLeakCommandErrors(t *testing.T) {
	executor := &recordingPveshExecutor{err: errors.New("private-node token secret")}
	backend, err := newLocalBackendWithExecutor(validActuatorConfig(), executor)
	if err != nil {
		t.Fatal(err)
	}
	err = backend.UpdateQEMUDisk(
		context.Background(), "private-node", "101", "scsi1", validBackendDisk(32), digestA,
	)
	if err == nil || err.Error() != "PVE backend update failed" || strings.Contains(err.Error(), "private") {
		t.Fatalf("unsafe error: %v", err)
	}
	if len(executor.calls) != 1 {
		t.Fatalf("ambiguous update was retried: calls=%d", len(executor.calls))
	}

	executor.calls = nil
	_, err = backend.ReadQEMUConfig(context.Background(), "private-node", "101")
	if err == nil || err.Error() != "PVE backend configuration read failed" || len(executor.calls) != 1 {
		t.Fatalf("err=%v calls=%d", err, len(executor.calls))
	}
}

func TestLocalBackendFailsClosedIfExecutorReturnsAfterDeadline(t *testing.T) {
	backend, err := newLocalBackendWithExecutor(
		validActuatorConfig(), deadlineIgnoringExecutor{delay: 40 * time.Millisecond},
	)
	if err != nil {
		t.Fatal(err)
	}
	backend.binding.commandTimeout = 20 * time.Millisecond
	if _, err := backend.ReadQEMUConfig(context.Background(), "private-node", "101"); err == nil {
		t.Fatal("expected expired command result rejection")
	}
}

func TestFixedPveshHelper(_ *testing.T) {
	separator := slices.Index(os.Args, "--")
	if separator < 0 || separator+1 >= len(os.Args) {
		return
	}
	switch os.Args[separator+1] {
	case "sleep":
		time.Sleep(30 * time.Second)
	case "stdout":
		_, _ = fmt.Fprint(os.Stdout, "fixed-output")
	case "overflow":
		chunk := strings.Repeat("x", 64*1024)
		for written := 0; written <= maxPveshOutput; written += len(chunk) {
			_, _ = fmt.Fprint(os.Stdout, chunk)
		}
	case "stderr-overflow":
		chunk := strings.Repeat("x", 16*1024)
		for written := 0; written <= maxPveshErrorOutput; written += len(chunk) {
			_, _ = fmt.Fprint(os.Stderr, chunk)
		}
	case "stderr":
		_, _ = fmt.Fprint(os.Stderr, "private-node token secret")
		os.Exit(23)
	case "environment":
		_, _ = fmt.Fprintf(
			os.Stdout, "%s|%s|%s|%s|%s",
			os.Getenv("LC_ALL"), os.Getenv("LANG"), os.Getenv("TZ"), os.Getenv("HOME"), os.Getenv("PATH"),
		)
	default:
		os.Exit(24)
	}
}

func TestRunFixedPveshBoundsEnvironmentOutputAndErrors(t *testing.T) {
	payload, err := runFixedPvesh(context.Background(), os.Args[0], fixedPveshHelperArgs("stdout"))
	if err != nil || !strings.HasPrefix(string(payload), "fixed-output") {
		t.Fatalf("payload=%q err=%v", payload, err)
	}
	payload, err = runFixedPvesh(context.Background(), os.Args[0], fixedPveshHelperArgs("environment"))
	if err != nil || !strings.HasPrefix(string(payload), "C|C|UTC||/usr/sbin:/usr/bin:/sbin:/bin") {
		t.Fatalf("environment=%q err=%v", payload, err)
	}

	_, err = runFixedPvesh(context.Background(), os.Args[0], fixedPveshHelperArgs("stderr"))
	if err == nil || err.Error() != "PVE backend command failed" || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unsafe error: %v", err)
	}
	_, err = runFixedPvesh(context.Background(), os.Args[0], fixedPveshHelperArgs("overflow"))
	if err == nil || err.Error() != "PVE backend command output exceeded safety limit" {
		t.Fatalf("overflow error: %v", err)
	}
	_, err = runFixedPvesh(context.Background(), os.Args[0], fixedPveshHelperArgs("stderr-overflow"))
	if err == nil || err.Error() != "PVE backend command output exceeded safety limit" {
		t.Fatalf("stderr overflow error: %v", err)
	}
}

func TestRunFixedPveshCancelsAndReapsProcess(t *testing.T) {
	for name, test := range map[string]struct {
		setup   func() (context.Context, context.CancelFunc)
		message string
	}{
		"cancel": {
			message: "cancelled",
			setup: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				time.AfterFunc(50*time.Millisecond, cancel)
				return ctx, cancel
			},
		},
		"timeout": {
			message: "timed out",
			setup: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 50*time.Millisecond)
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := test.setup()
			defer cancel()
			started := time.Now()
			_, err := runFixedPvesh(ctx, os.Args[0], fixedPveshHelperArgs("sleep"))
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("unexpected error: %v", err)
			}
			if elapsed := time.Since(started); elapsed > 2*time.Second {
				t.Fatalf("child took %v to reap", elapsed)
			}
		})
	}
}

func validBackendDisk(limitMiBPS uint64) string {
	return fmt.Sprintf("private-storage:disk,bps_wr=%d", limitMiBPS*bytesPerMiB)
}

func fixedPveshHelperArgs(mode string) []string {
	return []string{"-test.run=^TestFixedPveshHelper$", "--", mode}
}
