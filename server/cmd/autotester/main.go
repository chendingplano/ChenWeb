package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/autotester"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/goose"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

func main() {
	// Define command-line flags
	purposes := flag.String("purpose", "", "Comma-separated test purposes to run")
	types := flag.String("type", "", "Comma-separated test types to run")
	tags := flag.String("tags", "", "Comma-separated tags to include")
	testerNames := flag.String("tester", "", "Comma-separated Tester names to run")
	testIDs := flag.String("test-id", "", "Comma-separated TestCase IDs to run")
	seed := flag.Int64("seed", 0, "Random seed (0 = auto-generate)")
	parallel := flag.Bool("parallel", false, "Enable parallel Tester execution")
	maxParallel := flag.Int("max-parallel", 4, "Maximum concurrent Testers")
	retryCount := flag.Int("retry", 0, "Retry count for failed cases")
	caseTimeout := flag.Duration("case-timeout", 30*time.Second, "Per-case timeout")
	runTimeout := flag.Duration("run-timeout", 30*time.Minute, "Overall run timeout")
	stopOnFail := flag.Bool("stop-on-fail", false, "Stop on first failure")
	skipCleanup := flag.Bool("skip-cleanup", false, "Skip Cleanup (for debugging)")
	verbose := flag.Bool("verbose", false, "Verbose logging")
	jsonReport := flag.String("json-report", "", "Write JSON report to this file")
	env := flag.String("env", "local", "Environment: local|test|staging")
	flag.Parse()

	// Load .env from current working directory (project root)
	err := godotenv.Load()
	if err != nil {
		slog.Error("Could not load .env file", "error", err)
	}

	ApiUtils.LoadLibConfig("MID_26022804")

	logger := loggerutil.CreateDefaultLogger("AUTO_TESTER")

	// Load config
	logger.Info("Step 1 Read Config")
	ctx := context.Background()
	configPath := "config.toml" // Default config path
	if err := LoadConfig(ctx, logger, configPath); err != nil {
		logger.Error("Config load failed", "error", err)
		os.Exit(2)
	}
	migrate_cfg := ApiTypes.CommonConfig.MigrationConfig
	logger.Info("Config loaded successfully",
		"migration_fs", migrate_cfg.MigrationsFS,
		"migration_dir", migrate_cfg.MigrationsDir,
		"tablename", migrate_cfg.TableName,
		"verbose", migrate_cfg.Verbose,
		"allow_outof_order", migrate_cfg.AllowOutOfOrder)

	// Safety check: refuse to run against production
	logger.Info("Step 2 Check ProductionDB")
	if isProductionDB() {
		logger.Error("Refusing to run AutoTester against production database")
		os.Exit(2)
	}

	// Init database
	logger.Info("Step 3 InitDB")
	new_ctx := context.WithValue(ctx, ApiTypes.CallFlowKey, "SHD_0220185500")
	if err := databaseutil.InitDB(new_ctx, ApiTypes.CommonConfig); err != nil {
		logger.Error("Failed to initialize database", "error", err)
		os.Exit(2)
	}

	// Verify database connection
	if ApiTypes.CommonConfig.PGConf.ProjectDBHandle == nil {
		logger.Error("Project Database connection not initialized")
		os.Exit(2)
	}

	if ApiTypes.CommonConfig.PGConf.AutotesterDBHandle == nil {
		logger.Error("Autotester DB connection not initialized")
		os.Exit(2)
	}

	// Run auto-test migrations
	logger.Info("Step 4 Run Migrations")
	if err := runAutoTestMigrations(ctx, logger); err != nil {
		logger.Error("Failed to run auto-test migrations", "error", err)
		os.Exit(2)
	}

	// Create auto-test tables in the autotester DB (runtime fallback)
	logger.Info("Step 4b Create AutoTest Tables")
	dbType := ApiTypes.CommonConfig.AppInfo.DatabaseType
	if err := autotester.CreateAutoTestTables(logger, ApiTypes.CommonConfig.PGConf.AutotesterDBHandle, dbType); err != nil {
		logger.Error("Failed to create auto-test tables", "error", err)
		os.Exit(2)
	}

	// Register testers
	logger.Info("Step 5 Register testers")
	registerAll()

	// Build runner
	logger.Info("Step 6 Create Test Runner")
	runner := autotester.NewTestRunner(
		autotester.GlobalRegistry.Build(),
		&autotester.RunConfig{
			Purposes:    split(*purposes),
			Types:       split(*types),
			Tags:        split(*tags),
			TesterNames: split(*testerNames),
			TestIDs:     split(*testIDs),
			Seed:        *seed,
			Parallel:    *parallel,
			MaxParallel: *maxParallel,
			RetryCount:  *retryCount,
			CaseTimeout: *caseTimeout,
			RunTimeout:  *runTimeout,
			StopOnFail:  *stopOnFail,
			SkipCleanup: *skipCleanup,
			Verbose:     *verbose,
			JSONReport:  *jsonReport,
			Environment: *env,
		},
		logger,
	)

	// Set up database persistence
	logger.Info("Step 7 Create DB Persistence")
	dbPersistence := autotester.NewDBPersistence(ApiTypes.CommonConfig.PGConf.AutotesterDBHandle)
	runner.SetDBPersistence(dbPersistence)

	logger.Info("Step 8 Run testers", "runner", runner)
	if err := runner.Run(ctx); err != nil {
		logger.Error("Test run failed", "error", err)
		os.Exit(2)
	}

	// Exit with appropriate code
	logger.Info("Step 9 Generate summary")
	summary := runner.Summary()
	if summary.Failed > 0 || summary.Errored > 0 {
		os.Exit(0)
	}

	logger.Info("Step 10 Test Finished")
	os.Exit(0)
}

