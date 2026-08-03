package pve

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	v1 "github.com/DjangoAILab/pve-storage-guard/api/v1"
	"github.com/DjangoAILab/pve-storage-guard/internal/config"
)

// CanaryPreflightReader performs fixed, read-only PVE lookups for one exact
// private enrollment. It has no actuator or arbitrary command surface.
type CanaryPreflightReader struct {
	config config.PVECanaryPreflightConfig
	runner commandRunner
}

// NewCanaryPreflightReader validates the private binding before any command.
func NewCanaryPreflightReader(document config.PVECanaryPreflightConfig) (*CanaryPreflightReader, error) {
	if err := document.Validate(); err != nil {
		return nil, err
	}
	return &CanaryPreflightReader{
		config: document,
		runner: localCommandRunner{timeout: time.Duration(document.Spec.CommandTimeoutSeconds) * time.Second},
	}, nil
}

// Assess returns an identity-free result and always denies active control.
func (r *CanaryPreflightReader) Assess(ctx context.Context) v1.PVECanaryPreflightAssessment {
	checks := v1.PVECanaryPreflightChecks{RollbackWithinBounds: true}
	request := commandRequest{Node: r.config.Spec.Node, Storage: r.config.Spec.Storage, ZPool: r.config.Spec.ZPool, WorkloadID: r.config.Spec.WorkloadID}
	if payload, err := r.runner.Run(ctx, opClusterStatus, request); err == nil {
		checks.ManagementHealthy, _ = parseClusterHealthy(payload, r.config.Spec.Node)
	}
	checks.StorageBound = r.storageBound(ctx, request)
	payload, err := r.runner.Run(ctx, opQEMUConfig, request)
	if err == nil {
		checks = r.assessQEMUConfig(payload, checks)
	}
	gaps := preflightGaps(checks)
	return v1.PVECanaryPreflightAssessment{
		SchemaVersion: v1.SchemaVersion, Kind: v1.PVECanaryPreflightAssessmentKind,
		ShadowOnly: true, RequestedMutations: 0, ControlledLoadEligible: len(gaps) == 0,
		ActiveControlEligible: false, Checks: checks, Gaps: gaps,
	}
}

func (r *CanaryPreflightReader) storageBound(ctx context.Context, request commandRequest) bool {
	configPayload, err := r.runner.Run(ctx, opStorageConfig, request)
	if err != nil {
		return false
	}
	binding, err := parseStorageConfig(configPayload)
	if err != nil || binding.StorageType != "zfspool" || validatePoolBinding(r.config.Spec.ZPool, binding.Pool) != nil {
		return false
	}
	statusPayload, err := r.runner.Run(ctx, opStorageStatus, request)
	if err != nil {
		return false
	}
	active, storageType, err := parseStorageActive(statusPayload)
	return err == nil && active && storageType == "zfspool"
}

func (r *CanaryPreflightReader) assessQEMUConfig(payload []byte, checks v1.PVECanaryPreflightChecks) v1.PVECanaryPreflightChecks {
	var document map[string]any
	if json.Unmarshal(payload, &document) != nil {
		return checks
	}
	checks.WorkloadUnlocked = strings.TrimSpace(stringField(document["lock"])) == ""
	tags := splitPVETags(stringField(document["tags"]))
	checks.ExplicitlyNonCritical = true
	for _, required := range r.config.Spec.RequiredTags {
		if _, exists := tags[required]; !exists {
			checks.ExplicitlyNonCritical = false
		}
	}
	disk, exists := document[r.config.Spec.DiskKey]
	diskValue := stringField(disk)
	checks.DiskExists = exists && diskValue != ""
	if !checks.DiskExists {
		return checks
	}
	parts := strings.Split(diskValue, ",")
	volume := strings.TrimSpace(parts[0])
	checks.DiskOnStorage = strings.HasPrefix(volume, r.config.Spec.Storage+":")
	checks.DiskIsData = !strings.Contains(strings.ToLower(volume), "cloudinit")
	checks.DiskIsWritable = true
	for _, option := range parts[1:] {
		key, value, found := strings.Cut(strings.TrimSpace(option), "=")
		if !found {
			continue
		}
		switch strings.ToLower(key) {
		case "media":
			if strings.EqualFold(value, "cdrom") {
				checks.DiskIsData = false
			}
		case "readonly", "ro":
			if value == "1" || strings.EqualFold(value, "true") {
				checks.DiskIsWritable = false
			}
		}
	}
	boot := stringField(document["boot"])
	checks.DiskIsNonBoot = boot != "" && !bootOrderContains(boot, r.config.Spec.DiskKey)
	return checks
}

func stringField(value any) string {
	text, _ := value.(string)
	return text
}

func splitPVETags(value string) map[string]struct{} {
	tags := make(map[string]struct{})
	for _, tag := range strings.FieldsFunc(value, func(r rune) bool { return r == ';' || r == ',' || r == ' ' }) {
		if normalized := strings.ToLower(strings.TrimSpace(tag)); normalized != "" {
			tags[normalized] = struct{}{}
		}
	}
	return tags
}

func bootOrderContains(value, diskKey string) bool {
	value = strings.TrimPrefix(value, "order=")
	for _, entry := range strings.Split(value, ";") {
		if strings.TrimSpace(entry) == diskKey {
			return true
		}
	}
	return false
}

func preflightGaps(checks v1.PVECanaryPreflightChecks) []string {
	ordered := []struct {
		name string
		ok   bool
	}{
		{"managementHealthy", checks.ManagementHealthy},
		{"storageBound", checks.StorageBound},
		{"explicitlyNonCritical", checks.ExplicitlyNonCritical},
		{"workloadUnlocked", checks.WorkloadUnlocked},
		{"diskExists", checks.DiskExists},
		{"diskOnStorage", checks.DiskOnStorage},
		{"diskIsData", checks.DiskIsData},
		{"diskIsNonBoot", checks.DiskIsNonBoot},
		{"diskIsWritable", checks.DiskIsWritable},
		{"rollbackWithinBounds", checks.RollbackWithinBounds},
	}
	gaps := make([]string, 0)
	for _, check := range ordered {
		if !check.ok {
			gaps = append(gaps, check.name)
		}
	}
	return gaps
}
