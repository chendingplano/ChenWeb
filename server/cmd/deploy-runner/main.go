// Command deploy-runner runs one build or deployment for the Deployment page and records
// its steps in kb.deployment_builds / kb.deployments / kb.deployment_steps.
//
//	deploy-runner build --build-id <id>   build machine; started detached by chenweb
//	deploy-runner deploy                  deployment machine; run by chenweb-deploy.service
//
// It runs from the ChenWeb root, wraps shell_server_build_rcp.sh or shell_server_deploy.sh,
// and turns their `::step::` marker lines into step rows. It runs outside chenweb so that a
// restart of chenweb (air on the build machine, the deploy itself on the deployment
// machine) does not stop it. It does not run migrations.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/chendingplano/deepdoc/server/api/deployment"
	"github.com/chendingplano/deepdoc/server/api/releaseshandler"
	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/joho/godotenv"
)

func usage() {
	fmt.Fprintln(os.Stderr, "usage: deploy-runner build --build-id <id> | deploy-runner deploy  [--config ./config.toml]")
	os.Exit(2)
}

func main() {
	if len(os.Args) < 2 || (os.Args[1] != deployment.KindBuild && os.Args[1] != deployment.KindDeploy) {
		usage()
	}
	kind := os.Args[1]
	fs := flag.NewFlagSet(kind, flag.ExitOnError)
	configPath := fs.String("config", "./config.toml", "path to config.toml")
	buildID := fs.Int64("build-id", 0, "kb.deployment_builds id (build only)")
	_ = fs.Parse(os.Args[2:])
	if kind == deployment.KindBuild && *buildID <= 0 {
		usage()
	}

	logger := loggerutil.CreateDefaultLogger("20261116-731")
	defer logger.Close()
	ctx := context.WithValue(context.Background(), ApiTypes.CallFlowKey, "20261116-732")

	// The caller's environment (mise on the build machine, the unit's EnvironmentFile on
	// the deployment machine) wins; .env only fills what is missing.
	_ = godotenv.Load("./.env")
	ApiUtils.LoadLibConfig("20261116-733")
	if err := config.LoadConfig(ctx, logger, *configPath); err != nil {
		logger.Error("deploy-runner: load config failed", "config", *configPath, "error", err)
		os.Exit(1)
	}
	if err := databaseutil.InitDB(ctx, ApiTypes.CommonConfig); err != nil {
		logger.Error("deploy-runner: init database failed", "error", err)
		os.Exit(1)
	}
	defer databaseutil.CloseDatabase(ApiTypes.CommonConfig)
	db := ApiTypes.ProjectDBHandle

	var run deployment.Run
	var cmd *exec.Cmd
	switch kind {
	case deployment.KindBuild:
		r, err := deployment.GetRun(ctx, db, kind, *buildID)
		if err != nil || r == nil || r.Status != deployment.StatusRunning {
			logger.Error("deploy-runner: build not found or not running", "build_id", *buildID, "error", err)
			os.Exit(1)
		}
		run = *r
		cmd = exec.Command("bash", "./shell_server_build_rcp.sh", run.BuildName, run.TargetMachine, run.OSInfo, run.Description)
	case deployment.KindDeploy:
		r, ok, err := deployment.ClaimDeployment(ctx, db, os.Getpid())
		if err != nil {
			logger.Error("deploy-runner: claim deployment failed", "error", err)
			os.Exit(1)
		}
		if !ok {
			logger.Info("deploy-runner: no queued deployment")
			return
		}
		run = r
		cmd = exec.Command("bash", "./shell_server_deploy.sh")
		cmd.Env = append(os.Environ(), "RELAY_ARCHIVE="+run.ArchiveName)
	}
	// Python buffers stdout in a pipe, which would attach its lines to a later step.
	cmd.Env = append(cmd.Environ(), "PYTHONUNBUFFERED=1")
	logger.Info("deploy-runner: started", "kind", kind, "id", run.ID, "build_number", run.BuildNumber,
		"archive", run.ArchiveName, "pid", os.Getpid())

	tracker := deployment.NewTracker(deployment.Plan(kind), time.Now)
	save := func(steps []deployment.Step) {
		if err := deployment.SaveSteps(ctx, db, kind, run.ID, steps); err != nil {
			logger.Error("deploy-runner: save steps failed", "kind", kind, "id", run.ID, "error", err)
		}
	}
	exitCode, cause := deployment.Execute(cmd, tracker, save, time.Second)
	archive := ""
	if kind == deployment.KindBuild && exitCode == 0 && cause == nil {
		archive = tracker.ArchiveName()
		if archive == "" {
			cause = fmt.Errorf("build script did not report the uploaded archive")
		}
	}
	status, errorMessage := tracker.Finish(exitCode, cause)
	save(tracker.TakeDirty())
	if err := deployment.FinishRun(ctx, db, kind, run.ID, status, errorMessage, archive); err != nil {
		logger.Error("deploy-runner: finish run failed", "kind", kind, "id", run.ID, "error", err)
		os.Exit(1)
	}
	if release := tracker.ReleaseVersion(); kind == deployment.KindBuild && release != "" {
		if err := deployment.SetBuildRelease(ctx, db, run.ID, release); err != nil {
			logger.Error("deploy-runner: record release version failed", "id", run.ID, "release", release, "error", err)
		}
	}
	if kind == deployment.KindDeploy && status == deployment.StatusSucceeded {
		recordRunningRelease(ctx, logger, db, run)
	}
	logger.Info("deploy-runner: finished", "kind", kind, "id", run.ID, "status", status,
		"exit_code", strconv.Itoa(exitCode), "archive", archive, "release", tracker.ReleaseVersion())
	if status != deployment.StatusSucceeded {
		os.Exit(1)
	}
}

// recordRunningRelease sets the release this machine now runs. The new server also records
// it at start from release/release.json, but a server from before release versioning
// (a rollback) cannot, so the runner, which is always the newer binary here, clears it.
func recordRunningRelease(ctx context.Context, logger ApiTypes.JimoLogger, db *sql.DB, run deployment.Run) {
	var err error
	if v, perr := releaseshandler.ParseVersion(run.ReleaseVersion); perr == nil {
		err = releaseshandler.SetCurrent(ctx, db, releaseshandler.ProjectChenWeb, v, run.BuildNumber)
	} else {
		err = releaseshandler.ClearCurrent(ctx, db, releaseshandler.ProjectChenWeb)
	}
	if err != nil {
		logger.Error("deploy-runner: record running release failed", "release", run.ReleaseVersion, "error", err)
		return
	}
	logger.Info("deploy-runner: running release recorded", "release", run.ReleaseVersion, "build_number", run.BuildNumber)
}
