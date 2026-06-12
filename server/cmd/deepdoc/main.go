package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/chendingplano/deepdoc/server/api"
	"github.com/chendingplano/deepdoc/server/api/agentplatformhandler"
	"github.com/chendingplano/deepdoc/server/api/database"
	"github.com/chendingplano/deepdoc/server/api/docgenworker"
	"github.com/chendingplano/deepdoc/server/api/kbhandler"
	"github.com/chendingplano/deepdoc/server/api/promptoptimizerhandler"
	"github.com/chendingplano/deepdoc/server/cmd/config"
	shared_api "github.com/chendingplano/shared/go/api"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/libmanager"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/chendingplano/shared/go/api/observability"
	"github.com/chendingplano/shared/go/api/security"
	"github.com/chendingplano/shared/go/api/sysdatastores"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func ExitApp() {
	databaseutil.CloseDatabase(ApiTypes.CommonConfig)
	libmanager.ExitLib()
}

func main() {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	new_ctx := context.WithValue(ctx, ApiTypes.CallFlowKey, "CWB_DDM_035")

	// Load .env from project root (../../.env relative to cmd/mirai)
	// err := godotenv.Load("../../../.env")
	env_path := "./.env"
	err := godotenv.Load(env_path)

	ApiUtils.LoadLibConfig("CWB_0207150500")
	var logger = loggerutil.CreateDefaultLogger("CWB_MAN_032")
	logger.Info("load .env", "loc", "(CWB_DDM_198)")

	if err != nil {
		logger.Error("Cound not load .env file", "error", err, "path", env_path)

		// Try loading from current directory as fallback
		err = godotenv.Load()
		if err != nil {
			logger.Error("Cound not load .env file from current directory", "error", err)
		} else {
			logger.Info("load .env file from current directory")
		}
	}

	// Register custom email sender for Shared project
	// This makes password reset emails use our branded template and Resend service
	// ApiUtils.SetEmailSender(createMiraiEmailSender())

	// Verify Google OAuth environment variables are loaded
	logger.Info("Verifying Google OAuth env vars (CWB_DMN_052):")
	logger.Info("  GOOGLE_OAUTH_CLIENT_ID", "value", os.Getenv("GOOGLE_OAUTH_CLIENT_ID"))
	logger.Info("  GOOGLE_OAUTH_REDIRECT_URL", "value", os.Getenv("GOOGLE_OAUTH_REDIRECT_URL"))
	logger.Info("  VITE_DEV_ONLY_URL", "value", os.Getenv("VITE_DEV_ONLY_URL"))

	configPath := "./config.toml"
	err = config.LoadConfig(new_ctx, logger, configPath)
	if err != nil {
		logger.Error("failed loading config 'config.toml'", "error", err)
	}
	config.NormalizeMigrationPaths(logger, configPath)

	security.InitAccCtrlMgr()

	// load libconfig.toml
	// libmanager.LoadLibConfig(new_ctx, "../Shared/libconfig.toml")

	defer ExitApp()

	logger.Info("Starting server",
		"loc", "CWB_DDM_020)",
		"app_name", ApiTypes.CommonConfig.AppInfo.AppName,
		"host", ApiTypes.CommonConfig.AppInfo.AppHost,
		"port", ApiTypes.CommonConfig.AppInfo.AppPort)

	e := echo.New()
	obsCfg := observability.ConfigFromEnv("chenweb")
	obsShutdown, err := observability.Init(ctx, obsCfg)
	if err != nil {
		logger.Error("failed to initialize observability", "error", err, "enabled", obsCfg.Enabled)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := obsShutdown(shutdownCtx); err != nil {
			logger.Warn("failed to shutdown observability", "error", err)
		}
	}()
	if obsCfg.Enabled {
		logger.Info("observability enabled",
			"service", obsCfg.ServiceName,
			"environment", obsCfg.Environment,
			"otlp_endpoint", obsCfg.OTLPEndpoint)
	} else {
		logger.Info("observability disabled")
	}
	e.Use(observability.RequestMiddleware(obsCfg))

	// ✅ Enable CORS
	logger.Info("Configure CORS", "loc", "CWB_DDM_045")
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     []string{"http://localhost:5173"}, // frontend
		AllowMethods:     []string{echo.GET, echo.POST, echo.PUT, echo.DELETE, echo.OPTIONS},
		AllowHeaders:     []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
		AllowCredentials: true,
	}))

	logger.Info("Register api Routes", "loc", "CWB_DDM_052")
	err = api.RegisterRoutes(e)
	if err != nil {
		panic(err)
	}

	logger.Info("To Init DB", "loc", "CWB_DDM_095")
	db_type := ApiTypes.CommonConfig.AppInfo.DatabaseType
	if err := databaseutil.InitDB(new_ctx, ApiTypes.CommonConfig); err != nil {
		logger.Error("Failed to initialize database", "error", err)
		os.Exit(2)
	}

	logger.Info("To Init DB ... Finished",
		"db_type", db_type,
		"loc", "CWB_DDM_111")

	// Make sure the db is created and valid.
	var db = ApiTypes.ProjectDBHandle
	if db == nil {
		logger.Error("ProjectDBHandle is nil", "db_type", db_type)
		os.Exit(1)
	}

	// Create tables
	if config.NeedCreateTables() {
		logger.Info("To Create Tables (CWB_DDM_036)")
		err = sysdatastores.CreateSysTables(logger)
		if err != nil {
			logger.Error("failed creating system tables", "error", err)
			os.Exit(1)
		}

		err = database.CreateTables(logger)
		if err != nil {
			logger.Error("failed creating tables", "error", err)
			os.Exit(1)
		}
	} else {
		logger.Warn("Not creating tables")
	}

	var project_db *sql.DB
	dbType := ApiTypes.CommonConfig.AppInfo.DatabaseType

	// Currently, only PG is supported
	if dbType != ApiTypes.PgName {
		logger.Error("unsupported db type. System exit!!!!", "db_type", dbType)
		os.Exit(1)
	}

	project_db = ApiTypes.ProjectDBHandle
	if project_db == nil {
		logger.Error("project db is not set. System exit!!!!", "db_type", dbType)
		os.Exit(1)
	}

	logger.Info("Start Project Migrations")
	migrationCtx, migrationCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer migrationCancel()
	err = config.RunMigrations(migrationCtx, logger)
	if err != nil {
		logger.Error("failed to run migrations. System exit!!!!", "error", err)
		os.Exit(1)
	}

	logger.Info("migrations completed successfully")

	if err := kbhandler.CheckSearchBackend(project_db); err != nil {
		logger.Error("search backend check failed; system exit", "error", err, "loc", "CWB_DDM_200")
		os.Exit(1)
	}
	logger.Info("search backend verified", "loc", "CWB_DDM_201")

	// Prompt Optimizer initialization: load the AES-GCM key used to encrypt
	// per-user API keys at rest, then seed the built-in templates so every
	// user has a usable starter set on first visit.
	poEncKey, err := security.LoadKeyFromEnv("PROMPT_OPTIMIZER_ENCRYPTION_KEY")
	if err != nil {
		logger.Error("failed to load PROMPT_OPTIMIZER_ENCRYPTION_KEY",
			"error", err, "loc", "CWB_DDM_165")
		os.Exit(1)
	}
	if err := promptoptimizerhandler.Init(poEncKey); err != nil {
		logger.Error("failed to init promptoptimizerhandler",
			"error", err, "loc", "CWB_DDM_166")
		os.Exit(1)
	}

	// Agent-platform background workers (M1b). No-op unless
	// AGENT_PLATFORM_ENABLE_WORKERS=true; fails fast if Docker is unreachable
	// (unless AGENT_PLATFORM_FORCE_DRYRUN=true, which keeps execution in-process).
	if err := agentplatformhandler.StartWorkers(context.Background(), logger, 2); err != nil {
		logger.Error("failed to start agent-platform workers",
			"error", err, "loc", "CWB_DDM_169")
		os.Exit(1)
	}
	if err := promptoptimizerhandler.SeedBuiltinTemplates(project_db); err != nil {
		logger.Error("failed to seed prompt optimizer built-in templates",
			"error", err, "loc", "CWB_DDM_167")
		// non-fatal: users can still create their own templates
	} else {
		logger.Info("prompt optimizer built-in templates seeded", "loc", "CWB_DDM_168")
	}

	/*
		parserCfg := config.GetPDFParserConfig()
		if parserCfg.Enabled {
			parserServiceCfg := pdfparser.Config{
				PollInterval:      time.Duration(parserCfg.PollIntervalSeconds) * time.Second,
				BatchSize:         parserCfg.BatchSize,
				StagingDir:        parserCfg.StagingDir,
				RepoDirs:          parserCfg.RepoDirs,
				BackupDir:         parserCfg.BackupDir,
				PythonBin:         parserCfg.PythonBin,
				PaddleOCRScript:   parserCfg.PaddleOCRScript,
				UsePaddleOCRVL:    parserCfg.UsePaddleOCRVL,
				DeleteFromStaging: parserCfg.DeleteFromStaging,
				WorkDir:           parserCfg.WorkDir,
			}

			pdfService, err := pdfparser.NewService(parserServiceCfg)
			if err != nil {
				logger.Error("failed to initialize PDF parser service", "error", err)
				os.Exit(1)
			}
			go func() {
				if runErr := pdfService.Run(context.Background(), logger); runErr != nil {
					logger.Error("PDF parser service exited", "error", runErr)
				}
			}()
			logger.Info("PDF parser service started", "staging_dir", parserCfg.StagingDir)
		} else {
			logger.Info("PDF parser service disabled by config")
		}
	*/

	// Init the library
	logger.Info("InitLib")
	libmanager.InitLib(new_ctx, "../Shared/libconfig.toml", "CWB_MAN_157")

	logger.Info("Register Shared Routes (CWB_DDM_055)")
	shared_api.RegisterRoutes(e)

	var server_port = ApiTypes.CommonConfig.AppInfo.AppPort
	if server_port <= 0 {
		logger.Error("",
			"message", "missing SERVER_PORT env var",
			"loc", "CWB_MAN_145")
		os.Exit(1)
	}
	var pp = fmt.Sprintf(":%d", server_port)

	// Start doc generation worker pool
	docGenCfg := config.GetDocGenConfig()
	workerCount := docGenCfg.WorkerCount
	if workerCount <= 0 {
		workerCount = 3
	}
	docgenJobCh := make(chan int64, 100)
	docgenworker.Start(project_db, docgenJobCh, workerCount)
	go func() {
		if err := docgenworker.RequeueStalledJobs(project_db, docgenJobCh); err != nil {
			logger.Warn("failed to requeue stalled doc gen jobs", "err", err)
		}
	}()
	logger.Info("doc gen worker pool started", "workers", workerCount)

	logger.Info("[API] ⇨ http server started on", "server_port", pp)
	e.Logger.Fatal(e.Start(pp))
}
