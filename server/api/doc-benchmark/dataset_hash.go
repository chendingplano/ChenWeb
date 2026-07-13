package docbenchmark

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strconv"
)

const datasetHashPrefix = "chenweb-doc-benchmark-dataset-v1\n"

// MaxRepetitions bounds experiment sampling expansion. CaseSetHash streams at
// most selected-cases*MaxRepetitions frames without materializing sampling units.
const MaxRepetitions = 10_000

type hashEntry struct {
	path string
	body []byte
}

func populateHashes(ds *Dataset) error {
	entries := []hashEntry{{path: "manifest.json", body: ds.ManifestBytes}}
	for _, c := range ds.Cases {
		entries = append(entries, hashEntry{c.Input, c.InputBytes}, hashEntry{c.Expected, c.ExpectedBytes})
		for p, b := range map[string][]byte{c.Input: c.InputBytes, c.Expected: c.ExpectedBytes} {
			sum := sha256.Sum256(b)
			ds.FileHashes[p] = hex.EncodeToString(sum[:])
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].path < entries[j].path })
	h := sha256.New()
	io.WriteString(h, datasetHashPrefix)
	writeUint64(h, uint64(len(entries)))
	for _, e := range entries {
		writeFrame(h, e.path, e.body)
	}
	ds.Hash = hex.EncodeToString(h.Sum(nil))
	ds.DatasetHash = ds.Hash
	ds.ProcessorCaseSetHashes = make(map[Processor]string)
	for _, p := range []Processor{ProcessorChunking, ProcessorExtractMetrics} {
		v, err := ds.CaseSetHash(p, nil, 1)
		if err != nil {
			return err
		}
		ds.ProcessorCaseSetHashes[p] = v
	}
	return nil
}

func writeFrame(w io.Writer, path string, content []byte) {
	writeUint64(w, uint64(len([]byte(path))))
	_, _ = io.WriteString(w, path)
	writeUint64(w, uint64(len(content)))
	_, _ = w.Write(content)
}

func writeUint64(w io.Writer, v uint64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	_, _ = w.Write(b[:])
}

func (d *Dataset) CaseSetHash(processor Processor, caseTags []string, repetitions int) (string, error) {
	if processor != ProcessorChunking && processor != ProcessorExtractMetrics {
		return "", fmt.Errorf("unsupported processor %q", processor)
	}
	if repetitions < 1 {
		return "", fmt.Errorf("repetitions must be positive")
	}
	if repetitions > MaxRepetitions {
		return "", fmt.Errorf("repetitions must not exceed %d", MaxRepetitions)
	}
	tags := map[string]struct{}{}
	for _, tag := range caseTags {
		if _, ok := allowedTags[tag]; !ok {
			return "", fmt.Errorf("unknown case tag %q", tag)
		}
		tags[tag] = struct{}{}
	}
	selectedCaseIDs := make([]string, 0, len(d.Cases))
	for _, c := range d.Cases {
		if !hasProcessor(c.Processors, processor) || !hasAllTags(c.Tags, tags) {
			continue
		}
		selectedCaseIDs = append(selectedCaseIDs, c.CaseID)
	}
	sort.Strings(selectedCaseIDs)
	caseCount := uint64(len(selectedCaseIDs))
	repetitionCount := uint64(repetitions)
	if caseCount != 0 && repetitionCount > ^uint64(0)/caseCount {
		return "", fmt.Errorf("case-set sampling unit count overflows uint64")
	}
	h := sha256.New()
	writeUint64(h, caseCount*repetitionCount)
	for _, caseID := range selectedCaseIDs {
		for repetition := 1; repetition <= repetitions; repetition++ {
			writeFrame(h, caseID, []byte(strconv.Itoa(repetition)))
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func ComputeCaseSetHash(d *Dataset, processor Processor, tags []string, repetitions int) (string, error) {
	return d.CaseSetHash(processor, tags, repetitions)
}
func ComputeDatasetHash(d *Dataset) string {
	if d == nil {
		return ""
	}
	return d.Hash
}
func ComputeFileHashes(d *Dataset) map[string]string {
	out := map[string]string{}
	if d != nil {
		for k, v := range d.FileHashes {
			out[k] = v
		}
	}
	return out
}

func hasProcessor(ps []Processor, want Processor) bool {
	for _, p := range ps {
		if p == want {
			return true
		}
	}
	return false
}
func hasAllTags(got []string, want map[string]struct{}) bool {
	if len(want) == 0 {
		return true
	}
	set := map[string]struct{}{}
	for _, t := range got {
		set[t] = struct{}{}
	}
	for t := range want {
		if _, ok := set[t]; !ok {
			return false
		}
	}
	return true
}
