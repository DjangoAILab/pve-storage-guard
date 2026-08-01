// Command pve-storage-guard is the product CLI and service entrypoint.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	v1 "github.com/DjangoAILab/pve-storage-guard/api/v1"
	"github.com/DjangoAILab/pve-storage-guard/internal/allocator"
	"github.com/DjangoAILab/pve-storage-guard/internal/config"
	"github.com/DjangoAILab/pve-storage-guard/internal/controller"
)

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "version" {
		_, _ = fmt.Fprintln(stdout, version)
		return 0
	}
	if len(args) > 0 && args[0] == "shadow" {
		if err := runShadow(args[1:], stdin, stdout, stderr); err != nil {
			_, _ = fmt.Fprintf(stderr, "shadow: %v\n", err)
			return 1
		}
		return 0
	}
	_ = writeUsage(stderr)
	if len(args) == 0 {
		return 0
	}
	return 2
}

func runShadow(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("shadow", flag.ContinueOnError)
	flags.SetOutput(stderr)
	policyPath := flags.String("policy", "", "storage-domain policy JSON")
	var enrollmentPaths stringList
	flags.Var(&enrollmentPaths, "enrollment", "disk enrollment JSON; repeat for each resource")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if *policyPath == "" || len(enrollmentPaths) == 0 {
		return errors.New("--policy and at least one --enrollment are required")
	}

	document, err := config.ReadPolicy(*policyPath)
	if err != nil {
		return err
	}
	corePolicy, err := document.CorePolicy()
	if err != nil {
		return err
	}
	envelopes := make([]allocator.DiskEnvelope, 0, len(enrollmentPaths))
	for _, path := range enrollmentPaths {
		enrollment, readErr := config.ReadEnrollment(path)
		if readErr != nil {
			return readErr
		}
		if enrollment.Spec.StorageDomain != document.Metadata.Name {
			return fmt.Errorf("enrollment %q belongs to a different storage domain", enrollment.Metadata.Name)
		}
		envelopes = append(envelopes, enrollment.Envelope())
	}
	shadow, err := controller.NewShadow(
		document.Metadata.Name,
		document.Metadata.Version,
		time.Duration(document.Spec.TelemetryMaxAgeSeconds)*time.Second,
		corePolicy,
		envelopes,
	)
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	line := 0
	for scanner.Scan() {
		line++
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		var observation v1.Observation
		decoder := json.NewDecoder(strings.NewReader(scanner.Text()))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&observation); err != nil {
			return fmt.Errorf("decode observation on line %d: %w", line, err)
		}
		proposal, err := shadow.Process(observation, time.Now().UTC())
		if err != nil {
			return fmt.Errorf("process observation on line %d: %w", line, err)
		}
		if err := encoder.Encode(proposal); err != nil {
			return fmt.Errorf("write proposal: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read observations: %w", err)
	}
	return nil
}

func writeUsage(writer io.Writer) error {
	_, err := fmt.Fprint(writer, `pve-storage-guard — Adaptive I/O protection for Proxmox VE hosts

Status: pre-release; observer/shadow only

Usage:
  pve-storage-guard version
  pve-storage-guard shadow --policy POLICY.json --enrollment RESOURCE.json [--enrollment ...]

The shadow command reads newline-delimited observations from stdin and writes
newline-delimited proposals to stdout. It never invokes an actuator.
`)
	return err
}

type stringList []string

func (values *stringList) String() string { return strings.Join(*values, ",") }

func (values *stringList) Set(value string) error {
	if value == "" {
		return errors.New("path cannot be empty")
	}
	*values = append(*values, value)
	return nil
}
