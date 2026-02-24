package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/autotesters"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/goose"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/dinglind/mirai/server/cmd/config"
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

	logger := loggerutil.CreateDefaultLogger("AUTO_TESTER")

	// Load config
	logger.Info("Step 1 Read Config")
	ctx := context.Background()
	configPath := "config.toml" // Default config path
	if err := config.LoadConfig(ctx, configPath); err != nil {
		logger.Error("Config load failed", "error", err)
		os.Exit(2)
	}
	cfg := config.GlobalConfig
	migrate_cfg := cfg.MigrationConfig
	logger.Info("Config loaded successfully",
		"migration_fs", migrate_cfg.MigrationsFS,
		"migration_dir", migrate_cfg.MigrationsDir,
		"tablename", migrate_cfg.TableName,
		"verbose", migrate_cfg.Verbose,
		"allow_outof_order", migrate_cfg.AllowOutOfOrder)

	// Safety check: refuse to run against production
	logger.Info("Step 2 Check ProductionDB")
	if isProductionDB(&cfg) {
		logger.Error("Refusing to run AutoTester against production database")
		os.Exit(2)
	}

	// Init database
	logger.Info("Step 3 InitDB", "config", config.PGConfig)
	new_ctx := context.WithValue(ctx, ApiTypes.CallFlowKey, "SHD_0220185500")
	if err := databaseutil.InitDB(new_ctx, config.MySQLConfig, config.PGConfig); err != nil {
		logger.Error("Failed to initialize database", "error", err)
		os.Exit(2)
	}

	// Verify database connection
	if ApiTypes.PG_DB_Project == nil {
		logger.Error("Project Database connection not initialized")
		os.Exit(2)
	}

	if ApiTypes.PG_DB_Shared == nil {
		logger.Error("Shared DB connection not initialized")
		os.Exit(2)
	}

	if ApiTypes.PG_DB_AutoTester == nil {
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
	dbType := ApiTypes.DatabaseInfo.DBType
	if err := autotesters.CreateAutoTestTables(logger, ApiTypes.PG_DB_AutoTester, dbType); err != nil {
		logger.Error("Failed to create auto-test tables", "error", err)
		os.Exit(2)
	}

	// Register testers
	logger.Info("Step 5 Register testers")
	registerAll(&cfg)

	// Build runner
	logger.Info("Step 6 Create Test Runner")
	runner := autotesters.NewTestRunner(
		autotesters.GlobalRegistry.Build(),
		&autotesters.RunConfig{
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
	dbPersistence := autotesters.NewDBPersistence(ApiTypes.PG_DB_AutoTester)
	runner.SetDBPersistence(dbPersistence)

	logger.Info("Step 8 Run testers")
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
func isProductionDB(cfg *config.Config) bool {
	// Check if the database host matches a known production hostname
	// This should be configured based on your actual production settings
	productionHosts := []string{
		"prod-db.example.com",
		"production.database.example.com",
		// Add your actual production database hosts here
	}

	dbHost := cfg.Database.PGHost
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
	migrate_cfg := config.GlobalConfig.MigrationConfig
	logger.Info("Running auto-test migrations",
		"migration_fs", migrate_cfg.MigrationsFS,
		"migration_dir", migrate_cfg.MigrationsDir,
		"tablename", migrate_cfg.TableName,
		"verbose", migrate_cfg.Verbose,
		"allow_outof_order", migrate_cfg.AllowOutOfOrder)

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
		return fmt.Errorf("unsupported db type (MID_060221143000): %s", dbType)
	}

	if project_db == nil {
		return fmt.Errorf("project db is not set (MID_060221143038) for db type: %s", dbType)
	}

	if shared_db == nil {
		return fmt.Errorf("shared database connection is not set (MID_060221143039) for db type: %s", dbType)
	}

	logger.Info("Running project migrations")
	err := goose.RunProjectMigrations(ctx, logger, migrate_cfg, project_db)
	if err != nil {
		return fmt.Errorf("failed to run project migrator (MID_060221143036): %w", err)
	}

	logger.Info("Running shared migrations")
	err = goose.RunSharedMigrations(ctx, logger, migrate_cfg, shared_db)
	if err != nil {
		return fmt.Errorf("failed to run shared migrator (MID_060221143037): %w", err)
	}

	logger.Info("Running autotester migrations")
	var autotester_db *sql.DB
	switch dbType {
	case ApiTypes.PgName:
		autotester_db = ApiTypes.PG_DB_AutoTester
	case ApiTypes.MysqlName:
		autotester_db = ApiTypes.MySql_DB_AutoTester
	default:
		return fmt.Errorf("unsupported db type (MID_060221143040): %s", dbType)
	}

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
