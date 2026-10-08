package kbhandler

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

type metricScoreFailure struct{ Code string }

func (e metricScoreFailure) Error() string { return e.Code }
func metricScoreErrorCode(err error) string {
	if err == nil {
		return ""
	}
	var failure metricScoreFailure
	if errors.As(err, &failure) {
		return failure.Code
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "TIMEOUT"
	}
	return "SCORING_FAILED"
}

func metricScoreScriptPath() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("METRIC_BENCHMARK_SCORER_PATH")); configured != "" {
		path, err := filepath.Abs(configured)
		if err == nil {
			_, err = os.Stat(path)
		}
		if err != nil {
			return "", metricScoreFailure{"SCORER_UNAVAILABLE"}
		}
		return path, nil
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", metricScoreFailure{"SCORER_UNAVAILABLE"}
	}
	for {
		path := filepath.Join(dir, ".agents", "skills", "score-extract-metrics", "scripts", "score_io.py")
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", metricScoreFailure{"SCORER_UNAVAILABLE"}
		}
		dir = parent
	}
}

func metricScoreDatabaseConfig() ApiTypes.DatabaseConfig {
	cfg := ApiTypes.CommonConfig.PGConf
	if cfg.UserName == "" {
		cfg.UserName = os.Getenv("PG_USER_NAME")
	}
	if cfg.Password == "" {
		cfg.Password = os.Getenv("PG_PASSWORD")
	}
	if cfg.ProjectDBName == "" {
		cfg.ProjectDBName = os.Getenv("PG_DB_NAME")
	}
	if cfg.Host == "" {
		cfg.Host = os.Getenv("PG_HOST")
	}
	if cfg.Port == 0 {
		cfg.Port, _ = strconv.Atoi(os.Getenv("PG_PORT"))
	}
	return cfg
}

func metricScoreCommandEnv() []string {
	env := os.Environ()
	// libpq address/service overrides could otherwise bypass the pool's configured host.
	for i := len(env) - 1; i >= 0; i-- {
		key := strings.SplitN(env[i], "=", 2)[0]
		if key == "PGHOSTADDR" || key == "PGSERVICE" || key == "PGSERVICEFILE" || key == "PGOPTIONS" {
			env = append(env[:i], env[i+1:]...)
		}
	}
	cfg := metricScoreDatabaseConfig()
	// Use the pool configuration, not inherited libpq defaults or the skill's .env fallback.
	for pg, value := range map[string]string{"PGUSER": cfg.UserName, "PGPASSWORD": cfg.Password, "PGDATABASE": cfg.ProjectDBName, "PGHOST": cfg.Host, "PGPORT": strconv.Itoa(cfg.Port)} {
		for i := len(env) - 1; i >= 0; i-- {
			if strings.HasPrefix(env[i], pg+"=") {
				env = append(env[:i], env[i+1:]...)
			}
		}
		env = append(env, pg+"="+value)
	}
	// Bound psql children as well as the Python parent. Never put credentials in argv.
	env = append(env, "PGCONNECT_TIMEOUT=10", "PGOPTIONS=-c statement_timeout=30000")
	return env
}

func runMetricScoreScript(ctx context.Context, args ...string) error {
	script, err := metricScoreScriptPath()
	if err != nil {
		return err
	}
	return runMetricScoreScriptAt(ctx, script, args...)
}

func runMetricScoreScriptAt(ctx context.Context, script string, args ...string) error {
	cmd := exec.CommandContext(ctx, "python3", append([]string{script}, args...)...)
	cmd.Env = metricScoreCommandEnv()
	// No subprocess output is sent to the browser or logs: export diagnostics may include DB details.
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) && len(args) > 0 && args[0] == "export" {
			switch exit.ExitCode() {
			case 3:
				return metricScoreFailure{"NO_GOLD"}
			case 4:
				return metricScoreFailure{"NO_PREDICTIONS"}
			}
		}
		if errors.Is(err, exec.ErrNotFound) {
			return metricScoreFailure{"SCORER_UNAVAILABLE"}
		}
		if len(args) > 0 && args[0] == "score" {
			return metricScoreFailure{"INVALID_MATCHES"}
		}
		return metricScoreFailure{"EXPORT_FAILED"}
	}
	return nil
}

