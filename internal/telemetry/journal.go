package telemetry

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"

	v1 "github.com/DjangoAILab/pve-storage-guard/api/v1"
)

var eventIDPattern = regexp.MustCompile(`^event-[0-9a-f]{24}$`)

// Journal is a single-writer, append-only JSONL decision journal.
type Journal struct {
	file *os.File
}

// OpenJournal opens an existing private regular file or atomically creates a
// new one with mode 0600. It rejects symlinks and group/other-accessible files.
func OpenJournal(path string) (*Journal, error) {
	if path == "" {
		return nil, errors.New("journal path is required")
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
		if validateErr := validateOpenedJournal(file, nil); validateErr != nil {
			_ = file.Close()
			return nil, validateErr
		}
		return &Journal{file: file}, nil
	}
	if err := validateJournalInfo(info); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return nil, fmt.Errorf("open journal: %w", err)
	}
	if err := validateOpenedJournal(file, info); err != nil {
		_ = file.Close()
		return nil, err
	}
	return &Journal{file: file}, nil
}

func validateOpenedJournal(file *os.File, expected os.FileInfo) error {
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat opened journal: %w", err)
	}
	if expected != nil && !os.SameFile(expected, info) {
		return errors.New("journal target changed while opening")
	}
	return validateJournalInfo(info)
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
	err := journal.file.Close()
	journal.file = nil
	return err
}

func validateDecisionEvent(event v1.DecisionEvent) error {
	if event.SchemaVersion != v1.SchemaVersion || event.EventType != v1.DecisionEventType || !eventIDPattern.MatchString(event.EventID) {
		return errors.New("decision event envelope is invalid")
	}
	if event.RecordedAt.IsZero() || event.DomainKey == "" || event.PolicyVersion == "" || event.Mode != "shadow" || event.Outcome != v1.DecisionOutcomeShadowEvaluated {
		return errors.New("decision event identity or outcome is invalid")
	}
	if event.Observation.ID == "" || event.Observation.ObservedAt.IsZero() || !finite(event.Observation.AgeSeconds) || !finiteNonNegative(event.Observation.WriteWaitP95Milliseconds) {
		return errors.New("decision event observation is invalid")
	}
	if event.Decision.ProposalID == "" || event.Decision.Reason == "" || !finiteNonNegative(event.Decision.PreviousBudgetMiBPS) || !finiteNonNegative(event.Decision.DesiredBudgetMiBPS) || len(event.Decision.Allocations) == 0 {
		return errors.New("decision event decision is invalid")
	}
	for resourceKey, allocation := range event.Decision.Allocations {
		if resourceKey == "" || !finiteNonNegative(allocation) {
			return errors.New("decision event allocation is invalid")
		}
	}
	if event.Safety.ActuationAllowed || event.Safety.AllocationFeasible != event.Decision.AllocationFeasible || event.Safety.LeaseStatus != v1.SafetyNotEvaluated || event.Safety.ApprovalStatus != v1.SafetyNotEvaluated || event.Safety.EffectiveStateStatus != v1.SafetyNotEvaluated {
		return errors.New("decision event shadow safety result is invalid")
	}
	return nil
}
