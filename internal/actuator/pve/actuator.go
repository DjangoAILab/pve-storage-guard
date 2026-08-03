package pve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	v1 "github.com/DjangoAILab/pve-storage-guard/api/v1"
	"github.com/DjangoAILab/pve-storage-guard/internal/config"
	"github.com/DjangoAILab/pve-storage-guard/internal/safety"
)

const (
	bytesPerMiB     = uint64(1024 * 1024)
	maxConfigBytes  = 1024 * 1024
	maxDiskValueLen = 16 * 1024
)

var (
	digestPattern    = regexp.MustCompile(`^[0-9a-f]{40}$`)
	optionKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
)

// Actuator implements only an exact bps_wr update for one owner-configured
// QEMU data disk. Constructing it does not open a listener or execute a read.
type Actuator struct {
	binding binding
	backend Backend
}

type binding struct {
	domainKey      string
	resourceKey    string
	node           string
	storage        string
	workloadID     string
	diskKey        string
	requiredTags   []string
	minimumMiBPS   float64
	maximumMiBPS   float64
	rollbackMiBPS  float64
	commandTimeout time.Duration
}

type qemuState struct {
	digest string
	disk   diskConfig
}

type diskConfig struct {
	volume  string
	options []diskOption
	limit   uint64
}

type diskOption struct {
	key   string
	value string
	raw   string
}

var _ safety.Actuator = (*Actuator)(nil)

// NewActuator validates an exact private enrollment and injects the only
// privileged operations it may use. The repository provides no live backend.
func NewActuator(document config.PVECanaryPreflightConfig, backend Backend) (*Actuator, error) {
	if err := document.Validate(); err != nil || backend == nil {
		return nil, errors.New("PVE actuator binding is invalid")
	}
	return &Actuator{
		binding: binding{
			domainKey: document.Spec.DomainKey, resourceKey: document.Spec.ResourceKey,
			node: document.Spec.Node, storage: document.Spec.Storage,
			workloadID: document.Spec.WorkloadID, diskKey: document.Spec.DiskKey,
			requiredTags:   append([]string(nil), document.Spec.RequiredTags...),
			minimumMiBPS:   document.Spec.Envelope.MinimumMiBPS,
			maximumMiBPS:   document.Spec.Envelope.MaximumMiBPS,
			rollbackMiBPS:  document.Spec.Envelope.RollbackMiBPS,
			commandTimeout: time.Duration(document.Spec.CommandTimeoutSeconds) * time.Second,
		},
		backend: backend,
	}, nil
}

// ReadEffective returns the exact existing bps_wr value after revalidating the
// live disk binding. An unlimited disk is not a safe bootstrap state.
func (a *Actuator) ReadEffective(ctx context.Context, resourceKey string) (EffectiveLimit, error) {
	if resourceKey != a.binding.resourceKey {
		return EffectiveLimit{}, errors.New("PVE actuator resource is not enrolled")
	}
	state, err := a.readState(ctx)
	if err != nil {
		return EffectiveLimit{}, err
	}
	return EffectiveLimit{ResourceKey: resourceKey, WriteLimitMiBPS: float64(state.disk.limit) / float64(bytesPerMiB)}, nil
}

// ApplyApproved changes only bps_wr, fences the update with the current PVE
// digest, and then reads the full binding back. Lease and approval authority
// are verified by safety.Gate before this method is called.
func (a *Actuator) ApplyApproved(ctx context.Context, request v1.ApplyRequest) (EffectiveLimit, error) {
	limit, err := a.validateRequest(request)
	if err != nil {
		return EffectiveLimit{}, err
	}
	before, err := a.readState(ctx)
	if err != nil {
		return EffectiveLimit{}, err
	}
	if before.disk.limit == limit {
		return EffectiveLimit{ResourceKey: a.binding.resourceKey, WriteLimitMiBPS: request.WriteLimitMiBPS}, nil
	}
	updated := before.disk.withLimit(limit)
	updateCtx, cancel := context.WithTimeout(ctx, a.binding.commandTimeout)
	err = a.backend.UpdateQEMUDisk(updateCtx, a.binding.node, a.binding.workloadID, a.binding.diskKey, updated, before.digest)
	cancel()
	if err != nil {
		return EffectiveLimit{}, errors.New("PVE actuator update failed")
	}
	after, err := a.readState(ctx)
	if err != nil {
		return EffectiveLimit{}, errors.New("PVE actuator read-back failed")
	}
	if before.disk.volume != after.disk.volume || !reflect.DeepEqual(before.disk.nonManagedOptions(), after.disk.nonManagedOptions()) {
		return EffectiveLimit{}, errors.New("PVE actuator read-back binding changed")
	}
	return EffectiveLimit{
		ResourceKey:     a.binding.resourceKey,
		WriteLimitMiBPS: float64(after.disk.limit) / float64(bytesPerMiB),
	}, nil
}

