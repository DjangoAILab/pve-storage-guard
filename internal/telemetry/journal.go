package telemetry

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"syscall"

	v1 "github.com/DjangoAILab/pve-storage-guard/api/v1"
)

const defaultJournalMaxBytes int64 = 256 * 1024 * 1024

const decisionEventAgeToleranceSeconds = 0.002

var (
	eventIDPattern    = regexp.MustCompile(`^event-[0-9a-f]{24}$`)
	proposalIDPattern = regexp.MustCompile(`^proposal-[0-9a-f]{24}$`)
)

// Journal is a single-writer, append-only JSONL decision journal.
type Journal struct {
	file     *os.File
	maxBytes int64
}

// OpenJournal opens an existing private regular file or atomically creates a
// new one with mode 0600. It rejects symlinks and group/other-accessible files.
func OpenJournal(path string) (*Journal, error) {
	return openJournal(path, defaultJournalMaxBytes)
}

func openJournal(path string, maxBytes int64) (*Journal, error) {
	if path == "" {
		return nil, errors.New("journal path is required")
	}
	if maxBytes <= 0 {
		return nil, errors.New("journal size limit must be positive")
	}
	info, err := os.Lstat(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect journal: %w", err)
		}
		file, createErr := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE|os.O_EXCL, 0o600)
		if createErr != nil {
			return nil, fmt.Errorf("create journal: %w", createErr)
		}
		if validateErr := validateOpenedJournal(file, nil, maxBytes); validateErr != nil {
			_ = file.Close()
			return nil, validateErr
		}
		if lockErr := lockJournal(file); lockErr != nil {
			_ = file.Close()
			return nil, lockErr
		}
		return &Journal{file: file, maxBytes: maxBytes}, nil
	}
	if err := validateJournalInfo(info); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return nil, fmt.Errorf("open journal: %w", err)
	}
	if err := validateOpenedJournal(file, info, maxBytes); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := lockJournal(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	return &Journal{file: file, maxBytes: maxBytes}, nil
}

func validateOpenedJournal(file *os.File, expected os.FileInfo, maxBytes int64) error {
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat opened journal: %w", err)
	}
	if expected != nil && !os.SameFile(expected, info) {
		return errors.New("journal target changed while opening")
	}
	if err := validateJournalInfo(info); err != nil {
		return err
	}
	if info.Size() > maxBytes {
		return errors.New("existing journal exceeds the size limit")
	}
	return nil
}

func lockJournal(file *os.File) error {
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("lock journal for a single writer: %w", err)
	}
	return nil
}

func validateJournalInfo(info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return errors.New("journal must be a regular file and not a symlink")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return errors.New("existing journal must not be accessible by group or other")
	}
	return nil
}

// Append validates, writes, and syncs one complete event before returning.
func (journal *Journal) Append(event v1.DecisionEvent) error {
	if journal == nil || journal.file == nil {
		return errors.New("journal is closed")
	}
	if err := validateDecisionEvent(event); err != nil {
		return err
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode decision event: %w", err)
	}
	payload = append(payload, '\n')
	info, err := journal.file.Stat()
	if err != nil {
		return fmt.Errorf("stat journal before append: %w", err)
	}
	if info.Size() > journal.maxBytes-int64(len(payload)) {
		return errors.New("journal size limit reached; rotate it before continuing")
	}
	written, err := journal.file.Write(payload)
	if err != nil {
		return fmt.Errorf("append decision event: %w", err)
	}
	if written != len(payload) {
		return fmt.Errorf("append decision event: %w", io.ErrShortWrite)
	}
	if err := journal.file.Sync(); err != nil {
		return fmt.Errorf("sync decision event: %w", err)
	}
	return nil
}

// Close closes the journal. Every successful Append has already been synced.
func (journal *Journal) Close() error {
	if journal == nil || journal.file == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(journal.file.Fd()), syscall.LOCK_UN)
	closeErr := journal.file.Close()
	journal.file = nil
	if unlockErr != nil {
		return fmt.Errorf("unlock journal: %w", unlockErr)
	}
	return closeErr
}

