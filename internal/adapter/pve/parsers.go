package pve

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"

	v1 "github.com/DjangoAILab/pve-storage-guard/api/v1"
)

var errNoWriteSamples = errors.New("histogram contains no write samples")

func parseHistogramLayout(payload []byte, expectedPool string) error {
	lines := strings.Split(strings.TrimSpace(string(payload)), "\n")
	if len(lines) < 2 {
		return errors.New("histogram layout lacks headers")
	}
	top := strings.Fields(lines[0])
	if len(top) != 5 || top[0] != expectedPool || !slices.Equal(top[1:], []string{"total_wait", "disk_wait", "syncq_wait", "asyncq_wait"}) {
		return errors.New("unsupported histogram top-level layout")
	}
	bottom := strings.Fields(lines[1])
	required := []string{"latency", "read", "write", "read", "write", "read", "write", "read", "write"}
	if len(bottom) < len(required) || len(bottom) > len(required)+3 || !slices.Equal(bottom[:len(required)], required) {
		return errors.New("unsupported histogram read/write layout")
	}
	optional := []string{"scrub", "trim", "rebuild"}
	if !slices.Equal(bottom[len(required):], optional[:len(bottom)-len(required)]) {
		return errors.New("unsupported histogram optional layout")
	}
	return nil
}

func parseClusterHealthy(payload []byte, expectedNode string) (bool, error) {
	var rows []map[string]json.RawMessage
	if err := decodeOneJSON(payload, &rows); err != nil {
		return false, errors.New("cluster status is invalid JSON")
	}
	nodeFound := false
	nodeOnline := false
	clusterFound := false
	quorate := false
	for _, row := range rows {
		var kind string
		_ = json.Unmarshal(row["type"], &kind)
		switch kind {
		case "node":
			var name string
			_ = json.Unmarshal(row["name"], &name)
			if name == expectedNode {
				nodeFound = true
				nodeOnline, _ = parseJSONBool(row["online"])
			}
		case "cluster":
			clusterFound = true
			quorate, _ = parseJSONBool(row["quorate"])
		}
	}
	if !nodeFound {
		return false, errors.New("configured node is absent from cluster status")
	}
	return nodeOnline && (!clusterFound || quorate), nil
}

type storageBinding struct {
	StorageType string
	Pool        string
}

func parseStorageConfig(payload []byte) (storageBinding, error) {
	var row map[string]json.RawMessage
	if err := decodeOneJSON(payload, &row); err != nil {
		return storageBinding{}, errors.New("storage config is invalid JSON")
	}
	var result storageBinding
	_ = json.Unmarshal(row["type"], &result.StorageType)
	_ = json.Unmarshal(row["pool"], &result.Pool)
	if result.StorageType == "" || result.Pool == "" {
		return storageBinding{}, errors.New("storage config lacks type or pool")
	}
	return result, nil
}

func parseStorageActive(payload []byte) (bool, string, error) {
	var row map[string]json.RawMessage
	if err := decodeOneJSON(payload, &row); err != nil {
		return false, "", errors.New("storage status is invalid JSON")
	}
	active, activeOK := parseJSONBool(row["active"])
	enabled, enabledOK := parseJSONBool(row["enabled"])
	var storageType string
	_ = json.Unmarshal(row["type"], &storageType)
	if !activeOK || !enabledOK || storageType == "" {
		return false, "", errors.New("storage status lacks required fields")
	}
	return active && enabled, storageType, nil
}

func parseHistogramP95(payload []byte, expectedPool string) (float64, float64, uint64, error) {
	lines := strings.Split(strings.TrimSpace(string(payload)), "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != expectedPool {
		return 0, 0, 0, errors.New("histogram pool binding is invalid")
	}
	type bucket struct {
		upper  uint64
		weight float64
	}
	buckets := make([]bucket, 0, len(lines)-1)
	previous := uint64(0)
	total := float64(0)
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 9 || len(fields) > 12 {
			return 0, 0, 0, errors.New("unsupported histogram column count")
		}
		upper, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil || upper == 0 || (upper+1)&upper != 0 || (len(buckets) > 0 && upper <= previous) {
			return 0, 0, 0, errors.New("invalid histogram bucket boundary")
		}
		weight, err := strconv.ParseFloat(fields[2], 64)
		if err != nil || weight < 0 || math.IsNaN(weight) || math.IsInf(weight, 0) {
			return 0, 0, 0, errors.New("invalid histogram write weight")
		}
		buckets = append(buckets, bucket{upper: upper, weight: weight})
		previous = upper
		total += weight
		if math.IsInf(total, 0) || math.IsNaN(total) {
			return 0, 0, 0, errors.New("histogram total weight is invalid")
		}
		if len(buckets) > 64 {
			return 0, 0, 0, errors.New("histogram has too many buckets")
		}
	}
	if total == 0 {
		return 0, 0, 0, errNoWriteSamples
	}
	target := total * 0.95
	cumulative := float64(0)
	for _, candidate := range buckets {
		cumulative += candidate.weight
		if cumulative >= target {
			return float64(candidate.upper) / 1_000_000, total, candidate.upper, nil
		}
	}
	return 0, 0, 0, errors.New("histogram percentile is incomplete")
}

