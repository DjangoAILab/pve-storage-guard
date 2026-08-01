// Package config decodes versioned user configuration into core policy types.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	v1 "github.com/DjangoAILab/pve-storage-guard/api/v1"
	"github.com/DjangoAILab/pve-storage-guard/internal/allocator"
	"github.com/DjangoAILab/pve-storage-guard/internal/policy"
)

// StorageDomainPolicy is the v1alpha1 JSON policy document.
type StorageDomainPolicy struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"metadata"`
	Spec struct {
		Mode                   string `json:"mode"`
		ControlIntervalSeconds int    `json:"controlIntervalSeconds"`
		CooldownSeconds        int    `json:"cooldownSeconds"`
		TelemetryMaxAgeSeconds int    `json:"telemetryMaxAgeSeconds"`
		Latency                struct {
			HealthyP95Milliseconds float64 `json:"healthyP95Milliseconds"`
			TargetP95Milliseconds  float64 `json:"targetP95Milliseconds"`
			EmergencyMilliseconds  float64 `json:"emergencyMilliseconds"`
		} `json:"latency"`
		Budget struct {
			MinimumMiBPS float64 `json:"minimumMiBPS"`
			InitialMiBPS float64 `json:"initialMiBPS"`
			MaximumMiBPS float64 `json:"maximumMiBPS"`
		} `json:"budget"`
		AIMD struct {
			AdditiveIncreaseMiBPS  float64 `json:"additiveIncreaseMiBPS"`
			MultiplicativeDecrease float64 `json:"multiplicativeDecrease"`
			HealthyWindows         int     `json:"healthyWindows"`
			BreachWindows          int     `json:"breachWindows"`
		} `json:"aimd"`
	} `json:"spec"`
}

// DiskEnrollment is the v1alpha1 JSON enrollment document.
type DiskEnrollment struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		StorageDomain string `json:"storageDomain"`
		ResourceKey   string `json:"resourceKey"`
		WorkloadClass string `json:"workloadClass"`
		Criticality   string `json:"criticality"`
		Envelope      struct {
			MinimumMiBPS float64 `json:"minimumMiBPS"`
			MaximumMiBPS float64 `json:"maximumMiBPS"`
			Weight       float64 `json:"weight"`
		} `json:"envelope"`
	} `json:"spec"`
}

// ReadPolicy strictly decodes and validates a policy file.
func ReadPolicy(path string) (StorageDomainPolicy, error) {
	var document StorageDomainPolicy
	if err := readStrictJSON(path, &document); err != nil {
		return document, fmt.Errorf("read policy: %w", err)
	}
	if document.APIVersion != v1.SchemaVersion || document.Kind != "StorageDomainPolicy" {
		return document, errors.New("policy apiVersion or kind is unsupported")
	}
	if document.Metadata.Name == "" || document.Metadata.Version == "" {
		return document, errors.New("policy metadata name and version are required")
	}
	if document.Spec.Mode != "shadow" {
		return document, errors.New("this command accepts shadow policies only")
	}
	if document.Spec.ControlIntervalSeconds < 1 || document.Spec.TelemetryMaxAgeSeconds < 1 {
		return document, errors.New("control interval and telemetry max age must be positive")
	}
	if _, err := document.CorePolicy(); err != nil {
		return document, err
	}
	return document, nil
}

// CorePolicy converts the versioned document to a platform-neutral policy.
func (d StorageDomainPolicy) CorePolicy() (policy.PoolPolicy, error) {
	core := policy.PoolPolicy{
		MinimumBudgetMiBPS:      d.Spec.Budget.MinimumMiBPS,
		InitialBudgetMiBPS:      d.Spec.Budget.InitialMiBPS,
		MaximumBudgetMiBPS:      d.Spec.Budget.MaximumMiBPS,
		HealthyWaitMilliseconds: d.Spec.Latency.HealthyP95Milliseconds,
		TargetWaitMilliseconds:  d.Spec.Latency.TargetP95Milliseconds,
		EmergencyMilliseconds:   d.Spec.Latency.EmergencyMilliseconds,
		AdditiveIncreaseMiBPS:   d.Spec.AIMD.AdditiveIncreaseMiBPS,
		MultiplicativeDecrease:  d.Spec.AIMD.MultiplicativeDecrease,
		HealthyWindows:          d.Spec.AIMD.HealthyWindows,
		BreachWindows:           d.Spec.AIMD.BreachWindows,
		Cooldown:                time.Duration(d.Spec.CooldownSeconds) * time.Second,
	}
	if err := core.Validate(); err != nil {
		return policy.PoolPolicy{}, fmt.Errorf("validate policy: %w", err)
	}
	return core, nil
}

// ReadEnrollment strictly decodes one explicitly enrolled resource.
func ReadEnrollment(path string) (DiskEnrollment, error) {
	var document DiskEnrollment
	if err := readStrictJSON(path, &document); err != nil {
		return document, fmt.Errorf("read enrollment: %w", err)
	}
	if document.APIVersion != v1.SchemaVersion || document.Kind != "DiskEnrollment" {
		return document, errors.New("enrollment apiVersion or kind is unsupported")
	}
	if document.Metadata.Name == "" || document.Spec.StorageDomain == "" || document.Spec.ResourceKey == "" {
		return document, errors.New("enrollment name, storage domain, and resource key are required")
	}
	if document.Spec.Criticality != "non-critical" {
		return document, errors.New("only explicitly non-critical resources can be enrolled")
	}
	if _, err := allocator.Allocate(document.Spec.Envelope.MaximumMiBPS, []allocator.DiskEnvelope{document.Envelope()}); err != nil {
		return document, fmt.Errorf("validate enrollment envelope: %w", err)
	}
	return document, nil
}

// Envelope converts an enrollment to a generic allocation boundary.
func (d DiskEnrollment) Envelope() allocator.DiskEnvelope {
	return allocator.DiskEnvelope{
		ResourceKey:  d.Spec.ResourceKey,
		MinimumMiBPS: d.Spec.Envelope.MinimumMiBPS,
		MaximumMiBPS: d.Spec.Envelope.MaximumMiBPS,
		Weight:       d.Spec.Envelope.Weight,
	}
}

func readStrictJSON(path string, value any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}
