package telemetry

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"syscall"
	"time"

	v1 "github.com/DjangoAILab/pve-storage-guard/api/v1"
)

const defaultJournalEventMaxBytes = 1024 * 1024
const maxJournalBatchEvents = 64

var journalDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type journalScanOptions struct {
	collect bool
	offset  uint64
	limit   uint64
}

// VerifyJournal validates one sealed private decision journal and returns an
// identity-free summary. It rejects an active writer and performs no mutation.
func VerifyJournal(path string) (v1.DecisionJournalVerification, error) {
	return verifyJournal(path, defaultJournalMaxBytes, defaultJournalEventMaxBytes)
}

// ReadVerifiedJournalBatch returns one bounded private page only after the
// complete sealed journal validates and matches the already-reviewed digest.
func ReadVerifiedJournalBatch(path, expectedDigest string, offset, limit uint64) (v1.DecisionJournalBatch, error) {
	batch := v1.DecisionJournalBatch{
		SchemaVersion: v1.SchemaVersion,
		Kind:          v1.DecisionJournalBatchKind,
		Offset:        offset,
		Events:        make([]v1.DecisionEvent, 0),
	}
	if !journalDigestPattern.MatchString(expectedDigest) {
		return batch, errors.New("expected journal digest must be canonical sha256")
	}
	if limit == 0 || limit > maxJournalBatchEvents {
		return batch, fmt.Errorf("journal batch limit must be between 1 and %d", maxJournalBatchEvents)
	}
	summary, events, err := scanSealedJournal(
		path,
		defaultJournalMaxBytes,
		defaultJournalEventMaxBytes,
		journalScanOptions{collect: true, offset: offset, limit: limit},
	)
	if err != nil {
		return batch, err
	}
	if subtle.ConstantTimeCompare([]byte(summary.ContentDigest), []byte(expectedDigest)) != 1 {
		return batch, errors.New("sealed journal content digest does not match the approved digest")
	}
	if offset > summary.EventCount {
		return batch, errors.New("journal batch offset exceeds the sealed event count")
	}
	batch.Verification = summary
	batch.Events = events
	next := offset + uint64(len(events))
	batch.Complete = next >= summary.EventCount
	if !batch.Complete {
		batch.NextOffset = &next
	}
	return batch, nil
}

func verifyJournal(path string, maxFileBytes int64, maxEventBytes int) (v1.DecisionJournalVerification, error) {
	summary, _, err := scanSealedJournal(path, maxFileBytes, maxEventBytes, journalScanOptions{})
	return summary, err
}

func scanSealedJournal(
	path string,
	maxFileBytes int64,
	maxEventBytes int,
	options journalScanOptions,
) (summary v1.DecisionJournalVerification, events []v1.DecisionEvent, resultErr error) {
	summary = v1.DecisionJournalVerification{
		SchemaVersion: v1.SchemaVersion,
		Kind:          v1.DecisionJournalVerificationKind,
	}
	if options.collect {
		capacity := options.limit
		if capacity > maxJournalBatchEvents {
			capacity = maxJournalBatchEvents
		}
		events = make([]v1.DecisionEvent, 0, int(capacity))
	}
	if path == "" {
		return summary, events, errors.New("journal path is required")
	}
	if maxFileBytes <= 0 || maxEventBytes <= 0 {
		return summary, events, errors.New("journal verification limits must be positive")
	}
	expected, err := os.Lstat(path)
	if err != nil {
		return summary, events, fmt.Errorf("inspect journal: %w", err)
	}
	if err := validateJournalInfo(expected); err != nil {
		return summary, events, err
	}
	file, err := os.Open(path)
	if err != nil {
		return summary, events, fmt.Errorf("open journal for verification: %w", err)
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
		return summary, events, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); err != nil {
		return summary, events, fmt.Errorf("lock sealed journal for verification: %w", err)
	}
	locked = true
	lockedInfo, err := file.Stat()
	if err != nil {
		return summary, events, fmt.Errorf("stat locked journal: %w", err)
	}
	current, err := os.Lstat(path)
	if err != nil {
		return summary, events, fmt.Errorf("reinspect locked journal: %w", err)
	}
	if err := validateJournalInfo(current); err != nil {
		return summary, events, err
	}
	if !os.SameFile(current, lockedInfo) {
		return summary, events, errors.New("journal target changed while locking for verification")
	}

	policyVersions := make(map[string]struct{})
	eventIDs := make(map[string]struct{})
	var domainKey string
	var previousRecordedAt time.Time
	hasher := sha256.New()
	scanner := bufio.NewScanner(io.TeeReader(file, hasher))
	scanner.Buffer(make([]byte, 64*1024), maxEventBytes)
	line := 0
	for scanner.Scan() {
		line++
		event, err := decodeDecisionEvent(scanner.Bytes())
		if err != nil {
			return summary, events, fmt.Errorf("decode journal event on line %d: %w", line, err)
		}
		if err := validateDecisionEvent(event); err != nil {
			return summary, events, fmt.Errorf("validate journal event on line %d: %w", line, err)
		}
		if domainKey == "" {
			domainKey = event.DomainKey
		} else if event.DomainKey != domainKey {
			return summary, events, errors.New("sealed journal contains multiple storage domains")
		}
		index := summary.EventCount
		if options.collect && index >= options.offset && uint64(len(events)) < options.limit {
			events = append(events, event)
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
		return summary, events, fmt.Errorf("scan sealed journal after line %d: %w", line, err)
	}
	summary.ContentDigest = fmt.Sprintf("sha256:%x", hasher.Sum(nil))
	summary.PolicyVersionCount = uint64(len(policyVersions))
	finalInfo, err := file.Stat()
	if err != nil {
		return summary, events, fmt.Errorf("stat verified journal: %w", err)
	}
	if finalInfo.Size() != lockedInfo.Size() || !finalInfo.ModTime().Equal(lockedInfo.ModTime()) {
		return summary, events, errors.New("journal changed during verification")
	}
	if err := validateJournalInfo(finalInfo); err != nil {
		return summary, events, err
	}
	finalTarget, err := os.Lstat(path)
	if err != nil {
		return summary, events, fmt.Errorf("reinspect verified journal: %w", err)
	}
	if err := validateJournalInfo(finalTarget); err != nil {
		return summary, events, err
	}
	if !os.SameFile(finalTarget, finalInfo) {
		return summary, events, errors.New("journal target changed during verification")
	}
	return summary, events, nil
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
