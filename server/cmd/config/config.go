// config/config.go
package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	sharedgoose "github.com/chendingplano/shared/go/api/goose"
	"github.com/spf13/viper"
)

type DocGenConfig struct {
	TemplateDir   string `mapstructure:"template_dir"`
	WorkerCount   int    `mapstructure:"worker_count"`
	OutputBaseDir string `mapstructure:"output_base_dir"`
}

type MetricSearchWeightsConfig struct {
	MetricName         float64 `mapstructure:"metric_name"`
	MetricSubject      float64 `mapstructure:"metric_subject"`
	MetricKeywords     float64 `mapstructure:"metric_keywords"`
	MetricDesc         float64 `mapstructure:"metric_desc"`
	MetricContext      float64 `mapstructure:"metric_context"`
	ValueClass         float64 `mapstructure:"value_class"`
	MetricUnit         float64 `mapstructure:"metric_unit"`
	TableNameOrSection float64 `mapstructure:"table_name_or_section"`
	CategoryPaths      float64 `mapstructure:"category_paths"`
}

type MetricSearchConfig struct {
	Dictionary      string                    `mapstructure:"dictionary"`
	DefaultPageSize int                       `mapstructure:"default_page_size"`
	MaxPageSize     int                       `mapstructure:"max_page_size"`
	PreviewMaxWords int                       `mapstructure:"preview_max_words"`
	PhraseFriendly  bool                      `mapstructure:"phrase_friendly"`
	MinRank         float64                   `mapstructure:"min_rank"`
	Weights         MetricSearchWeightsConfig `mapstructure:"weights"`
}

type AppConfigDef struct {
	AppTableNames struct {
		TableName_ProcessStatus   string `mapstructure:"table_name_process_status"`
		TableName_Schedules       string `mapstructure:"table_name_schedules"`
		TableName_Documents       string `mapstructure:"table_name_documents"`
		TableName_Flows           string `mapstructure:"table_name_flows"`
		TableName_DspyPrompts     string `mapstructure:"table_name_dspy_prompts"`
		TableName_CustRequestLogs string `mapstructure:"table_name_cust_request_logs"`
	} `mapstructure:"app_table_names"`
	PDFParser    PDFParserConfig    `mapstructure:"pdf_parser"`
	DocGen       DocGenConfig       `mapstructure:"doc_gen"`
	MetricSearch MetricSearchConfig `mapstructure:"metric_search"`
}

type PDFParserConfig struct {
	Enabled             bool     `mapstructure:"enabled"`
	StagingDir          string   `mapstructure:"staging_dir"`
	RepoDirs            []string `mapstructure:"repo_dirs"`
	BackupDir           string   `mapstructure:"backup_dir"`
	PollIntervalSeconds int      `mapstructure:"poll_interval_seconds"`
	BatchSize           int      `mapstructure:"batch_size"`
	PythonBin           string   `mapstructure:"python_bin"`
	PaddleOCRScript     string   `mapstructure:"paddleocr_script"`
	UsePaddleOCRVL      bool     `mapstructure:"use_paddleocr_vl"`
	DeleteFromStaging   bool     `mapstructure:"delete_from_staging"`
	WorkDir             string   `mapstructure:"work_dir"`
}

var AppConfig AppConfigDef

func LoadConfig(ctx context.Context, logger ApiTypes.JimoLogger, configPath string) error {
	call_flow := ctx.Value(ApiTypes.CallFlowKey).(string)
	logger.Info("Loading config", "filePath", configPath)
	viper.SetConfigFile(configPath)
	viper.SetConfigType("toml")

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return fmt.Errorf("(MID_26031001) config file not found, file:%s", configPath)
		}
		return fmt.Errorf("(MID_26031002) error reading config (%s->CWB_CFG_056): %w", call_flow, err)
	}

	// Override with environment variables (e.g., DATABASE_URL)
	viper.AutomaticEnv()

	err := ApiUtils.LoadConfig(ctx, logger, configPath)
	if err != nil {
		return fmt.Errorf("failed to load commonConfig, error:%w", err)
	}

	// Unmarshal into struct
	if err := viper.Unmarshal(&AppConfig); err != nil {
		return fmt.Errorf("(MID_26031003) unable to decode app config, error:%w", err)
	}

	if err := ApiUtils.SetConfig(ApiTypes.CommonConfig); err != nil {
		return fmt.Errorf("(MID_26030903) failed set Util config, error:%w", err)
	}

	logger.Info("PG env vars",
		"user", ApiTypes.CommonConfig.PGConf.UserName,
		"project_db", ApiTypes.CommonConfig.PGConf.ProjectDBName,
		"shared_db", ApiTypes.CommonConfig.PGConf.SharedDBName,
		"autotester_db", ApiTypes.CommonConfig.PGConf.AutotesterDBName,
		"pwd_set", ApiTypes.CommonConfig.PGConf.Password != "")

	db_type := ApiTypes.CommonConfig.AppInfo.DatabaseType
	if db_type == "" {
		err1 := fmt.Errorf("(MID_26031004) unable to decode config")
		logger.Error("unable to decode config", "configPath", configPath)
		panic(err1)
	}

	if !ApiTypes.IsValidDBType(db_type) {
		err1 := fmt.Errorf("(MID_26031005) unsupported database type: %s, Allowed:pg|mysql", db_type)
		logger.Error("db_type not supported", "db_type", db_type)
		panic(err1)
	}

	logger.Info("Config load success",
		"database_type", db_type,
		"need_create_tables", ApiTypes.CommonConfig.AppInfo.NeedCreateTables,
		"pg", ApiTypes.CommonConfig.PGConf.Create,
		"mysql", ApiTypes.CommonConfig.MySQLConf.Create)
	return nil
}