type metricScoreEvidence struct {
	Model   string
	Input   json.RawMessage
	Matches json.RawMessage
	Score   json.RawMessage
	Report  string
}

func buildMetricScoreInput(snapshot json.RawMessage, lines []rawLine, lang string) (string, error) {
	var text strings.Builder
	fmt.Fprintf(&text, "output_language: %s\nSCORING_INPUT\n%s\nSOURCE_LINES\n", lang, snapshot)
	for _, line := range lines {
		fmt.Fprintf(&text, "L%d\t%s\t%s\n", line.LineNumber, line.LineType, line.Content)
	}
	if text.Len() > metricReviewMaxInputChars {
		return "", metricScoreFailure{"INPUT_TOO_LARGE"}
	}
	return text.String(), nil
}

func metricScoreDigest(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }

// Freeze both scorer and shared gold helper. The saved implementation can reproduce this run
// even if the installed skills change while matching is in progress or after completion.
func freezeMetricScoreTools(root, script string) (string, map[string]string, error) {
	helper := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(script))), "extract-metrics-benchmark", "scripts", "benchmark_io.py")
	hashes := map[string]string{}
	for name, source := range map[string]string{"score-extract-metrics/scripts/score_io.py": script, "extract-metrics-benchmark/scripts/benchmark_io.py": helper} {
		raw, err := os.ReadFile(source)
		if err != nil {
			return "", nil, metricScoreFailure{"SCORER_UNAVAILABLE"}
		}
		target := filepath.Join(root, "skills", name)
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return "", nil, err
		}
		if err = os.WriteFile(target, raw, 0600); err != nil {
			return "", nil, err
		}
		hashes[name] = metricScoreDigest(raw)
	}
	return filepath.Join(root, "skills", "score-extract-metrics", "scripts", "score_io.py"), hashes, nil
}

