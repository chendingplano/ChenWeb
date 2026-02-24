package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/chendingplano/deepdoc/server/api"
	"github.com/chendingplano/deepdoc/server/api/database"
	"github.com/chendingplano/deepdoc/server/cmd/config"
	shared_api "github.com/chendingplano/shared/go/api"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/databaseutil"
	sharedgoose "github.com/chendingplano/shared/go/api/goose"
	"github.com/chendingplano/shared/go/api/libmanager"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/chendingplano/shared/go/api/security"
	"github.com/chendingplano/shared/go/api/sysdatastores"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func ExitApp() {
	databaseutil.CloseDatabase()
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
		logger.Error("***** Alarm",
			"message", "Cound not load .env file",
			"error", err,
			"path", env_path,
			"loc", "CWB_DDM_200")

		// Try loading from current directory as fallback
		err = godotenv.Load()
		if err != nil {
			logger.Error("***** Alarm",
				"message", "Cound not load .env file from current directory",
				"error", err,
				"loc", "CWB_DDM_210")
		} else {
			logger.Info("load .env file from current directory",
				"loc", "CWB_DDM_212")
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

	err = config.LoadConfig(new_ctx, logger, "./config.toml")
	if err != nil {
		logger.Error("***** Alarm", "error", err, "loc", "CWB_DDM_075")
	}

	security.InitAccCtrlMgr()

	// load libconfig.toml
	// libmanager.LoadLibConfig(new_ctx, "../Shared/libconfig.toml")

	defer ExitApp()

	logger.Info("Starting server",
		"loc", "CWB_DDM_020)",
		"app_name", config.GlobalConfig.AppName,
		"host", config.GlobalConfig.Server.Host,
		"port", config.GlobalConfig.Server.Port)

	e := echo.New()

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
	db_type := ApiTypes.DatabaseInfo.DBType
	if err := databaseutil.InitDB(new_ctx, config.MySQLConfig, config.PGConfig); err != nil {
		logger.Error("Failed to initialize database", "error", err)
		os.Exit(2)
	}

	logger.Info("To Init DB ... Finished",
		"db_type", db_type,
		"loc", "CWB_DDM_111")

	// Make sure the db is created and valid.
	var db *sql.DB
	switch db_type {
	case ApiTypes.MysqlName:
		logger.Info("Set Mysql db (CWB_MAN_104)")
		db = ApiTypes.DatabaseInfo.MySQLDBHandle

	case ApiTypes.PgName:
		logger.Info("Set PG db (CWB_MAN_105)")
		db = ApiTypes.DatabaseInfo.PGDBHandle

	default:
		logger.Error("***** Alarm",
			"message", "unrecognized db_type",
			"db_type", db_type,
			"loc", "CWB_MAN_112")
		os.Exit(1)
	}

	if db == nil {
		logger.Error("***** Alarm",
			"message", "db is nil",
			"db_type", db_type,
			"loc", "CWB_MAN_077")
		os.Exit(1)
	}

	// Create tables
	if config.NeedCreateTables() {
		logger.Info("To Create Tables (CWB_DDM_036)")
		sysdatastores.CreateSysTables(logger)
		err = database.CreateTables(logger)
		if err != nil {
			logger.Error("***** Alarm",
				"message", "Failed creating tables",
				"error", err,
				"loc", "CWB_DDM_030")
		}
	} else {
		logger.Info("Not creating tables (CWB_DDM_041)")
	}

	migrate_cfg := config.GlobalConfig.Migration

	var project_db *sql.DB
	var shared_db *sql.DB
	dbType := ApiTypes.DatabaseInfo.DBType
	switch dbType {
	case ApiTypes.PgName:
		project_db = ApiTypes.PG_DB_Project
		shared_db = ApiTypes.PG_DB_Shared
	case ApiTypes.MysqlName:
		project_db = ApiTypes.MySql_DB_Project
		shared_db = ApiTypes.MySql_DB_Shared
	default:
		logger.Error("unsupported db type. System exit!!!!", "db_type", dbType)
		os.Exit(1)
	}

	if project_db == nil {
		logger.Error("project db is not set. System exit!!!!", "db_type", dbType)
		os.Exit(1)
	}

	if shared_db == nil {
		logger.Error("shared database connection is not set. System exit!!!!", "db_type", dbType)
		os.Exit(1)
	}

	err = sharedgoose.RunProjectMigrations(ctx, logger, migrate_cfg, project_db)
	if err != nil {
		logger.Error("failed to run project migrator. System exit!!!!", "error", err)
		os.Exit(1)
	}

	err = sharedgoose.RunSharedMigrations(ctx, logger, migrate_cfg, shared_db)
	if err != nil {
		logger.Error("failed to run shared migrator. System exit!!!!", "error", err)
		os.Exit(1)
	}

	logger.Info("Auto-test migrations completed successfully")

	// Init the library
	libmanager.InitLib(new_ctx, "../Shared/libconfig.toml", "CWB_MAN_157")

	logger.Info("Register Shared Routes (CWB_DDM_055)")
	shared_api.RegisterRoutes(e)

	var server_port = os.Getenv("SERVER_PORT")
	if server_port == "" {
		logger.Error("***** Alarm",
			"message", "missing SERVER_PORT env var",
			"loc", "CWB_MAN_145")
		os.Exit(1)
	}
	var pp = fmt.Sprintf(":%s", server_port)
	logger.Info("[API] ⇨ http server started on", "server_port", pp)
	e.Logger.Fatal(e.Start(pp))
}
