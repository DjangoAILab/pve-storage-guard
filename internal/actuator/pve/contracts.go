// Package pve defines the deliberately narrow future PVE actuation boundary.
package pve

import (
	"github.com/DjangoAILab/pve-storage-guard/internal/safety"
)

// EffectiveLimit is the generic safety boundary's read-back state.
type EffectiveLimit = safety.EffectiveLimit

// Actuator cannot accept arbitrary commands or VM lifecycle operations. The
// PVE implementation must satisfy the platform-neutral safety contract.
type Actuator = safety.Actuator