// split converts a comma-separated string to a slice.
func split(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// isProductionDB checks if the config points to a production database.
func isProductionDB() bool {
	// Check if the database host matches a known production hostname
	// This should be configured based on your actual production settings
	productionHosts := []string{
		"prod-db.example.com",
		"production.database.example.com",
		// Add your actual production database hosts here
	}

	dbHost := ApiTypes.CommonConfig.PGConf.Host
	for _, prodHost := range productionHosts {
		if dbHost == prodHost {
			return true
		}
	}
	return false
}

// runAutoTestMigrations runs goose migrations for auto-test tables.
func runAutoTestMigrations(ctx context.Context, logger ApiTypes.JimoLogger) error {
	// Read the config
	migrate_cfg := ApiTypes.CommonConfig.MigrationConfig
	logger.Info("Running auto-test migrations",
		"migration_fs", migrate_cfg.MigrationsFS,
		"migration_dir", migrate_cfg.MigrationsDir,
		"tablename", migrate_cfg.TableName,
		"verbose", migrate_cfg.Verbose,
		"allow_outof_order", migrate_cfg.AllowOutOfOrder)

	var project_db *sql.DB = ApiTypes.ProjectDBHandle
	var migrate_db *sql.DB = ApiTypes.SharedMigrationDBHandle
	var autotester_db *sql.DB = ApiTypes.AutotesterDBHandle
	var dbType = ApiTypes.DBType

	if project_db == nil {
		return fmt.Errorf("project db is not set (MID_060221143038) for db type: %s", dbType)
	}

	if migrate_db == nil {
		return fmt.Errorf("migrate database connection is not set (MID_060221143039) for db type: %s", dbType)
	}

	logger.Info("Running project migrations")
	err := goose.RunProjectMigrations(ctx, logger, migrate_cfg, project_db)
	if err != nil {
		return fmt.Errorf("failed to run project migrator (MID_060221143036): %w", err)
	}

	logger.Info("Running shared migrations")
	err = goose.RunSharedMigrations(ctx, logger, migrate_cfg, migrate_db)
	if err != nil {
		return fmt.Errorf("failed to run shared migrator (MID_060221143037): %w", err)
	}

	logger.Info("Running autotester migrations")

	if autotester_db == nil {
		return fmt.Errorf("autotester database connection is not set (MID_060221143041)")
	}

	err = goose.RunAutoTesterMigrations(ctx, logger, migrate_cfg, autotester_db)
	if err != nil {
		return fmt.Errorf("failed to run autotester migrator (MID_060221143038): %w", err)
	}

	logger.Info("Auto-test migrations completed successfully")
	return nil
}

func LoadConfig(
	ctx context.Context,
	logger ApiTypes.JimoLogger,
	configPath string) error {

	logger.Info("Loading config", "config_path", configPath)
	viper.SetConfigFile(configPath)
	viper.SetConfigType("toml")

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			return fmt.Errorf("config file not found (MID_26022801): %s", configPath)
		}
		return fmt.Errorf("error reading config (MID_26022802): %w, config_path:%s", err, configPath)
	}

	// Override with environment variables (e.g., DATABASE_URL)
	viper.AutomaticEnv()

	// Unmarshal into struct
	if err := viper.Unmarshal(&ApiTypes.CommonConfig); err != nil {
		return fmt.Errorf("unable to decode config (MID_26022803): %w", err)
	}

	logger.Info("PG env vars (TAX_CFG_115)",
		"user", ApiTypes.CommonConfig.PGConf.UserName,
		"db", ApiTypes.CommonConfig.PGConf.ProjectDBName,
		"pwd_set", ApiTypes.CommonConfig.PGConf.Password != "")

	// Fall back to config file values if env vars are not set (for backwards compatibility)
	if ApiTypes.CommonConfig.PGConf.UserName == "" {
		logger.Warn("(MID_26031208) PG_USER_NAME not set in env, falling back to config")
	}
	if ApiTypes.CommonConfig.PGConf.Password == "" {
		logger.Error("(MID_26031203) PG_PASSWORD not set in env, falling back to config")
	}
	if ApiTypes.CommonConfig.PGConf.ProjectDBName == "" {
		logger.Error("(MID_26031204) PG_DB_NAME not set in env, falling back to config")
	}

	logger.Info("Config load success",
		"database_type", ApiTypes.CommonConfig.AppInfo.DatabaseType,
		"need_create_tables", ApiTypes.CommonConfig.AppInfo.NeedCreateTables,
		"pg", ApiTypes.CommonConfig.PGConf.Create,
		"mysql", ApiTypes.CommonConfig.MySQLConf.Create)
	return nil
}
