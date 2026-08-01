// Package pve defines the read-only PVE adapter boundary.
package pve

import (
	"context"
	"time"

	v1 "github.com/DjangoAILab/pve-storage-guard/api/v1"
)

// InventoryDisk is an opaque mapping; user-facing names never become policy keys.
type InventoryDisk struct {
	ResourceKey string
	DomainKey   string
	Root        bool
	Critical    bool
}

// Reader is the observer-only v0.1 adapter contract.
type Reader interface {
	Inventory(context.Context) ([]InventoryDisk, error)
	Observe(context.Context, string, time.Time) (v1.Observation, error)
}
