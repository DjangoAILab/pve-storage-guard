package telemetry

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"

	v1 "github.com/DjangoAILab/pve-storage-guard/api/v1"
)

const defaultJournalEventMaxBytes = 1024 * 1024

// VerifyJournal validates one sealed private decision journal and returns an
// identity-free summary. It rejects an active writer and performs no mutation.
func VerifyJournal(path string) (v1.DecisionJournalVerification, error) {
	return verifyJournal(path, defaultJournalMaxBytes, defaultJournalEventMaxBytes)
}

func verifyJournal(path string, maxFileBytes int64, maxEventBytes int) (summary v1.DecisionJournalVerification, resultErr error) {
	summary = v1.DecisionJournalVerification{
		SchemaVersion: v1.SchemaVersion,
		Kind:          v1.DecisionJournalVerificationKind,
	}
	if path == "" {
		return summary, errors.New("journal path is required")
	}
	if maxFileBytes <= 0 || maxEventBytes <= 0 {
		return summary, errors.New("journal verification limits must be positive")
	}
	expected, err := os.Lstat(path)
	if err != nil {
		return summary, fmt.Errorf("inspect journal: %w", err)
	}
	if err := validateJournalInfo(expected); err != nil {
		return summary, err
	}
	file, err := os.Open(path)
	if err != nil {
		return summary, fmt.Errorf("open journal for verification: %w", err)
	}
	locked := false
	defer func() {
		var cleanupErr error
		if locked {
			if err := syscall.Flock(int(file.Fd()), syscall.LOCK_UN); err != nil {
				cleanupErr = fmt.Errorf("unlock verified journal: %w", err)
			}
		}
		if err := file.Close(); cleanupErr == nil && err != nil {
			cleanupErr = fmt.Errorf("close verified journal: %w", err)
		}
		if resultErr == nil && cleanupErr != nil {
			resultErr = cleanupErr
		}
	}()
	if err := validateOpenedJournal(file, expected, maxFileBytes); err != nil {
		return summary, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); err != nil {
		return summary, fmt.Errorf("lock sealed journal for verification: %w", err)
	}
	locked = true
	lockedInfo, err := file.Stat()
	if err != nil {
		return summary, fmt.Errorf("stat locked journal: %w", err)
	}
	current, err := os.Lstat(path)
	if err != nil {
		return summary, fmt.Errorf("reinspect locked journal: %w", err)
	}
	if err := validateJournalInfo(current); err != nil {
		return summary, err
	}
	if !os.SameFile(current, lockedInfo) {
		return summary, errors.New("journal target changed while locking for verification")
	}

	policyVersions := make(map[string]struct{})
	eventIDs := make(map[string]struct{})
	var domainKey string
	var previousRecordedAt time.Time
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxEventBytes)
	line := 0
	for scanner.Scan() {
		line++
		event, err := decodeDecisionEvent(scanner.Bytes())
		if err != nil {
			return summary, fmt.Errorf("decode journal event on line %d: %w", line, err)
		}
		if err := validateDecisionEvent(event); err != nil {
			return summary, fmt.Errorf("validate journal event on line %d: %w", line, err)
		}
		if domainKey == "" {
			domainKey = event.DomainKey
		} else if event.DomainKey != domainKey {
			return summary, errors.New("sealed journal contains multiple storage domains")
		}
		summary.EventCount++
		if event.Decision.Changed {
			summary.ChangedCount++
		}
		policyVersions[event.PolicyVersion] = struct{}{}
		if _, exists := eventIDs[event.EventID]; exists {
			summary.DuplicateEventCount++
		} else {
			eventIDs[event.EventID] = struct{}{}
		}
		if !previousRecordedAt.IsZero() && event.RecordedAt.Before(previousRecordedAt) {
			summary.TimestampRegressionCount++
		}
		previousRecordedAt = event.RecordedAt
		if summary.EarliestRecordedAt == nil || event.RecordedAt.Before(*summary.EarliestRecordedAt) {
			earliest := event.RecordedAt
			summary.EarliestRecordedAt = &earliest
		}
		if summary.LatestRecordedAt == nil || event.RecordedAt.After(*summary.LatestRecordedAt) {
			latest := event.RecordedAt
			summary.LatestRecordedAt = &latest
		}
	}
	if err := scanner.Err(); err != nil {
		return summary, fmt.Errorf("scan sealed journal after line %d: %w", line, err)
	}
	summary.PolicyVersionCount = uint64(len(policyVersions))
	finalInfo, err := file.Stat()
	if err != nil {
		return summary, fmt.Errorf("stat verified journal: %w", err)
	}
	if finalInfo.Size() != lockedInfo.Size() || !finalInfo.ModTime().Equal(lockedInfo.ModTime()) {
		return summary, errors.New("journal changed during verification")
	}
	if err := validateJournalInfo(finalInfo); err != nil {
		return summary, err
	}
	finalTarget, err := os.Lstat(path)
	if err != nil {
		return summary, fmt.Errorf("reinspect verified journal: %w", err)
	}
	if err := validateJournalInfo(finalTarget); err != nil {
		return summary, err
	}
	if !os.SameFile(finalTarget, finalInfo) {
		return summary, errors.New("journal target changed during verification")
	}
	return summary, nil
}

func decodeDecisionEvent(payload []byte) (v1.DecisionEvent, error) {
	var event v1.DecisionEvent
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return event, errors.New("journal line is not a strict decision event object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return event, errors.New("journal line contains trailing JSON data")
	}
	return event, nil
}
