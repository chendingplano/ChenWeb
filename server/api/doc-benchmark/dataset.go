package docbenchmark

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var semverRE = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-((?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

var allowedTags = map[string]struct{}{
	"toc": {}, "boundary": {}, "final-small-chunk": {}, "long-list": {},
	"overlap": {}, "reordered-lines": {}, "no-metric": {}, "negative-metric": {},
	"duplicate-mention": {}, "multiple-metrics": {}, "multiple-units": {},
	"implicit-metric": {}, "multilingual": {},
}

type validationErrors []string

func (v validationErrors) Error() string {
	sort.Strings(v)
	return "dataset validation failed:\n" + strings.Join(v, "\n")
}

func LoadDataset(root string) (*Dataset, error) {
	canonicalRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("dataset root: %w", err)
	}
	info, err := os.Stat(canonicalRoot)
	if err != nil || !info.IsDir() {
		if err == nil {
			err = errors.New("not a directory")
		}
		return nil, fmt.Errorf("dataset root: %w", err)
	}
	manifestBytes, err := readRegularFile(canonicalRoot, "manifest.json")
	if err != nil {
		return nil, fmt.Errorf("manifest.json: %w", err)
	}
	var rawManifest manifestJSON
	if err := decodeStrict(manifestBytes, &rawManifest); err != nil {
		return nil, fmt.Errorf("manifest.json: %w", err)
	}
	manifest := Manifest{
		SchemaVersion: rawManifest.SchemaVersion, DatasetID: rawManifest.DatasetID,
		DatasetVersion: rawManifest.DatasetVersion, GeneratorVersion: rawManifest.GeneratorVersion,
		seedPresent: rawManifest.Seed != nil, casesPresent: rawManifest.Cases != nil,
	}
	if rawManifest.Seed != nil {
		manifest.Seed = *rawManifest.Seed
	}
	if rawManifest.Cases != nil {
		manifest.Cases = *rawManifest.Cases
	}
	ds := &Dataset{Root: canonicalRoot, Manifest: manifest, ManifestBytes: manifestBytes, FileHashes: make(map[string]string)}
	problems := validateManifest(ds)
	refs := make(map[string]string)
	for i, c := range manifest.Cases {
		caseID := c.CaseID
		if caseID == "" {
			caseID = fmt.Sprintf("cases[%d]", i)
		}
		inputPath, inputOK := validateReference(canonicalRoot, c.Input, fmt.Sprintf("cases[%d].input", i), caseID, refs, &problems)
		expectedPath, expectedOK := validateReference(canonicalRoot, c.Expected, fmt.Sprintf("cases[%d].expected", i), caseID, refs, &problems)
		if !inputOK || !expectedOK {
			continue
		}
		inputBytes, inputErr := readRegularFile(canonicalRoot, inputPath)
		if inputErr != nil {
			problems = append(problems, fieldError(caseID, fmt.Sprintf("cases[%d].input", i), inputErr.Error()))
			continue
		}
		expectedBytes, expectedErr := readRegularFile(canonicalRoot, expectedPath)
		if expectedErr != nil {
			problems = append(problems, fieldError(caseID, fmt.Sprintf("cases[%d].expected", i), expectedErr.Error()))
			continue
		}
		lineNumbers, lineErr := canonicalLineNumbers(inputBytes)
		if lineErr != nil {
			problems = append(problems, fieldError(caseID, fmt.Sprintf("cases[%d].input", i), lineErr.Error()))
			continue
		}
		var expected ExpectedOutput
		if err := decodeStrict(expectedBytes, &expected); err != nil {
			problems = append(problems, fieldError(caseID, fmt.Sprintf("cases[%d].expected", i), err.Error()))
			continue
		}
		dc := DatasetCase{ManifestCase: c, ExpectedOutput: expected, InputBytes: inputBytes, ExpectedBytes: expectedBytes, LineNumbers: lineNumbers}
		validateExpected(dc, i, &problems)
		ds.Cases = append(ds.Cases, dc)
	}
	if len(problems) > 0 {
		return nil, validationErrors(problems)
	}
	if err := populateHashes(ds); err != nil {
		return nil, err
	}
	return ds, nil
}

// ValidateDataset reloads and fully validates a dataset root.
func ValidateDataset(root string) error { _, err := LoadDataset(root); return err }

func decodeStrict(raw []byte, dst any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return err
	}
	var trailing any
	if err := d.Decode(&trailing); err == nil {
		return errors.New("multiple JSON values")
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func validateManifest(ds *Dataset) validationErrors {
	m := ds.Manifest
	var out validationErrors
	if m.SchemaVersion != 1 {
		out = append(out, fmt.Sprintf("schema_version: unsupported schema version %d", m.SchemaVersion))
	}
	if m.DatasetID == "" {
		out = append(out, "dataset_id: must not be empty")
	}
	if !semverRE.MatchString(m.DatasetVersion) {
		out = append(out, "dataset_version: must be valid SemVer 2.0.0")
	}
	if !semverRE.MatchString(m.GeneratorVersion) {
		out = append(out, "generator_version: must be valid SemVer 2.0.0")
	}
	if !m.seedPresent {
		out = append(out, "seed: required")
	}
	if !m.casesPresent {
		out = append(out, "cases: required")
	} else if len(m.Cases) == 0 {
		out = append(out, "cases: must not be empty")
	}
	seenCases := make(map[string]struct{})
	for i, c := range m.Cases {
		id := c.CaseID
		if !restrictedASCII(id, false) {
			out = append(out, fieldError(id, fmt.Sprintf("cases[%d].case_id", i), "must use only ASCII letters, digits, '.', '_', and '-'"))
		}
		if _, ok := seenCases[id]; ok {
			out = append(out, fieldError(id, fmt.Sprintf("cases[%d].case_id", i), "duplicate case ID"))
		}
		seenCases[id] = struct{}{}
		if len(c.Processors) == 0 {
			out = append(out, fieldError(id, fmt.Sprintf("cases[%d].processors", i), "applicability must not be empty"))
		}
		seenProcessors := make(map[Processor]struct{})
		for j, p := range c.Processors {
			if p != ProcessorChunking && p != ProcessorExtractMetrics {
				out = append(out, fieldError(id, fmt.Sprintf("cases[%d].processors[%d]", i, j), fmt.Sprintf("unsupported processor %q", p)))
			}
			if _, ok := seenProcessors[p]; ok {
				out = append(out, fieldError(id, fmt.Sprintf("cases[%d].processors[%d]", i, j), "duplicate processor"))
			}
			seenProcessors[p] = struct{}{}
		}
		seenTags := make(map[string]struct{})
		for j, tag := range c.Tags {
			if _, ok := allowedTags[tag]; !ok {
				out = append(out, fieldError(id, fmt.Sprintf("cases[%d].tags[%d]", i, j), fmt.Sprintf("unknown tag %q", tag)))
			}
			if _, ok := seenTags[tag]; ok {
				out = append(out, fieldError(id, fmt.Sprintf("cases[%d].tags[%d]", i, j), fmt.Sprintf("repeated tag %q", tag)))
			}
			seenTags[tag] = struct{}{}
		}
	}
	return out
}

func validateReference(root, raw, field, caseID string, refs map[string]string, out *validationErrors) (string, bool) {
	if !restrictedASCII(raw, true) {
		*out = append(*out, fieldError(caseID, field, "path must use only ASCII letters, digits, '.', '_', '-', and '/'"))
		return "", false
	}
	if raw == "" || path.IsAbs(raw) || filepath.IsAbs(raw) {
		*out = append(*out, fieldError(caseID, field, "path must be relative"))
		return "", false
	}
	for _, part := range strings.Split(raw, "/") {
		if part == ".." {
			*out = append(*out, fieldError(caseID, field, "path must not contain '..'"))
			return "", false
		}
	}
	clean := path.Clean(raw)
	if clean == "." || strings.HasPrefix(clean, "../") {
		*out = append(*out, fieldError(caseID, field, "path escapes dataset root"))
		return "", false
	}
	if previous, exists := refs[clean]; exists {
		*out = append(*out, fieldError(caseID, field, "duplicate normalized reference (already used by "+previous+")"))
		return "", false
	}
	refs[clean] = field
	return clean, true
}

func readRegularFile(root, rel string) ([]byte, error) {
	cur := root
	parts := strings.Split(filepath.FromSlash(rel), string(filepath.Separator))
	for _, part := range parts {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("symlink component %q is forbidden", part)
		}
	}
	info, err := os.Stat(cur)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("referenced path is not a regular file")
	}
	return os.ReadFile(cur)
}

