package buildinfo

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Metadata struct {
	BuildNumber   string `json:"build_number"`
	BuildName     string `json:"build_name"`
	TargetMachine string `json:"target_machine"`
	OSInfo        string `json:"os_info"`
	Description   string `json:"description"`
}

type processRecord struct {
	Metadata
	ExecutablePath string    `json:"executable_path"`
	PID            int       `json:"pid"`
	StartedAt      time.Time `json:"started_at"`
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

// The shared registry lets the API report metadata owned by each live process.
func registryDir() string {
	if dir := strings.TrimSpace(os.Getenv("CHENWEB_BUILD_INFO_DIR")); dir != "" {
		return dir
	}
	return "/tmp/chenweb-build-info"
}

func WriteProcessInfo(executable, encodedMetadata string) error {
	if encodedMetadata == "" {
		return nil
	}
	data, err := base64.StdEncoding.DecodeString(encodedMetadata)
	if err != nil {
		return fmt.Errorf("decode build metadata: %w", err)
	}
	var metadata Metadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return fmt.Errorf("parse build metadata: %w", err)
	}

	path, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find process executable: %w", err)
	}
	record := processRecord{Metadata: metadata, ExecutablePath: path, PID: os.Getpid(), StartedAt: time.Now().UTC()}
	data, err = json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode process build info: %w", err)
	}
	dir := registryDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create process build info directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, executable+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create process build info file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write process build info: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close process build info: %w", err)
	}
	if err := os.Rename(tmpName, filepath.Join(dir, executable+".json")); err != nil {
		return fmt.Errorf("publish process build info: %w", err)
	}
	return nil
}

func ListProcesses(executables []string) []Process {
	processes := make([]Process, 0, len(executables))
	for _, executable := range executables {
		result := Process{Executable: executable, Status: "unreported"}
		data, err := os.ReadFile(filepath.Join(registryDir(), executable+".json"))
		if err != nil {
			processes = append(processes, result)
			continue
		}
		var record processRecord
		if json.Unmarshal(data, &record) != nil {
			processes = append(processes, result)
			continue
		}
		if !isRecordedProcessRunning(record) {
			result.Status = "stopped"
			processes = append(processes, result)
			continue
		}
		result.Status = "running"
		result.PID = record.PID
		result.BuildNumber = record.BuildNumber
		result.BuildName = record.BuildName
		result.TargetMachine = record.TargetMachine
		result.OSInfo = record.OSInfo
		result.Description = record.Description
		result.StartedAt = record.StartedAt
		processes = append(processes, result)
	}
	return processes
}

func isRecordedProcessRunning(record processRecord) bool {
	if runtime.GOOS != "linux" || record.PID < 1 {
		return false
	}
	executablePath, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", record.PID))
	if err != nil {
		return false
	}
	executablePath = strings.TrimSuffix(executablePath, " (deleted)")
	return filepath.Base(executablePath) == filepath.Base(record.ExecutablePath)
}

func RecordStartup(ctx context.Context, db *sql.DB, executable, encodedMetadata string) (Metadata, error) {
	var metadata Metadata
	if encodedMetadata == "" {
		return metadata, nil
	}
	data, err := base64.StdEncoding.DecodeString(encodedMetadata)
	if err != nil {
		return metadata, fmt.Errorf("decode build metadata: %w", err)
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return metadata, fmt.Errorf("parse build metadata: %w", err)
	}
	processErr := WriteProcessInfo(executable, encodedMetadata)

	_, err = db.ExecContext(ctx, `
		INSERT INTO kb.build_activities (
			build_number, build_name, executable, target_machine, os_info, description, activity
		) VALUES ($1, $2, $3, $4, $5, $6, 'started')`,
		metadata.BuildNumber, metadata.BuildName, executable,
		metadata.TargetMachine, metadata.OSInfo, metadata.Description,
	)
	if err != nil {
		err = fmt.Errorf("insert build activity: %w", err)
	}
	return metadata, errors.Join(processErr, err)
}
