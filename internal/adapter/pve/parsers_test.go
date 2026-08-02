package pve

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func BenchmarkParseHistogramP95(b *testing.B) {
	var fixture strings.Builder
	fixture.WriteString("pool\n")
	for index := 0; index < 40; index++ {
		upper := (uint64(1) << (index + 1)) - 1
		_, _ = fmt.Fprintf(&fixture, "%d 1 %d 0 0 0 0 0 0 0 0 0\n", upper, index+1)
	}
	payload := []byte(fixture.String())
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	for index := 0; index < b.N; index++ {
		if _, _, _, err := parseHistogramP95(payload, "pool"); err != nil {
			b.Fatal(err)
		}
	}
}

func FuzzParseHistogramP95NeverPanics(f *testing.F) {
	f.Add([]byte("pool\n1048575 1 1 0 0 0 0 0 0\n"), "pool")
	f.Add([]byte(""), "pool")
	f.Fuzz(func(t *testing.T, payload []byte, pool string) {
		_, _, _, _ = parseHistogramP95(payload, pool)
	})
}

func TestParseHistogramP95UsesTotalWaitWriteColumn(t *testing.T) {
	payload := []byte("privatepool\n" +
		"1048575\t900\t94\t999\t999\t999\t999\t999\t999\n" +
		"2097151\t0\t1\t999\t999\t999\t999\t999\t999\n" +
		"4194303\t0\t5\t999\t999\t999\t999\t999\t999\n")
	wait, weight, upper, err := parseHistogramP95(payload, "privatepool")
	if err != nil {
		t.Fatalf("parse histogram: %v", err)
	}
	if math.Abs(wait-2.097151) > 0.000001 || weight != 100 || upper != 2097151 {
		t.Fatalf("wait=%f weight=%f upper=%d", wait, weight, upper)
	}
}

func TestParseHistogramLayoutBindsColumnSemantics(t *testing.T) {
	valid := "privatepool total_wait disk_wait syncq_wait asyncq_wait\nlatency read write read write read write read write scrub trim rebuild\n"
	if err := parseHistogramLayout([]byte(valid), "privatepool"); err != nil {
		t.Fatalf("valid layout: %v", err)
	}
	for name, payload := range map[string]string{
		"pool":          strings.Replace(valid, "privatepool", "other", 1),
		"top order":     strings.Replace(valid, "total_wait disk_wait", "disk_wait total_wait", 1),
		"bottom order":  strings.Replace(valid, "latency read write", "latency write read", 1),
		"unknown extra": strings.Replace(valid, "rebuild", "rebuild future", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if err := parseHistogramLayout([]byte(payload), "privatepool"); err == nil {
				t.Fatal("expected unsupported layout")
			}
		})
	}
}

func TestParseHistogramP95FailsClosed(t *testing.T) {
	validRow := "1048575 1 1 0 0 0 0 0 0\n"
	for name, payload := range map[string]string{
		"pool mismatch":   "other\n" + validRow,
		"unknown shape":   "pool\n1048575 1 1\n",
		"bad boundary":    "pool\n1000000 1 1 0 0 0 0 0 0\n",
		"negative weight": "pool\n1048575 1 -1 0 0 0 0 0 0\n",
		"second pool":     "pool\n" + validRow + "other\n" + validRow,
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, _, err := parseHistogramP95([]byte(payload), "pool"); err == nil {
				t.Fatal("expected fail-closed parse error")
			}
		})
	}
}

func TestParseHistogramP95ReportsNoWrites(t *testing.T) {
	_, _, _, err := parseHistogramP95([]byte("pool\n1048575 1 0 0 0 0 0 0 0\n"), "pool")
	if err != errNoWriteSamples {
		t.Fatalf("err=%v", err)
	}
}

func TestParsePVEAndProcSignals(t *testing.T) {
	healthy, err := parseClusterHealthy([]byte(`[{"type":"cluster","quorate":1},{"type":"node","name":"private-node","online":true}]`), "private-node")
	if err != nil || !healthy {
		t.Fatalf("healthy=%v err=%v", healthy, err)
	}
	binding, err := parseStorageConfig([]byte(`{"type":"zfspool","pool":"privatepool/dataset","storage":"private-storage"}`))
	if err != nil || binding.StorageType != "zfspool" || validatePoolBinding("privatepool", binding.Pool) != nil {
		t.Fatalf("binding=%+v err=%v", binding, err)
	}
	active, storageType, err := parseStorageActive([]byte(`{"active":1,"enabled":true,"type":"zfspool","avail":42}`))
	if err != nil || !active || storageType != "zfspool" {
		t.Fatalf("active=%v type=%q err=%v", active, storageType, err)
	}
	pressure, err := parsePSI([]byte("some avg10=12.50 avg60=1.00 avg300=0.50 total=1\nfull avg10=3.25 avg60=1.00 avg300=0.50 total=1\n"))
	if err != nil || pressure.SomeAvg10 != 12.5 || pressure.FullAvg10 != 3.25 {
		t.Fatalf("pressure=%+v err=%v", pressure, err)
	}
	signals, err := parseDiskstats([]byte("8 16 private-disk 1 2 3 4 5 6 7 8 9 10 11 0 0 0\n"), map[string]string{"private-disk": "resource-a"})
	if err != nil || len(signals) != 1 || signals[0].ResourceKey != "resource-a" || signals[0].ReadsCompletedTotal != 1 || signals[0].WritesCompletedTotal != 5 || signals[0].ReadSectorsTotal != 3 || signals[0].WrittenSectorsTotal != 7 || signals[0].InFlightIO != 9 || signals[0].IOTimeMillisecondsTotal != 10 || signals[0].WeightedIOMillisecondsTotal != 11 {
		t.Fatalf("signals=%+v err=%v", signals, err)
	}
}

func TestCommandSpecsAreFixedAndShellFree(t *testing.T) {
	request := commandRequest{Node: "node", Storage: "store", ZPool: "pool", IntervalSeconds: 2}
	path, args, err := commandSpec(opZFSWaitHistogram, request)
	if err != nil || path != "/usr/sbin/zpool" || strings.Join(args, " ") != "iostat -wpH -y pool 2 1" {
		t.Fatalf("path=%q args=%v err=%v", path, args, err)
	}
	for _, op := range []operation{opClusterStatus, opStorageConfig, opStorageStatus, opZFSWaitHistogramLayout, opZFSWaitHistogram} {
		path, _, err := commandSpec(op, request)
		if err != nil || path == "/bin/sh" || path == "/usr/bin/env" {
			t.Fatalf("operation=%s path=%q err=%v", op, path, err)
		}
	}
}
