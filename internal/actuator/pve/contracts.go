// Package pve defines the deliberately narrow future PVE actuation boundary.
package pve

import (
	"context"

	v1 "github.com/DjangoAILab/pve-storage-guard/api/v1"
)

// EffectiveLimit is read back after any approved apply operation.
type EffectiveLimit struct {
	ResourceKey     string
	WriteLimitMiBPS float64
}

// Actuator cannot accept arbitrary commands or VM lifecycle operations.
type Actuator interface {
	ReadEffective(context.Context, string) (EffectiveLimit, error)
	ApplyApproved(context.Context, v1.ApplyRequest) (EffectiveLimit, error)
}
