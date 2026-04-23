package agentplatformhandler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/llm/agentrun"
)

// StartWorkers launches the agent-platform background workers and the
// lease sweeper. Intended to be called once from main.go AFTER migrations
// have completed.
//
// Behavior:
//   - If AGENT_PLATFORM_ENABLE_WORKERS != "true", this is a no-op (default;
//     lets the server run without Docker in dev).
//   - If AGENT_PLATFORM_FORCE_DRYRUN == "true", skips the Docker preflight
//     (the dryrun runner executes in-process).
//   - Otherwise, preflights `docker version` and returns an error if Docker
//     is unreachable — fail fast per design decision (§9.2).
//
// Workers run until ctx is canceled. Callers should pass a ctx that ties
// to shutdown (e.g. canceled on SIGINT/SIGTERM).
func StartWorkers(ctx context.Context, logger ApiTypes.JimoLogger, workerCount int) error {
	if os.Getenv("AGENT_PLATFORM_ENABLE_WORKERS") != "true" {
		logger.Info("agent-platform workers disabled (set AGENT_PLATFORM_ENABLE_WORKERS=true to enable)")
		return nil
	}
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return fmt.Errorf("StartWorkers: ProjectDBHandle is nil (call after InitDB)")
	}

	// Docker preflight (skipped in forced dryrun mode).
	if os.Getenv("AGENT_PLATFORM_FORCE_DRYRUN") != "true" {
		if err := agentrun.DockerAvailable(ctx); err != nil {
			return fmt.Errorf("agent-platform: docker preflight failed: %w", err)
		}
		logger.Info("agent-platform: docker preflight ok")
	} else {
		logger.Warn("agent-platform: AGENT_PLATFORM_FORCE_DRYRUN=true — Docker preflight skipped; all tasks use DryRunRunner")
	}

	workdirRoot := os.Getenv("AGENT_PLATFORM_WORKDIR_ROOT")
	if workdirRoot == "" {
		workdirRoot = filepath.Join(os.TempDir(), "chenweb-agentplatform", "workdirs")
	}
	if err := os.MkdirAll(workdirRoot, 0o755); err != nil {
		return fmt.Errorf("create workdir root %q: %w", workdirRoot, err)
	}
	logger.Info("agent-platform: workdir root", "path", workdirRoot)

	// Allow env override of worker count.
	if raw := os.Getenv("AGENT_PLATFORM_WORKER_COUNT"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			workerCount = n
		}
	}
	if workerCount <= 0 {
		workerCount = 2
	}

	for i := 0; i < workerCount; i++ {
		w := &worker{
			id:          i + 1,
			db:          db,
			workdirRoot: workdirRoot,
			logger:      logger,
		}
		go w.runLoop(ctx)
	}
	go leaseSweeper(ctx, db, logger)

	logger.Info("agent-platform workers started", "count", workerCount)
	return nil
}