func canonicalLineNumbers(raw []byte) ([]int, error) {
	lines := strings.Split(string(raw), "\n")
	out := make([]int, 0, len(lines))
	seen := map[int]struct{}{}
	for physical, line := range lines {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 7 {
			return nil, fmt.Errorf("physical line %d: expected 7 tab-separated fields", physical+1)
		}
		n, err := strconv.Atoi(strings.TrimSpace(fields[0]))
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("physical line %d: invalid line number", physical+1)
		}
		if _, ok := seen[n]; ok {
			return nil, fmt.Errorf("physical line %d: duplicate line number %d", physical+1, n)
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out, nil
}

func validateExpected(c DatasetCase, caseIndex int, out *validationErrors) {
	e := c.ExpectedOutput
	id := c.CaseID
	if e.SchemaVersion != 1 {
		*out = append(*out, fieldError(id, "expected.schema_version", fmt.Sprintf("unsupported schema version %d", e.SchemaVersion)))
	}
	want := map[Processor]bool{}
	for _, p := range c.Processors {
		want[p] = true
	}
	if want[ProcessorChunking] != (e.Chunking != nil) || want[ProcessorExtractMetrics] != (e.ExtractMetrics != nil) {
		*out = append(*out, fieldError(id, fmt.Sprintf("cases[%d].processors", caseIndex), "must exactly match expected JSON processor sections"))
	}
	linePos := map[int]int{}
	for i, n := range c.LineNumbers {
		linePos[n] = i
	}
	if e.Chunking != nil {
		validateChunking(id, e.Chunking, linePos, out)
	}
	if e.ExtractMetrics != nil {
		seen := map[string]struct{}{}
		for i, m := range e.ExtractMetrics.Metrics {
			base := fmt.Sprintf("expected.extract_metrics.metrics[%d]", i)
			if strings.TrimSpace(m.GoldID) == "" {
				*out = append(*out, fieldError(id, base+".gold_id", "must not be empty"))
			} else if _, ok := seen[m.GoldID]; ok {
				*out = append(*out, fieldError(id, base+".gold_id", "duplicate gold ID"))
			}
			seen[m.GoldID] = struct{}{}
			validateLineRefs(id, base+".source_lines", m.SourceLines, linePos, false, out)
		}
	}
}

func validateChunking(id string, c *ExpectedChunking, linePos map[int]int, out *validationErrors) {
	normalAssignments := make(map[int][]int)
	for chunkIndex, chunk := range c.Chunks {
		for _, line := range chunk.NormalLines {
			normalAssignments[line] = append(normalAssignments[line], chunkIndex)
		}
	}
	groups := map[string]struct{}{}
	for i, g := range c.ProtectedGroups {
		base := fmt.Sprintf("expected.chunking.protected_groups[%d]", i)
		if strings.TrimSpace(g.GroupID) == "" {
			*out = append(*out, fieldError(id, base+".group_id", "must not be empty"))
		} else if _, ok := groups[g.GroupID]; ok {
			*out = append(*out, fieldError(id, base+".group_id", "duplicate group ID"))
		}
		groups[g.GroupID] = struct{}{}
		if strings.TrimSpace(g.Kind) == "" {
			*out = append(*out, fieldError(id, base+".kind", "must not be empty"))
		}
		if g.SplitPolicy != "never" && g.SplitPolicy != "expected" {
			*out = append(*out, fieldError(id, base+".split_policy", "must be 'never' or 'expected'"))
		}
		validateLineRefs(id, base+".lines", g.Lines, linePos, true, out)
		groupChunk := -1
		policyMismatch := false
		for lineIndex, line := range g.Lines {
			assignments := normalAssignments[line]
			if len(assignments) != 1 {
				message := "must have exactly one expected normal-chunk assignment"
				*out = append(*out, fieldError(id, fmt.Sprintf("%s.lines[%d]", base, lineIndex), message))
				continue
			}
			if groupChunk == -1 {
				groupChunk = assignments[0]
			}
			if g.SplitPolicy == "never" && assignments[0] != groupChunk {
				policyMismatch = true
			}
		}
		if policyMismatch {
			*out = append(*out, fieldError(id, base+".split_policy", "'never' requires every group line in the same expected normal chunk"))
		}
	}
	for i, ch := range c.Chunks {
		base := fmt.Sprintf("expected.chunking.chunks[%d]", i)
		if ch.Sequence != i+1 {
			*out = append(*out, fieldError(id, base+".sequence", fmt.Sprintf("must be %d", i+1)))
		}
		validateLineRefs(id, base+".overlap_lines", ch.OverlapLines, linePos, false, out)
		validateLineRefs(id, base+".normal_lines", ch.NormalLines, linePos, false, out)
	}
}

func validateLineRefs(id, field string, refs []int, positions map[int]int, requireNonempty bool, out *validationErrors) {
	if requireNonempty && len(refs) == 0 {
		*out = append(*out, fieldError(id, field, "must not be empty"))
		return
	}
	seen := map[int]struct{}{}
	previous := -1
	for i, n := range refs {
		item := fmt.Sprintf("%s[%d]", field, i)
		pos, ok := positions[n]
		if n <= 0 || !ok {
			*out = append(*out, fieldError(id, item, "line reference is invalid or stale"))
			continue
		}
		if _, ok := seen[n]; ok {
			*out = append(*out, fieldError(id, item, "duplicate line reference"))
		}
		seen[n] = struct{}{}
		if previous > pos {
			*out = append(*out, fieldError(id, item, "line references must follow source order"))
		}
		previous = pos
	}
}

func restrictedASCII(s string, slash bool) bool {
	if s == "" {
		return false
	}
	for _, r := range []byte(s) {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' || (slash && r == '/') {
			continue
		}
		return false
	}
	return true
}
func fieldError(caseID, field, message string) string {
	if caseID == "" {
		caseID = "<empty>"
	}
	return fmt.Sprintf("case %q %s: %s", caseID, field, message)
}
