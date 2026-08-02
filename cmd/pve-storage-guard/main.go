// Command pve-storage-guard is the product CLI and service entrypoint.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	v1 "github.com/DjangoAILab/pve-storage-guard/api/v1"
	pveadapter "github.com/DjangoAILab/pve-storage-guard/internal/adapter/pve"
	"github.com/DjangoAILab/pve-storage-guard/internal/allocator"
	"github.com/DjangoAILab/pve-storage-guard/internal/config"
	"github.com/DjangoAILab/pve-storage-guard/internal/controller"
	"github.com/DjangoAILab/pve-storage-guard/internal/telemetry"
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
	if len(args) > 0 && args[0] == "agent" {
		if err := runAgent(args[1:], stdout, stderr); err != nil {
			_, _ = fmt.Fprintf(stderr, "agent: %v\n", err)
			return 1
		}
		return 0
	}
	if len(args) > 0 && args[0] == "journal" {
		if len(args) < 2 {
			_ = writeUsage(stderr)
			return 2
		}
		var err error
		switch args[1] {
		case "verify":
			err = runJournalVerify(args[2:], stdout, stderr)
		case "batch":
			err = runJournalBatch(args[2:], stdout, stderr)
		default:
			_ = writeUsage(stderr)
			return 2
		}
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "journal %s: %v\n", args[1], err)
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

func runAgent(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || (args[0] != "inventory" && args[0] != "observe") {
		return errors.New("expected agent inventory or agent observe")
	}
	operation := args[0]
	flags := flag.NewFlagSet("agent "+operation, flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "", "private local PVE agent JSON config")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *configPath == "" {
		return errors.New("--config is required and positional arguments are not accepted")
	}
	document, err := config.ReadPVEAgentConfig(*configPath)
	if err != nil {
		return err
	}
	reader, err := pveadapter.NewLocalReader(document)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if operation == "inventory" {
		inventory, err := reader.InventorySnapshot(ctx)
		if err != nil {
			return err
		}
		return encoder.Encode(inventory)
	}
	observation, err := reader.Observe(ctx, document.Spec.DomainKey, time.Time{})
	if err != nil {
		return err
	}
	return encoder.Encode(observation)
}

func runJournalVerify(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("journal verify", flag.ContinueOnError)
	flags.SetOutput(stderr)
	journalPath := flags.String("journal", "", "sealed private decision journal JSONL")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if *journalPath == "" {
		return errors.New("--journal is required")
	}
	summary, err := telemetry.VerifyJournal(*journalPath)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(summary); err != nil {
		return fmt.Errorf("write verification summary: %w", err)
	}
	return nil
}

func runJournalBatch(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("journal batch", flag.ContinueOnError)
	flags.SetOutput(stderr)
	journalPath := flags.String("journal", "", "sealed private decision journal JSONL")
	expectedDigest := flags.String("expected-digest", "", "approved canonical sha256 content digest")
	offset := flags.Uint64("offset", 0, "zero-based event offset")
	limit := flags.Uint64("limit", 64, "maximum events to emit (1-64)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if *journalPath == "" {
		return errors.New("--journal is required")
	}
	if *expectedDigest == "" {
		return errors.New("--expected-digest is required")
	}
	batch, err := telemetry.ReadVerifiedJournalBatch(*journalPath, *expectedDigest, *offset, *limit)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(batch); err != nil {
		return fmt.Errorf("write private journal batch: %w", err)
	}
	return nil
}

func runShadow(args []string, stdin io.Reader, stdout, stderr io.Writer) (resultErr error) {
	flags := flag.NewFlagSet("shadow", flag.ContinueOnError)
	flags.SetOutput(stderr)
	policyPath := flags.String("policy", "", "storage-domain policy JSON")
	journalPath := flags.String("journal", "", "optional private append-only decision journal JSONL")
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
	var journal *telemetry.Journal
	if *journalPath != "" {
		journal, err = telemetry.OpenJournal(*journalPath)
		if err != nil {
			return fmt.Errorf("open journal: %w", err)
		}
		defer func() {
			if closeErr := journal.Close(); resultErr == nil && closeErr != nil {
				resultErr = fmt.Errorf("close journal: %w", closeErr)
			}
		}()
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
		if journal != nil {
			event, eventErr := telemetry.NewShadowDecisionEvent(observation, proposal)
			if eventErr != nil {
				return fmt.Errorf("build decision event on line %d: %w", line, eventErr)
			}
			if eventErr := journal.Append(event); eventErr != nil {
				return fmt.Errorf("append decision event on line %d: %w", line, eventErr)
			}
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
  pve-storage-guard agent inventory --config PRIVATE-AGENT.json
  pve-storage-guard agent observe --config PRIVATE-AGENT.json
  pve-storage-guard shadow --policy POLICY.json --enrollment RESOURCE.json [--enrollment ...] [--journal DECISIONS.jsonl]
  pve-storage-guard journal verify --journal SEALED-DECISIONS.jsonl
  pve-storage-guard journal batch --journal SEALED-DECISIONS.jsonl --expected-digest sha256:... [--offset N] [--limit 64]

The agent commands perform one read-only local PVE/OpenZFS sample. Their private
config binds host identities to opaque output keys; neither command actuates,
opens a listener, or sends data over the network.

The shadow command reads newline-delimited observations from stdin and writes
newline-delimited proposals to stdout. An explicit --journal appends and syncs
private decision events before their proposals are emitted. No journal is
created by default, and the command never invokes an actuator.

The journal verify command takes a shared non-blocking lock and emits one
identity-free structural summary. It refuses an active writer and performs no
rotation, import, network delivery, or mutation.

The journal batch command revalidates the entire sealed file and its approved
content digest before emitting a bounded page of private events. Its stdout is
private data: pipe it only to an explicitly authorized local consumer. The
command performs no network delivery or mutation.
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
