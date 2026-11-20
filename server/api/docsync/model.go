package docsync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	formatVersion     = 1
	documentItem      = "document"
	pipelineItem      = "doc_processing"
	pageSize          = 100
	checkpointOverlap = time.Hour
)

type cursor struct {
	Time time.Time `json:"time"`
	ID   int64     `json:"id"`
}

type listRequest struct {
	After      cursor    `json:"after"`
	Until      time.Time `json:"until"`
	IDs        []int64   `json:"ids,omitempty"`
	Vocabulary bool      `json:"vocabulary,omitempty"`
	All        bool      `json:"all,omitempty"`
}

type listing struct {
	Until  time.Time  `json:"until,omitempty"`
	Cursor *cursor    `json:"cursor,omitempty"`
	Issue  string     `json:"issue,omitempty"`
	Hash   string     `json:"md5,omitempty"`
	Items  []workItem `json:"items,omitempty"`
}

type workItem struct {
	SourceID      int64             `json:"source_id"`
	Kind          string            `json:"kind"`
	Hash          string            `json:"md5,omitempty"`
	EstimateBytes int64             `json:"estimated_bytes,omitempty"`
	Digest        string            `json:"digest"`
	SeenAt        time.Time         `json:"observed_at"`
	Status        json.RawMessage   `json:"status,omitempty"`
	Requires      map[string]string `json:"requires,omitempty"`
	Held          map[string]string `json:"held_files,omitempty"`
	Issue         string            `json:"issue,omitempty"`
}

type fileEntry struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Omit   bool   `json:"omitted,omitempty"`
}

type rowFile struct {
	fileEntry
	Rows int `json:"rows"`
}

type manifestItem struct {
	Work     workItem    `json:"work"`
	Files    []fileEntry `json:"files,omitempty"`
	Rows     int         `json:"rows"`
	Contract int         `json:"contract_version"`
	Data     []rowFile   `json:"data"`
}

type manifest struct {
	Version   int            `json:"format_version"`
	Exporter  string         `json:"exporter_version"`
	RunID     int64          `json:"run_id"`
	TarNumber int            `json:"tar_number"`
	Items     []manifestItem `json:"items"`
}

func normalizeMD5(hash string) string {
	hash = strings.ToLower(strings.TrimSpace(hash))
	if len(hash) != 32 {
		return ""
	}
	if _, err := hex.DecodeString(hash); err != nil {
		return ""
	}
	return hash
}

func digest(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}
