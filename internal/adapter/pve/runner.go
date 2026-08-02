package pve

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

const maxCommandOutput = 1 << 20

type operation uint8

const (
	opClusterStatus operation = iota + 1
	opStorageConfig
	opStorageStatus
	opZFSWaitHistogramLayout
	opZFSWaitHistogram
)

func (o operation) String() string {
	switch o {
	case opClusterStatus:
		return "cluster-status"
	case opStorageConfig:
		return "storage-config"
	case opStorageStatus:
		return "storage-status"
	case opZFSWaitHistogramLayout:
		return "zfs-wait-histogram-layout"
	case opZFSWaitHistogram:
		return "zfs-wait-histogram"
	default:
		return "unknown-operation"
	}
}

type commandRequest struct {
	Node            string
	Storage         string
	ZPool           string
	IntervalSeconds int
}

type commandRunner interface {
	Run(context.Context, operation, commandRequest) ([]byte, error)
}

type localCommandRunner struct {
	timeout time.Duration
}

func (r localCommandRunner) Run(parent context.Context, op operation, request commandRequest) ([]byte, error) {
	path, args, err := commandSpec(op, request)
	if err != nil {
		return nil, err
	}
	return runBoundedCommand(parent, r.timeout, op, path, args)
}

func runBoundedCommand(parent context.Context, timeout time.Duration, op operation, path string, args []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL=C", "LANG=C", "TZ=UTC"}
	cmd.WaitDelay = time.Second
	stdout := &boundedBuffer{limit: maxCommandOutput}
	stderr := &boundedBuffer{limit: 64 << 10}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("%s timed out", op)
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, fmt.Errorf("%s cancelled: %w", op, context.Canceled)
		}
		if stdout.exceeded || stderr.exceeded {
			return nil, fmt.Errorf("%s output exceeded safety limit", op)
		}
		return nil, fmt.Errorf("%s failed", op)
	}
	if stdout.exceeded {
		return nil, fmt.Errorf("%s output exceeded safety limit", op)
	}
	return stdout.Bytes(), nil
}

func commandSpec(op operation, request commandRequest) (string, []string, error) {
	switch op {
	case opClusterStatus:
		return "/usr/bin/pvesh", []string{"get", "/cluster/status", "--output-format", "json"}, nil
	case opStorageConfig:
		return "/usr/bin/pvesh", []string{"get", "/storage/" + request.Storage, "--output-format", "json"}, nil
	case opStorageStatus:
		return "/usr/bin/pvesh", []string{"get", "/nodes/" + request.Node + "/storage/" + request.Storage + "/status", "--output-format", "json"}, nil
	case opZFSWaitHistogramLayout:
		return "/usr/sbin/zpool", []string{"iostat", "-w", request.ZPool}, nil
	case opZFSWaitHistogram:
		if request.IntervalSeconds < 1 || request.IntervalSeconds > 60 {
			return "", nil, errors.New("invalid histogram interval")
		}
		return "/usr/sbin/zpool", []string{"iostat", "-wpH", "-y", request.ZPool, strconv.Itoa(request.IntervalSeconds), "1"}, nil
	default:
		return "", nil, errors.New("unsupported local operation")
	}
}

type boundedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	exceeded bool
}

func (b *boundedBuffer) Write(payload []byte) (int, error) {
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

func (b *boundedBuffer) Bytes() []byte { return b.buffer.Bytes() }