// RollbackLimitMiBPS exposes the reviewed static recovery target to an
// operator-owned rollback workflow. It does not apply or schedule a rollback.
func (a *Actuator) RollbackLimitMiBPS() float64 { return a.binding.rollbackMiBPS }

func (a *Actuator) validateRequest(request v1.ApplyRequest) (uint64, error) {
	if request.SchemaVersion != v1.SchemaVersion || request.ProposalID == "" || request.ApprovalID == "" ||
		request.PolicyVersion == "" || request.DomainKey != a.binding.domainKey || request.ResourceKey != a.binding.resourceKey ||
		request.LeaseHolderID == "" || request.LeaseGeneration == 0 || request.ExpiresAt.IsZero() ||
		math.IsNaN(request.WriteLimitMiBPS) || math.IsInf(request.WriteLimitMiBPS, 0) ||
		request.WriteLimitMiBPS < a.binding.minimumMiBPS || request.WriteLimitMiBPS > a.binding.maximumMiBPS {
		return 0, errors.New("PVE actuator request is invalid")
	}
	bytes := request.WriteLimitMiBPS * float64(bytesPerMiB)
	if bytes <= 0 || bytes > float64(math.MaxUint64) || math.Abs(bytes-math.Round(bytes)) > 1e-6 {
		return 0, errors.New("PVE actuator limit is not an exact byte rate")
	}
	return uint64(math.Round(bytes)), nil
}

func (a *Actuator) readState(ctx context.Context) (qemuState, error) {
	readCtx, cancel := context.WithTimeout(ctx, a.binding.commandTimeout)
	payload, err := a.backend.ReadQEMUConfig(readCtx, a.binding.node, a.binding.workloadID)
	cancel()
	if err != nil {
		return qemuState{}, errors.New("PVE actuator configuration read failed")
	}
	return parseQEMUState(payload, a.binding)
}

func parseQEMUState(payload []byte, binding binding) (qemuState, error) {
	if len(payload) == 0 || len(payload) > maxConfigBytes {
		return qemuState{}, errors.New("PVE actuator configuration payload is invalid")
	}
	document, err := decodeUniqueObject(payload)
	if err != nil {
		return qemuState{}, errors.New("PVE actuator configuration payload is invalid")
	}
	digest, ok := strictString(document, "digest")
	if !ok || !digestPattern.MatchString(digest) {
		return qemuState{}, errors.New("PVE actuator configuration digest is invalid")
	}
	if lock, present := document["lock"]; present {
		var value string
		if json.Unmarshal(lock, &value) != nil || strings.TrimSpace(value) != "" {
			return qemuState{}, errors.New("PVE actuator workload is locked")
		}
	}
	tags, ok := strictString(document, "tags")
	if !ok || !containsRequiredTags(tags, binding.requiredTags) {
		return qemuState{}, errors.New("PVE actuator workload classification is invalid")
	}
	boot, ok := strictString(document, "boot")
	if !ok || strings.TrimSpace(boot) == "" || bootOrderContains(boot, binding.diskKey) {
		return qemuState{}, errors.New("PVE actuator disk boot role is invalid")
	}
	diskValue, ok := strictString(document, binding.diskKey)
	if !ok {
		return qemuState{}, errors.New("PVE actuator disk binding is missing")
	}
	disk, err := parseDiskConfig(diskValue, binding.storage)
	if err != nil {
		return qemuState{}, err
	}
	limitMiBPS := float64(disk.limit) / float64(bytesPerMiB)
	if limitMiBPS < binding.minimumMiBPS || limitMiBPS > binding.maximumMiBPS {
		return qemuState{}, errors.New("PVE actuator effective limit is outside the envelope")
	}
	return qemuState{digest: digest, disk: disk}, nil
}