func parsePSI(payload []byte) (v1.IOPressure, error) {
	result := v1.IOPressure{}
	foundSome := false
	foundFull := false
	for _, line := range strings.Split(strings.TrimSpace(string(payload)), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value := float64(0)
		foundAvg := false
		for _, field := range fields[1:] {
			if strings.HasPrefix(field, "avg10=") {
				parsed, err := strconv.ParseFloat(strings.TrimPrefix(field, "avg10="), 64)
				if err != nil || parsed < 0 || parsed > 100 || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
					return v1.IOPressure{}, errors.New("invalid PSI avg10")
				}
				value = parsed
				foundAvg = true
			}
		}
		if !foundAvg {
			continue
		}
		switch fields[0] {
		case "some":
			result.SomeAvg10 = value
			foundSome = true
		case "full":
			result.FullAvg10 = value
			foundFull = true
		}
	}
	if !foundSome || !foundFull {
		return v1.IOPressure{}, errors.New("PSI sample lacks some or full avg10")
	}
	return result, nil
}

type diskstatsValue struct {
	Reads, Writes, ReadSectors, WrittenSectors uint64
	InFlight, IOTime, Weighted                 uint64
}

func parseDiskstats(payload []byte, devices map[string]string) ([]v1.DiskSignal, error) {
	found := make(map[string]diskstatsValue, len(devices))
	for _, line := range strings.Split(strings.TrimSpace(string(payload)), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 14 {
			continue
		}
		resourceKey, wanted := devices[fields[2]]
		if !wanted {
			continue
		}
		values := [7]uint64{}
		for index, fieldIndex := range []int{3, 7, 5, 9, 11, 12, 13} {
			value, err := strconv.ParseUint(fields[fieldIndex], 10, 64)
			if err != nil {
				return nil, errors.New("configured diskstats row is invalid")
			}
			values[index] = value
		}
		found[resourceKey] = diskstatsValue{Reads: values[0], Writes: values[1], ReadSectors: values[2], WrittenSectors: values[3], InFlight: values[4], IOTime: values[5], Weighted: values[6]}
	}
	if len(found) != len(devices) {
		return nil, errors.New("one or more configured devices are absent from diskstats")
	}
	result := make([]v1.DiskSignal, 0, len(devices))
	for _, resourceKey := range sortedResourceKeys(devices) {
		value := found[resourceKey]
		result = append(result, v1.DiskSignal{
			ResourceKey: resourceKey, ReadsCompletedTotal: value.Reads, WritesCompletedTotal: value.Writes,
			ReadSectorsTotal: value.ReadSectors, WrittenSectorsTotal: value.WrittenSectors,
			InFlightIO: value.InFlight, IOTimeMillisecondsTotal: value.IOTime, WeightedIOMillisecondsTotal: value.Weighted,
		})
	}
	return result, nil
}

func sortedResourceKeys(devices map[string]string) []string {
	keys := make([]string, 0, len(devices))
	for _, key := range devices {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func decodeOneJSON(payload []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func parseJSONBool(raw json.RawMessage) (bool, bool) {
	var value bool
	if err := json.Unmarshal(raw, &value); err == nil {
		return value, true
	}
	var number int
	if err := json.Unmarshal(raw, &number); err == nil && (number == 0 || number == 1) {
		return number == 1, true
	}
	return false, false
}

func validatePoolBinding(configured, returned string) error {
	if returned == configured || strings.HasPrefix(returned, configured+"/") {
		return nil
	}
	return errors.New("storage pool does not match configured ZFS pool")
}
