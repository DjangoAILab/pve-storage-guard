package main

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const shadowBenchmarkBatchSize = 32

func BenchmarkShadowCommandStream(b *testing.B) {
	benchmarkShadowCommand(b, false)
}

func BenchmarkShadowCommandStreamWithJournal(b *testing.B) {
	benchmarkShadowCommand(b, true)
}

func benchmarkShadowCommand(b *testing.B, withJournal bool) {
	b.Helper()
	root := filepath.Join("..", "..")
	input := benchmarkObservationBatch(b, shadowBenchmarkBatchSize)
	args := []string{
		"shadow",
		"--policy", filepath.Join(root, "configs", "examples", "reference-shadow-policy.json"),
		"--enrollment", filepath.Join(root, "configs", "examples", "reference-enrollment.json"),
	}
	if withJournal {
		args = append(args, "--journal", filepath.Join(b.TempDir(), "decisions.jsonl"))
	}

	var stderr bytes.Buffer
	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	b.ResetTimer()
	for range b.N {
		if code := run(args, strings.NewReader(input), io.Discard, &stderr); code != 0 {
			b.Fatalf("shadow exit %d: %s", code, stderr.String())
		}
	}
	b.StopTimer()
	b.ReportMetric(shadowBenchmarkBatchSize, "observations/op")
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*shadowBenchmarkBatchSize), "ns/observation")
}

func benchmarkObservationBatch(b *testing.B, count int) string {
	b.Helper()
	observedAt := time.Now().UTC().Format(time.RFC3339Nano)
	var input strings.Builder
	for index := range count {
		wait := 10.0
		if index%10 >= 8 {
			wait = 30
		}
		_, err := fmt.Fprintf(
			&input,
			`{"schemaVersion":"guard.storage-slo.io/v1alpha1","id":"benchmark-observation-%04d","observedAt":%q,"domainKey":"reference-bulk-pool","writeWaitP95Milliseconds":%.1f,"waitValid":true,"emergency":false,"managementPlaneHealthy":true}`+"\n",
			index,
			observedAt,
			wait,
		)
		if err != nil {
			b.Fatal(err)
		}
	}
	return input.String()
}
