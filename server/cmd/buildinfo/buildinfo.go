package buildinfo

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

type Metadata struct {
	BuildNumber   string `json:"build_number"`
	BuildName     string `json:"build_name"`
	TargetMachine string `json:"target_machine"`
	OSInfo        string `json:"os_info"`
	Description   string `json:"description"`
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

	_, err = db.ExecContext(ctx, `
		INSERT INTO kb.build_activities (
			build_number, build_name, executable, target_machine, os_info, description, activity
		) VALUES ($1, $2, $3, $4, $5, $6, 'started')`,
		metadata.BuildNumber, metadata.BuildName, executable,
		metadata.TargetMachine, metadata.OSInfo, metadata.Description,
	)
	if err != nil {
		return metadata, fmt.Errorf("insert build activity: %w", err)
	}
	return metadata, nil
}
