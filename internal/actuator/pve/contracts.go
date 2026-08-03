// Package pve defines the deliberately narrow PVE QEMU disk actuation core.
package pve

import (
	"context"

	"github.com/DjangoAILab/pve-storage-guard/internal/safety"
)

// EffectiveLimit is the generic safety boundary's read-back state.
type EffectiveLimit = safety.EffectiveLimit

// Backend is the complete privileged surface required by this package. An OS
// implementation is deliberately absent: this interface cannot execute an
// arbitrary command or perform a VM lifecycle operation.
type Backend interface {
	ReadQEMUConfig(ctx context.Context, node, workloadID string) ([]byte, error)
	UpdateQEMUDisk(ctx context.Context, node, workloadID, diskKey, diskValue, digest string) error
}
