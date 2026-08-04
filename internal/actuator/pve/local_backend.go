package pve

import (
	"bytes"
	"context"
	"errors"
	"math"
	"os/exec"
	"slices"
	"time"

	"github.com/DjangoAILab/pve-storage-guard/internal/config"
)

const (
	pveshPath           = "/usr/bin/pvesh"
	maxPveshOutput      = 1 << 20
	maxPveshErrorOutput = 64 << 10
)

// localBackend is a dormant, enrollment-bound implementation of Backend. It
// has no exported command surface and is not registered by any runtime.
type localBackend struct {
	binding  binding
	executor pveshExecutor
}

type pveshExecutor interface {
	run(context.Context, []string) ([]byte, error)
}

type osPveshExecutor struct{}

var _ Backend = (*localBackend)(nil)

// newLocalBackend constructs the reviewed fixed-argv implementation without
// making it reachable from another package. Exporting or wiring this
// constructor is a separate production-capability checkpoint.
func newLocalBackend(document config.PVECanaryPreflightConfig) (*localBackend, error) {
	return newLocalBackendWithExecutor(document, osPveshExecutor{})
}

func newLocalBackendWithExecutor(document config.PVECanaryPreflightConfig, executor pveshExecutor) (*localBackend, error) {
	if err := document.Validate(); err != nil || executor == nil {
		return nil, errors.New("PVE backend binding is invalid")
	}
	return &localBackend{binding: bindingFromDocument(document), executor: executor}, nil
}

func (b *localBackend) ReadQEMUConfig(ctx context.Context, node, workloadID string) ([]byte, error) {
	if ctx == nil || node != b.binding.node || workloadID != b.binding.workloadID {
		return nil, errors.New("PVE backend read binding is invalid")
	}
	commandCtx, cancel := context.WithTimeout(ctx, b.binding.commandTimeout)
	defer cancel()
	payload, err := b.executor.run(commandCtx, []string{
		"get", b.configPath(), "--output-format", "json",
	})
	if err != nil || commandCtx.Err() != nil {
		return nil, errors.New("PVE backend configuration read failed")
	}
	return slices.Clone(payload), nil
}

func (b *localBackend) UpdateQEMUDisk(
	ctx context.Context,
	node, workloadID, diskKey, diskValue, digest string,
) error {
	if ctx == nil || node != b.binding.node || workloadID != b.binding.workloadID || diskKey != b.binding.diskKey ||
		!digestPattern.MatchString(digest) {
		return errors.New("PVE backend update binding is invalid")
	}
	disk, err := parseDiskConfig(diskValue, b.binding.storage)
	if err != nil {
		return errors.New("PVE backend disk value is invalid")
	}
	limitMiBPS := float64(disk.limit) / float64(bytesPerMiB)
	if math.IsNaN(limitMiBPS) || math.IsInf(limitMiBPS, 0) ||
		limitMiBPS < b.binding.minimumMiBPS || limitMiBPS > b.binding.maximumMiBPS {
		return errors.New("PVE backend disk value is outside the envelope")
	}

	commandCtx, cancel := context.WithTimeout(ctx, b.binding.commandTimeout)
	defer cancel()
	_, err = b.executor.run(commandCtx, []string{
		"set", b.configPath(), "--digest", digest, "--" + b.binding.diskKey, diskValue,
	})
	if err != nil || commandCtx.Err() != nil {
		return errors.New("PVE backend update failed")
	}
	return nil
}

func (b *localBackend) configPath() string {
	return "/nodes/" + b.binding.node + "/qemu/" + b.binding.workloadID + "/config"
}

func (osPveshExecutor) run(ctx context.Context, args []string) ([]byte, error) {
	return runFixedPvesh(ctx, pveshPath, args)
}

func runFixedPvesh(ctx context.Context, path string, args []string) ([]byte, error) {
	if ctx == nil {
		return nil, errors.New("PVE backend context is invalid")
	}
	cmd := exec.CommandContext(ctx, path, slices.Clone(args)...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL=C", "LANG=C", "TZ=UTC"}
	cmd.Dir = "/"
	cmd.WaitDelay = time.Second
	stdout := &boundedCapture{limit: maxPveshOutput}
	stderr := &boundedCapture{limit: maxPveshErrorOutput}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, errors.New("PVE backend command timed out")
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, errors.New("PVE backend command was cancelled")
		}
		if stdout.exceeded || stderr.exceeded {
			return nil, errors.New("PVE backend command output exceeded safety limit")
		}
		return nil, errors.New("PVE backend command failed")
	}
	if stdout.exceeded || stderr.exceeded {
		return nil, errors.New("PVE backend command output exceeded safety limit")
	}
	return bytes.Clone(stdout.buffer.Bytes()), nil
}

type boundedCapture struct {
	buffer   bytes.Buffer
	limit    int
	exceeded bool
}

func (b *boundedCapture) Write(payload []byte) (int, error) {
	remaining := b.limit - b.buffer.Len()
	if remaining <= 0 {
		b.exceeded = true
		return len(payload), nil
	}
	if len(payload) > remaining {
		_, _ = b.buffer.Write(payload[:remaining])
		b.exceeded = true
		return len(payload), nil
	}
	return b.buffer.Write(payload)
}
