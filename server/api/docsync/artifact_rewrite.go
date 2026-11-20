package docsync

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
)

func artifactNeedsIDRewrite(kind, name string) bool {
	return (kind == documentItem && name == "result.json") || kind == "generate_summaries" || kind == "generate_topics" ||
		(kind == "structure_analyzer" && path.Base(name) == "structure_summary.json")
}

var quotedArtifactID = regexp.MustCompile(`"[^"\\]+"`)

func rewritePortableFile(src string, sourceID, localID int64, kind, original string) (string, error) {
	raw, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	var result []byte
	if kind == "structure_analyzer" || kind == documentItem {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return "", err
		}
		if kind == "structure_analyzer" {
			fields["record_id"], _ = json.Marshal(localID)
		} else {
			if fields["input_id"] == nil || fields["source_pdf"] == nil {
				return src, nil
			}
			fields["input_id"], _ = json.Marshal(localID)
			fields["source_pdf"], _ = json.Marshal(original)
		}
		result, err = json.MarshalIndent(fields, "", "  ")
		if err != nil {
			return "", err
		}
	} else {
		lines := strings.Split(string(raw), "\n")
		for i, line := range lines {
			switch {
			case line == fmt.Sprintf("record_id: %d", sourceID):
				lines[i] = fmt.Sprintf("record_id: %d", localID)
			case kind == "generate_summaries" && (strings.HasPrefix(line, "summary_id: ") || strings.HasPrefix(line, "children: ")):
				lines[i] = quotedArtifactID.ReplaceAllStringFunc(line, func(token string) string {
					return "\"" + portableArtifactID(token[1:len(token)-1], sourceID, localID) + "\""
				})
			}
		}
		result = []byte(strings.Join(lines, "\n"))
	}
	file := src + ".local"
	return file, os.WriteFile(file, result, 0o600)
}