func decodeUniqueObject(payload []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return nil, errors.New("object required")
	}
	document := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok {
			return nil, errors.New("object key is invalid")
		}
		if _, duplicate := document[key]; duplicate {
			return nil, errors.New("object key is duplicated")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, errors.New("object value is invalid")
		}
		document[key] = value
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return nil, errors.New("object is incomplete")
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing data is invalid")
	}
	return document, nil
}

func strictString(document map[string]json.RawMessage, key string) (string, bool) {
	raw, ok := document[key]
	if !ok {
		return "", false
	}
	var value string
	if json.Unmarshal(raw, &value) != nil || value == "" || strings.ContainsAny(value, "\r\n\x00") {
		return "", false
	}
	return value, true
}

func parseDiskConfig(value, storage string) (diskConfig, error) {
	if value == "" || len(value) > maxDiskValueLen || strings.ContainsAny(value, "\r\n\x00") {
		return diskConfig{}, errors.New("PVE actuator disk configuration is invalid")
	}
	parts := strings.Split(value, ",")
	volume := strings.TrimSpace(parts[0])
	lowerVolume := strings.ToLower(volume)
	if !strings.HasPrefix(volume, storage+":") || strings.Contains(lowerVolume, "cloudinit") || strings.ContainsAny(volume, " \t") {
		return diskConfig{}, errors.New("PVE actuator disk storage binding is invalid")
	}
	disk := diskConfig{volume: volume, options: make([]diskOption, 0, len(parts)-1)}
	seen := make(map[string]struct{}, len(parts)-1)
	for _, part := range parts[1:] {
		raw := strings.TrimSpace(part)
		key, optionValue, found := strings.Cut(raw, "=")
		key = strings.TrimSpace(key)
		optionValue = strings.TrimSpace(optionValue)
		if !found || !optionKeyPattern.MatchString(key) || optionValue == "" || raw != key+"="+optionValue || strings.ContainsAny(optionValue, "\t") {
			return diskConfig{}, errors.New("PVE actuator disk option is invalid")
		}
		if _, duplicate := seen[key]; duplicate {
			return diskConfig{}, errors.New("PVE actuator disk option is duplicated")
		}
		seen[key] = struct{}{}
		if isRateKey(key) && key != "bps_wr" {
			return diskConfig{}, errors.New("PVE actuator disk has a conflicting rate option")
		}
		switch key {
		case "media":
			if optionValue != "disk" {
				return diskConfig{}, errors.New("PVE actuator disk is not a data disk")
			}
		case "readonly", "ro":
			if optionValue != "0" && optionValue != "false" {
				return diskConfig{}, errors.New("PVE actuator disk is read-only")
			}
		case "bps_wr":
			limit, err := strconv.ParseUint(optionValue, 10, 64)
			if err != nil || limit == 0 {
				return diskConfig{}, errors.New("PVE actuator write limit is invalid")
			}
			disk.limit = limit
		}
		disk.options = append(disk.options, diskOption{key: key, value: optionValue, raw: raw})
	}
	if disk.limit == 0 {
		return diskConfig{}, errors.New("PVE actuator requires an existing write limit")
	}
	return disk, nil
}

func (d diskConfig) withLimit(limit uint64) string {
	parts := make([]string, 1, len(d.options)+1)
	parts[0] = d.volume
	for _, option := range d.options {
		if option.key == "bps_wr" {
			parts = append(parts, "bps_wr="+strconv.FormatUint(limit, 10))
		} else {
			parts = append(parts, option.raw)
		}
	}
	return strings.Join(parts, ",")
}

func (d diskConfig) nonManagedOptions() []diskOption {
	options := make([]diskOption, 0, len(d.options)-1)
	for _, option := range d.options {
		if option.key != "bps_wr" {
			option.raw = ""
			options = append(options, option)
		}
	}
	return options
}

func isRateKey(key string) bool {
	return strings.HasPrefix(key, "bps") || strings.HasPrefix(key, "mbps") || strings.HasPrefix(key, "iops")
}

func containsRequiredTags(value string, required []string) bool {
	tags := make(map[string]struct{})
	for _, tag := range strings.FieldsFunc(value, func(r rune) bool { return r == ';' || r == ',' || r == ' ' }) {
		tags[strings.ToLower(strings.TrimSpace(tag))] = struct{}{}
	}
	for _, tag := range required {
		if _, ok := tags[tag]; !ok {
			return false
		}
	}
	return true
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