func GetDatabaseType() string {
	return ApiTypes.CommonConfig.AppInfo.DatabaseType
}

func NeedCreateTables() bool {
	return ApiTypes.CommonConfig.AppInfo.NeedCreateTables
}

func GetProcessStatusTableName() string {
	return AppConfig.AppTableNames.TableName_ProcessStatus
}

func GetPDFParserConfig() PDFParserConfig {
	return AppConfig.PDFParser
}

func GetDocGenConfig() DocGenConfig {
	return AppConfig.DocGen
}

func GetMetricSearchConfig() MetricSearchConfig {
	cfg := AppConfig.MetricSearch
	if strings.TrimSpace(cfg.Dictionary) == "" {
		cfg.Dictionary = "simple"
	}
	if cfg.DefaultPageSize <= 0 {
		cfg.DefaultPageSize = 20
	}
	if cfg.MaxPageSize <= 0 {
		cfg.MaxPageSize = 100
	}
	if cfg.PreviewMaxWords <= 0 {
		cfg.PreviewMaxWords = 18
	}
	if !viper.IsSet("metric_search.phrase_friendly") {
		cfg.PhraseFriendly = true
	}
	if cfg.Weights.MetricName <= 0 {
		cfg.Weights.MetricName = 1.8
	}
	if cfg.Weights.MetricSubject <= 0 {
		cfg.Weights.MetricSubject = 1.5
	}
	if cfg.Weights.MetricKeywords <= 0 {
		cfg.Weights.MetricKeywords = 2.8
	}
	if cfg.Weights.MetricDesc <= 0 {
		cfg.Weights.MetricDesc = 1.0
	}
	if cfg.Weights.MetricContext <= 0 {
		cfg.Weights.MetricContext = 0.8
	}
	if cfg.Weights.ValueClass <= 0 {
		cfg.Weights.ValueClass = 0.7
	}
	if cfg.Weights.MetricUnit <= 0 {
		cfg.Weights.MetricUnit = 0.5
	}
	if cfg.Weights.TableNameOrSection <= 0 {
		cfg.Weights.TableNameOrSection = 0.4
	}
	if cfg.Weights.CategoryPaths <= 0 {
		cfg.Weights.CategoryPaths = 0.6
	}
	return cfg
}

func NormalizeMigrationPaths(logger ApiTypes.JimoLogger, configPath string) {
	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		logger.Warn("failed to resolve absolute config path; migration paths left as-is",
			"config_path", configPath, "error", err)
		return
	}
	configDir := filepath.Dir(absConfigPath)
	cfg := ApiTypes.CommonConfig.MigrationConfig

	projectRel := strings.TrimSpace(cfg.MigrationsDir)
	if projectRel == "" {
		projectRel = "project_migrations"
	}
	sharedRel := strings.TrimSpace(cfg.SharedMigrationsDir)
	if sharedRel == "" {
		sharedRel = "shared_migrations"
	}

	cfg.MigrationsDir = resolveMigrationDir(configDir, projectRel)
	cfg.SharedMigrationsDir = resolveMigrationDir(configDir, sharedRel)

	cfg.MigrationsFS = os.DirFS(cfg.MigrationsDir)
	ApiTypes.CommonConfig.MigrationConfig = cfg

	logger.Info("normalized migration paths",
		"project_migrations_dir", cfg.MigrationsDir,
		"shared_migrations_dir", cfg.SharedMigrationsDir)
}

func resolveMigrationDir(configDir string, relOrAbs string) string {
	path := strings.TrimSpace(relOrAbs)
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}

	candidate := filepath.Clean(filepath.Join(configDir, path))
	if isDir(candidate) {
		return candidate
	}

	// If the config lives under server/cmd/*, search ancestors for the first
	// existing migration directory so all services share ChenWeb/{project,shared}_migrations.
	base := configDir
	for {
		probe := filepath.Clean(filepath.Join(base, path))
		if isDir(probe) {
			return probe
		}
		parent := filepath.Dir(base)
		if parent == base {
			break
		}
		base = parent
	}

	// If not found, keep the config-relative target (goose will create dir if needed).
	return candidate
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func RunMigrations(ctx context.Context, logger ApiTypes.JimoLogger) error {
	if ApiTypes.ProjectDBHandle == nil {
		return fmt.Errorf("project db handle is nil")
	}
	if ApiTypes.SharedDBHandle == nil {
		return fmt.Errorf("shared db handle is nil")
	}

	if err := sharedgoose.RunProjectMigrations(ctx, logger, ApiTypes.ProjectDBHandle); err != nil {
		return fmt.Errorf("project migrations failed: %w", err)
	}
	if err := sharedgoose.RunSharedMigrations(ctx, logger, ApiTypes.SharedDBHandle); err != nil {
		return fmt.Errorf("shared migrations failed: %w", err)
	}
	return nil
}
