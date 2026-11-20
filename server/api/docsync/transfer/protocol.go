// Package transfer implements the ChenWeb document export wire protocol.
package transfer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	StateMissing = "missing"
	StatePacking = "packing"
	StateReady   = "ready"
	StateFailed  = "failed"

	// SessionExitCode tells the client that it needs to authenticate again.
	SessionExitCode = 23
	maxRequestSize  = 64 << 20
)

var (
	ErrSession       = errors.New("document sync requires an active remote session")
	ErrSSHConnection = errors.New("document sync SSH connection failed")
	errCommand       = errors.New("unsupported document export command")
)

type PackRequest struct {
	RunID        int64           `json:"run_id"`
	TarNumber    int             `json:"tar_number"`
	MaxBytes     int64           `json:"max_bytes"`
	SegmentBytes int64           `json:"segment_bytes"`
	Work         json.RawMessage `json:"work"`
}

type PackResult struct {
	Included json.RawMessage `json:"included,omitempty"`
	Deferred json.RawMessage `json:"deferred,omitempty"`
}

type Index struct {
	RunID         int64    `json:"run_id"`
	TarNumber     int      `json:"tar_number,omitempty"`
	Name          string   `json:"name,omitempty"`
	State         string   `json:"state"`
	Size          int64    `json:"size,omitempty"`
	SHA256        string   `json:"sha256,omitempty"`
	SegmentSize   int64    `json:"segment_size,omitempty"`
	SegmentSHA256 []string `json:"segment_sha256,omitempty"`
	Error         string   `json:"error,omitempty"`
	PackResult
}

type Callbacks struct {
	List func(context.Context, json.RawMessage, io.Writer) error
	Pack func(context.Context, PackRequest, string) (PackResult, error)
}

type envelope struct {
	Token string          `json:"token"`
	Body  json.RawMessage `json:"body,omitempty"`
}

type loginRequest struct {
	Phone    string `json:"phone"`
	Password string `json:"password"`
}

type loginReply struct {
	Token string `json:"token"`
}

type command struct {
	Action    string
	RunID     int64
	TarNumber int
	Name      string
	Segment   int
	Offset    int64
}

func archiveName(run int64, number int) string {
	return fmt.Sprintf("%d-%04d.tar.gz", run, number)
}

func decimal(s string, min int64) (int64, bool) {
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil && n >= min && strconv.FormatInt(n, 10) == s
}

func parseCommand(input string) (command, error) {
	if len(input) > 512 {
		return command{}, errCommand
	}
	p := strings.Fields(input)
	if len(p) < 2 || p[0] != "export" {
		return command{}, errCommand
	}
	c := command{Action: p[1]}
	switch c.Action {
	case "login", "logout", "list":
		if len(p) != 2 {
			return command{}, errCommand
		}
		return c, nil
	case "pack", "status", "fetch", "ack":
		if len(p) < 4 || p[2] != "--run" {
			return command{}, errCommand
		}
	default:
		return command{}, errCommand
	}
	var ok bool
	if c.RunID, ok = decimal(p[3], 1); !ok {
		return command{}, errCommand
	}
	if c.Action == "status" || c.Action == "ack" && len(p) == 4 {
		if len(p) != 4 {
			return command{}, errCommand
		}
		return c, nil
	}
	if len(p) < 6 || p[4] != "--tar" {
		return command{}, errCommand
	}
	if c.Action == "pack" {
		number, ok := decimal(p[5], 1)
		if len(p) != 6 || !ok || number > 1<<31-1 {
			return command{}, errCommand
		}
		c.TarNumber = int(number)
		c.Name = archiveName(c.RunID, c.TarNumber)
		return c, nil
	}
	prefix := strconv.FormatInt(c.RunID, 10) + "-"
	suffix := ".tar.gz"
	if !strings.HasPrefix(p[5], prefix) || !strings.HasSuffix(p[5], suffix) {
		return command{}, errCommand
	}
	number, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(p[5], prefix), suffix), 10, 32)
	if err != nil || number < 1 || p[5] != archiveName(c.RunID, int(number)) {
		return command{}, errCommand
	}
	c.Name, c.TarNumber = p[5], int(number)
	if c.Action == "ack" && len(p) == 6 {
		return c, nil
	}
	if c.Action != "fetch" || len(p) != 10 || p[6] != "--segment" || p[8] != "--offset" {
		return command{}, errCommand
	}
	segment, ok := decimal(p[7], 0)
	if !ok || segment > 1<<31-1 {
		return command{}, errCommand
	}
	if c.Offset, ok = decimal(p[9], 0); !ok {
		return command{}, errCommand
	}
	c.Segment = int(segment)
	return c, nil
}

func readJSON(r io.Reader, v any) error {
	l := &io.LimitedReader{R: r, N: maxRequestSize + 1}
	d := json.NewDecoder(l)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return errors.New("invalid export JSON message")
	}
	if err := d.Decode(new(json.RawMessage)); err != io.EOF || l.N == 0 {
		return errors.New("invalid export JSON message size or trailing data")
	}
	return nil
}

func hashHex(r io.Reader) (string, error) {
	h := sha256.New()
	_, err := io.Copy(h, r)
	return hex.EncodeToString(h.Sum(nil)), err
}

func validDigest(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == sha256.Size && s == strings.ToLower(s)
}

func (i Index) validateReady() error {
	if i.State != StateReady || i.RunID <= 0 || i.TarNumber <= 0 || i.Name != archiveName(i.RunID, i.TarNumber) ||
		i.Size <= 0 || i.SegmentSize <= 0 || int64(len(i.SegmentSHA256)) != 1+(i.Size-1)/i.SegmentSize || !validDigest(i.SHA256) {
		return errors.New("invalid archive index")
	}
	for _, digest := range i.SegmentSHA256 {
		if !validDigest(digest) {
			return errors.New("invalid segment hash in archive index")
		}
	}
	return nil
}