func validateDecisionEvent(event v1.DecisionEvent) error {
	if event.SchemaVersion != v1.SchemaVersion || event.EventType != v1.DecisionEventType || !eventIDPattern.MatchString(event.EventID) {
		return errors.New("decision event envelope is invalid")
	}
	if event.RecordedAt.IsZero() || !validString(event.DomainKey, 256) || !validString(event.PolicyVersion, 128) || event.Mode != "shadow" || event.Outcome != v1.DecisionOutcomeShadowEvaluated {
		return errors.New("decision event identity or outcome is invalid")
	}
	if !validString(event.Observation.ID, 256) || event.Observation.ObservedAt.IsZero() || !finite(event.Observation.AgeSeconds) || !finiteNonNegative(event.Observation.WriteWaitP95Milliseconds) {
		return errors.New("decision event observation is invalid")
	}
	if evidence := event.Observation.WaitEvidence; evidence != nil {
		if !event.Observation.WaitValid || evidence.MeasurementLayer != "storage-domain" || evidence.Statistic != "p95-upper-bound" || evidence.Source != "openzfs-total-wait-histogram" || evidence.Provenance != "observed" || evidence.SampleIntervalSeconds < 1 || evidence.SampleIntervalSeconds > 60 || !finite(evidence.SampleWeight) || evidence.SampleWeight <= 0 || evidence.BucketUpperBoundNanoseconds == 0 {
			return errors.New("decision event wait evidence is invalid")
		}
		expectedWait := float64(evidence.BucketUpperBoundNanoseconds) / 1_000_000
		if math.Abs(expectedWait-event.Observation.WriteWaitP95Milliseconds) > 0.000001 {
			return errors.New("decision event wait evidence is inconsistent")
		}
	}
	if pressure := event.Observation.IOPressure; pressure != nil {
		if !finite(pressure.SomeAvg10) || !finite(pressure.FullAvg10) || pressure.SomeAvg10 < 0 || pressure.SomeAvg10 > 100 || pressure.FullAvg10 < 0 || pressure.FullAvg10 > 100 {
			return errors.New("decision event PSI evidence is invalid")
		}
	}
	if len(event.Observation.DiskSignals) > 64 {
		return errors.New("decision event has too many disk signals")
	}
	diskKeys := make(map[string]struct{}, len(event.Observation.DiskSignals))
	for _, signal := range event.Observation.DiskSignals {
		if !validString(signal.ResourceKey, 63) {
			return errors.New("decision event disk signal is invalid")
		}
		if _, exists := diskKeys[signal.ResourceKey]; exists {
			return errors.New("decision event disk signal is duplicated")
		}
		diskKeys[signal.ResourceKey] = struct{}{}
	}
	if !proposalIDPattern.MatchString(event.Decision.ProposalID) || !validString(event.Decision.Reason, 128) || !finiteNonNegative(event.Decision.PreviousBudgetMiBPS) || !finiteNonNegative(event.Decision.DesiredBudgetMiBPS) || len(event.Decision.Allocations) == 0 {
		return errors.New("decision event decision is invalid")
	}
	if event.EventID != eventID(event.Decision.ProposalID) {
		return errors.New("decision event id does not match its proposal")
	}
	ageSeconds := event.RecordedAt.Sub(event.Observation.ObservedAt).Seconds()
	if !finite(ageSeconds) || math.Abs(ageSeconds-event.Observation.AgeSeconds) > decisionEventAgeToleranceSeconds {
		return errors.New("decision event observation age is inconsistent")
	}
	for resourceKey, allocation := range event.Decision.Allocations {
		if !validString(resourceKey, 256) || !finiteNonNegative(allocation) {
			return errors.New("decision event allocation is invalid")
		}
	}
	if event.Safety.ActuationAllowed || event.Safety.AllocationFeasible != event.Decision.AllocationFeasible || event.Safety.LeaseStatus != v1.SafetyNotEvaluated || event.Safety.ApprovalStatus != v1.SafetyNotEvaluated || event.Safety.EffectiveStateStatus != v1.SafetyNotEvaluated {
		return errors.New("decision event shadow safety result is invalid")
	}
	return nil
}

func validString(value string, maxLength int) bool {
	return value != "" && len(value) <= maxLength
}
