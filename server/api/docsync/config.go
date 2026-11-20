package docsync

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/docsync/transfer"
	"github.com/chendingplano/deepdoc/server/api/pathutil"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/pelletier/go-toml/v2"
)

type pullConfig struct {
	Host     string `toml:"remote_ip"`
	Port     int    `toml:"ssh_port"`
	User     string `toml:"user_name"`
	Phone    string `toml:"remote_user_phone"`
	Password string `toml:"remote_user_password"`
	Store    int64  `toml:"knowledge_store_id"`
	Start    string `toml:"sync_start_time"`
	End      string `toml:"sync_end_time"`
	Max      int64  `toml:"tar_file_max_size"`
	Segment  int64  `toml:"tar_file_segment_size"`
	Throttle int64  `toml:"max_bytes_per_second"`
}

func LoadRunner(path string, logger ApiTypes.JimoLogger) (*Runner, error) {
	cfg := struct {
		Section pullConfig `toml:"data-sync"`
	}{pullConfig{Port: 22, Start: "1am", End: "5am", Max: 5000, Segment: 5}}
	for _, filename := range []string{path, filepath.Join(filepath.Dir(path), "config.local.toml")} {
		raw, err := os.ReadFile(filename)
		if errors.Is(err, fs.ErrNotExist) && filename != path {
			continue
		}
		if err != nil {
			return nil, err
		}
		if err := toml.Unmarshal(raw, &cfg); err != nil {
			return nil, fmt.Errorf("invalid document sync TOML in %s", filepath.Base(filename))
		}
	}
	s := cfg.Section
	if strings.TrimSpace(s.Host) == "" {
		return nil, nil
	}
	const megabyte int64 = 1_000_000
	if s.Max <= 0 || s.Max > math.MaxInt64/megabyte/3 || s.Segment <= 0 || s.Segment > s.Max || s.Throttle < 0 ||
		s.Port < 1 || s.Port > 65535 || s.User == "" || s.Phone == "" || s.Password == "" || s.Store <= 0 {
		return nil, errors.New("invalid or missing document sync settings in [data-sync]")
	}
	start, err := ParseWallTime(s.Start)
	if err != nil {
		return nil, err
	}
	end, err := ParseWallTime(s.End)
	if err != nil {
		return nil, err
	}
	ssh := transfer.SSHConfig{Host: s.Host, Port: s.Port, User: s.User, MaxBytesPerSecond: s.Throttle, Logger: logger}
	return &Runner{
		Logger: logger, SSH: ssh, Site: SiteKey(ssh), RemotePhone: strings.TrimSpace(s.Phone), Password: s.Password, StoreID: s.Store,
		AdminURL: strings.TrimSpace(os.Getenv("KRATOS_ADMIN_URL")), ArtifactRoot: strings.TrimSpace(os.Getenv("ARTIFACT_DIR")),
		Inbox: filepath.Join(pathutil.DataHomeDir(), ".docsync-inbox"), Window: Window{Start: start, End: end},
		MaxBytes: s.Max * megabyte, SegmentBytes: s.Segment * megabyte,
	}, nil
}