func executeMetricScore(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, id int64, req metricScoreRequest, userID string) (e metricScoreEvidence, err error) {
	e.Model = req.Model
	script, err := metricScoreScriptPath()
	if err != nil {
		return e, err
	}
	dataHome := strings.TrimSpace(os.Getenv("DATA_HOME_DIR"))
	if dataHome == "" {
		return e, metricScoreFailure{"DATA_HOME_UNAVAILABLE"}
	}
	// Pool and scorer must address the same configured project database.
	var poolDatabase, poolUser string
	if err = db.QueryRowContext(ctx, `SELECT current_database(),current_user`).Scan(&poolDatabase, &poolUser); err != nil {
		return e, err
	}
	poolConfig := metricScoreDatabaseConfig()
	if poolDatabase != poolConfig.ProjectDBName || poolUser != poolConfig.UserName {
		return e, metricScoreFailure{"DATABASE_MISMATCH"}
	}
	root := filepath.Join(dataHome, "metric-score-runs", strconv.FormatInt(id, 10))
	if err = os.MkdirAll(root, 0700); err != nil {
		return e, err
	}
	frozen, hashes, err := freezeMetricScoreTools(root, script)
	if err != nil {
		return e, err
	}
	// export creates this directory itself with exist_ok=False.
	dir := filepath.Join(root, "evidence")
	args := []string{"export", "--record-id", strconv.FormatInt(req.RecordID, 10), "--out-dir", dir}
	if req.GoldRunID != "" {
		args = append(args, "--gold-version", req.GoldVersion, "--gold-model", req.GoldModel, "--gold-run-id", req.GoldRunID)
	}
	if err = runMetricScoreScriptAt(ctx, frozen, args...); err != nil {
		return e, err
	}
	e.Input, err = os.ReadFile(filepath.Join(dir, "input.json"))
	if err != nil {
		return e, err
	}
	var input map[string]any
	if err = json.Unmarshal(e.Input, &input); err != nil {
		return e, err
	}
	prompt, err := os.ReadFile(filepath.Join("prompts", metricScorePrompt))
	if err != nil {
		return e, metricScoreFailure{"PROMPT_UNAVAILABLE"}
	}
	hashes[metricScorePrompt] = metricScoreDigest(prompt)
	if err = os.WriteFile(filepath.Join(root, metricScorePrompt), prompt, 0600); err != nil {
		return e, err
	}
	cfg, client, err := newMetricReviewLLMClient(logger, req.Model)
	if err != nil {
		return e, metricScoreFailure{"MODEL_UNAVAILABLE"}
	}
	e.Model = cfg.ModelName
	if goldRun, ok := input["gold_run"].(map[string]any); ok && goldRun["model_name"] == cfg.ModelName {
		warnings, _ := input["warnings"].([]any)
		input["warnings"] = append(warnings, "Gold and matching use the same model; use another scoring model for an independent check")
	}
	var lines []rawLine
	source, _ := input["source"].(map[string]any)
	sourcePath, _ := source["path"].(string)
	if sourcePath != "" {
		// Keep exactly the canonical source bytes used for the matcher and their digest.
		raw, readErr := os.ReadFile(sourcePath)
		if readErr == nil {
			if len(raw) > metricReviewMaxInputChars {
				return e, metricScoreFailure{"INPUT_TOO_LARGE"}
			}
			if hash := metricScoreDigest(raw); hash != source["current_sha256"] {
				source["current_sha256"] = hash
				source["lines_comparable"] = false
				warnings, _ := input["warnings"].([]any)
				input["warnings"] = append(warnings, "Source changed after export; line checks disabled")
			}
			input["source_text"] = string(raw)
			for _, physical := range strings.Split(string(raw), "\n") {
				if line, ok := parseRawLine(physical); ok {
					lines = append(lines, line)
				}
			}
		} else {
			source["lines_comparable"] = false
			warnings, _ := input["warnings"].([]any)
			input["warnings"] = append(warnings, "Source could not be read for matching; line checks disabled, matching uses metric content")
		}
	}
	input["provenance"] = map[string]any{"implementation_sha256": hashes, "prompt_name": metricScorePrompt, "model_profile": req.Model, "scorer_model": cfg.ModelName, "lang": req.Lang}
	e.Input, err = json.MarshalIndent(input, "", "  ")
	if err != nil {
		return e, err
	}
	if err = os.WriteFile(filepath.Join(dir, "input.json"), e.Input, 0600); err != nil {
		return e, err
	}
	// source_text is already in the immutable snapshot; do not duplicate it in the LLM input.
	matchInput := e.Input
	if _, ok := input["source_text"]; ok {
		delete(input, "source_text")
		matchInput, err = json.Marshal(input)
		if err != nil {
			return e, err
		}
	}
	inputText, err := buildMetricScoreInput(matchInput, lines, req.Lang)
	if err != nil {
		return e, err
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("MODEL_DEFAULT_REASONING_POLICY")), "no-reasoning") {
		client.ThinkingType = "disabled"
	}
	logger.Info("calling LLM for metric benchmark matching", "run_id", id, "record_id", req.RecordID, "model", cfg.ModelName, "input_bytes", len(inputText))
	payload, err := client.ExtractJSON(ctx, llmclients.JSONExtractionInput{
		UserID: userID, PromptName: metricScorePrompt, PromptText: string(prompt), ModelName: cfg.ModelName,
		InputText: inputText, RecordID: req.RecordID, CallReason: "score_extract_metrics", CallLoc: "MID-20261007-04",
	})
	if err != nil {
		if ctx.Err() != nil {
			return e, ctx.Err()
		}
		return e, metricScoreFailure{"LLM_FAILED"}
	}
	e.Matches, err = json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return e, err
	}
	if err = os.WriteFile(filepath.Join(dir, "matches.json"), e.Matches, 0600); err != nil {
		return e, err
	}
	// Python refuses missing, duplicate or unknown row IDs and prohibited overrides.
	if err = runMetricScoreScriptAt(ctx, frozen, "score", "--dir", dir, "--scorer-model", cfg.ModelName); err != nil {
		return e, err
	}
	e.Score, err = os.ReadFile(filepath.Join(dir, "score.json"))
	if err != nil {
		return e, err
	}
	rawReport, err := os.ReadFile(filepath.Join(dir, "report.md"))
	if err != nil {
		return e, err
	}
	e.Report = string(rawReport)
	return e, nil
}
