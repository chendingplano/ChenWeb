package buildinfo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

// EmbeddedMetadataBase64 is set by the Go linker for deployment builds.
var EmbeddedMetadataBase64 string
var processStartedAt = time.Now().UTC()

type Metadata struct {
	BuildNumber   string `json:"build_number"`
	BuildName     string `json:"build_name"`
	TargetMachine string `json:"target_machine"`
	OSInfo        string `json:"os_info"`
	Description   string `json:"description"`
}

type Process struct {
	Executable    string    `json:"executable"`
	Status        string    `json:"status"`
	PID           int       `json:"pid,omitempty"`
	BuildNumber   string    `json:"build_number,omitempty"`
	BuildName     string    `json:"build_name,omitempty"`
	TargetMachine string    `json:"target_machine,omitempty"`
	OSInfo        string    `json:"os_info,omitempty"`
	Description   string    `json:"description,omitempty"`
	StartedAt     time.Time `json:"started_at,omitempty"`
}

func EmbeddedMetadata() (Metadata, error) {
	var metadata Metadata
	if EmbeddedMetadataBase64 == "" {
		return metadata, fmt.Errorf("build metadata is not embedded")
	}
	data, err := base64.StdEncoding.DecodeString(EmbeddedMetadataBase64)
	if err != nil {
		return metadata, fmt.Errorf("decode build metadata: %w", err)
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return metadata, fmt.Errorf("parse build metadata: %w", err)
	}
	return metadata, nil
}

func Subject(executable string) string {
	return "chenweb.build-info." + executable
}

func CurrentProcess(executable string) (Process, error) {
	metadata, err := EmbeddedMetadata()
	if err != nil {
		return Process{}, err
	}
	return Process{
		Executable: executable, Status: "running", PID: os.Getpid(),
		BuildNumber: metadata.BuildNumber, BuildName: metadata.BuildName,
		TargetMachine: metadata.TargetMachine, OSInfo: metadata.OSInfo,
		Description: metadata.Description, StartedAt: processStartedAt,
	}, nil
}

func connectNATS() (*nats.Conn, error) {
	url := strings.TrimSpace(os.Getenv("NATS_URL"))
	if url == "" {
		url = "nats://127.0.0.1:4222"
	}
	opts := make([]nats.Option, 0, 1)
	if token := strings.TrimSpace(os.Getenv("NATS_TOKEN")); token != "" {
		opts = append(opts, nats.Token(token))
	} else {
		user := strings.TrimSpace(os.Getenv("NATS_USER"))
		password := strings.TrimSpace(os.Getenv("NATS_PASS"))
		if password == "" {
			password = strings.TrimSpace(os.Getenv("NATS_PASSWORD"))
		}
		if user != "" || password != "" {
			opts = append(opts, nats.UserInfo(user, password))
		}
	}
	return nats.Connect(url, opts...)
}

// StartResponder serves this executable's embedded build metadata on NATS.
func StartResponder(executable string) (func(), error) {
	if EmbeddedMetadataBase64 == "" {
		return func() {}, nil
	}
	if _, err := EmbeddedMetadata(); err != nil {
		return func() {}, err
	}
	nc, err := connectNATS()
	if err != nil {
		return func() {}, fmt.Errorf("connect build-info responder to NATS: %w", err)
	}
	_, err = nc.Subscribe(Subject(executable), func(msg *nats.Msg) {
		response, _ := CurrentProcess(executable)
		data, marshalErr := json.Marshal(response)
		if marshalErr == nil {
			_ = msg.Respond(data)
		}
	})
	if err != nil {
		nc.Close()
		return func() {}, fmt.Errorf("subscribe build-info responder: %w", err)
	}
	if err := nc.Flush(); err != nil {
		nc.Close()
		return func() {}, fmt.Errorf("flush build-info responder subscription: %w", err)
	}
	return func() { nc.Close() }, nil
}

// ListProcesses requests metadata from the running worker processes.
func ListProcesses(ctx context.Context, executables []string) []Process {
	processes := make([]Process, 0, len(executables))
	for _, executable := range executables {
		processes = append(processes, Process{Executable: executable, Status: "unreported"})
	}
	nc, err := connectNATS()
	if err != nil {
		return processes
	}
	defer nc.Close()
	var wg sync.WaitGroup
	for i, executable := range executables {
		wg.Add(1)
		go func(i int, executable string) {
			defer wg.Done()
			requestCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			msg, requestErr := nc.RequestWithContext(requestCtx, Subject(executable), nil)
			if requestErr != nil {
				return
			}
			var process Process
			if json.Unmarshal(msg.Data, &process) != nil || process.BuildNumber == "" {
				return
			}
			processes[i] = process
		}(i, executable)
	}
	wg.Wait()
	return processes
}
