// config/config.go
package config

import (
	"context"
	"errors"
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

type ArtifactSearchWeightsConfig struct {
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

type SemanticProjectionSearchWeightsConfig struct {
	DescriptiveName    float64 `mapstructure:"descriptive_name"`
	Keywords           float64 `mapstructure:"keywords"`
	SemanticProjection float64 `mapstructure:"semantic_projection"`
	CategoryPaths      float64 `mapstructure:"category_paths"`
}

type InventoryItemSearchWeightsConfig struct {
	ItemName         float64 `mapstructure:"item_name"`
	CanonicalName    float64 `mapstructure:"canonical_name"`
	ItemCategories   float64 `mapstructure:"item_categories"`
	Manufacturer     float64 `mapstructure:"manufacturer"`
	Brand            float64 `mapstructure:"brand"`
	ModelNumber      float64 `mapstructure:"model_number"`
	PartNumber       float64 `mapstructure:"part_number"`
	Aliases          float64 `mapstructure:"aliases"`
	Standards        float64 `mapstructure:"standards"`
	NormalizedSpecs  float64 `mapstructure:"normalized_specs"`
	DedupeKey        float64 `mapstructure:"dedupe_key"`
	ConfidenceReason float64 `mapstructure:"confidence_reason"`
}

type SceneBlockSearchWeightsConfig struct {
	Title     float64 `mapstructure:"title"`
	SceneType float64 `mapstructure:"scene_type"`
	Summary   float64 `mapstructure:"summary"`
	Keywords  float64 `mapstructure:"keywords"`
}

type ProvisionSearchWeightsConfig struct {
	ProvisionName float64 `mapstructure:"provision_name"`
	ProvisionType float64 `mapstructure:"provision_type"`
	ProvisionDesc float64 `mapstructure:"provision_desc"`
	Keywords      float64 `mapstructure:"keywords"`
	CategoryPaths float64 `mapstructure:"category_paths"`
}

type SummarySearchWeightsConfig struct {
	SummaryText   float64 `mapstructure:"summary_text"`
	Keywords      float64 `mapstructure:"keywords"`
	CategoryPaths float64 `mapstructure:"category_paths"`
}

type TopicSearchWeightsConfig struct {
	TopicType     float64 `mapstructure:"topic_type"`
	TopicDesc     float64 `mapstructure:"topic_desc"`
	Keywords      float64 `mapstructure:"keywords"`
	CategoryPaths float64 `mapstructure:"category_paths"`
}

type EntitySearchWeightsConfig struct {
	Entity     float64 `mapstructure:"entity"`
	EntityType float64 `mapstructure:"entity_type"`
	Aliases    float64 `mapstructure:"aliases"`
	DescText   float64 `mapstructure:"desc_text"`
	Keywords   float64 `mapstructure:"keywords"`
}

type RelationSearchWeightsConfig struct {
	Subject   float64 `mapstructure:"subject"`
	Predicate float64 `mapstructure:"predicate"`
	Object    float64 `mapstructure:"object"`
	DescText  float64 `mapstructure:"desc_text"`
	Keywords  float64 `mapstructure:"keywords"`
}

type ArtifactSearchConfig struct {
	Dictionary      string  `mapstructure:"dictionary"`
	DefaultPageSize int     `mapstructure:"default_page_size"`
	MaxPageSize     int     `mapstructure:"max_page_size"`
	PreviewMaxWords int     `mapstructure:"preview_max_words"`
	PhraseFriendly  bool    `mapstructure:"phrase_friendly"`
	MinRank         float64 `mapstructure:"min_rank"`
}

type legacyArtifactSearchConfig struct {
	Dictionary      string                      `mapstructure:"dictionary"`
	DefaultPageSize int                         `mapstructure:"default_page_size"`
	MaxPageSize     int                         `mapstructure:"max_page_size"`
	PreviewMaxWords int                         `mapstructure:"preview_max_words"`
	PhraseFriendly  bool                        `mapstructure:"phrase_friendly"`
	MinRank         float64                     `mapstructure:"min_rank"`
	Weights         ArtifactSearchWeightsConfig `mapstructure:"weights"`
}

type LLMConfig struct {
	WorkspaceTimezone     string `mapstructure:"workspace_timezone"`
	UsageRetentionDays    int    `mapstructure:"usage_retention_days"`
	ArchiveRoot           string `mapstructure:"archive_root"`
	ReconciliationRunHour int    `mapstructure:"reconciliation_run_hour"`
}

type LanguagesConfig struct {
	Languages []string `mapstructure:"languages"`
}

type FrontendConfigSection struct {
	// DefaultKnowledgeStore is the ks_name of the kb.knowledge_store row that
	// /home3/knowledge selects on entry. It must match exactly one row; when it
	// is empty or matches none/several rows, no store is selected and the user
	// picks one manually.
	DefaultKnowledgeStore string `mapstructure:"default_knowledge_store"`
	// EnableLoginWithGithub/EnableLoginWithGoogle control whether the
	// respective OAuth buttons are shown on the login page. Pointers so an
	// unset key can default to true. Configured via [frontend] in
	// config.toml / config.local.toml.
	EnableLoginWithGithub *bool `mapstructure:"enable_login_with_github"`
	EnableLoginWithGoogle *bool `mapstructure:"enable_login_with_google"`
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
	PDFParser                       PDFParserConfig                       `mapstructure:"pdf_parser"`
	DocGen                          DocGenConfig                          `mapstructure:"doc_gen"`
	ArtifactSearch                  legacyArtifactSearchConfig            `mapstructure:"artifact_search"`
	MetricSearch                    legacyArtifactSearchConfig            `mapstructure:"metric_search"`
	MetricSearchWeights             ArtifactSearchWeightsConfig           `mapstructure:"metric_search_weights"`
	SemanticProjectionSearchWeights SemanticProjectionSearchWeightsConfig `mapstructure:"semantic_projections_search_weights"`
	InventoryItemSearchWeights      InventoryItemSearchWeightsConfig      `mapstructure:"inventory_items_search_weights"`
	SceneBlockSearchWeights         SceneBlockSearchWeightsConfig         `mapstructure:"scene_blocks_search_weights"`
	ProvisionSearchWeights          ProvisionSearchWeightsConfig          `mapstructure:"provisions_search_weights"`
	SummarySearchWeights            SummarySearchWeightsConfig            `mapstructure:"summaries_search_weights"`
	TopicSearchWeights              TopicSearchWeightsConfig              `mapstructure:"topics_search_weights"`
	EntitySearchWeights             EntitySearchWeightsConfig             `mapstructure:"entities_search_weights"`
	RelationSearchWeights           RelationSearchWeightsConfig           `mapstructure:"relations_search_weights"`
	LLM                             LLMConfig                             `mapstructure:"llm"`
	Languages                       LanguagesConfig                       `mapstructure:"languages"`
	// SiteConfig names the tenant-independent site-config file for the
	// customer-facing frontend (ADR 2026071102). Tenant-dependent config
	// filenames come from the site_tenants table, never from here.
	SiteConfig SiteConfigSection `mapstructure:"config"`
	// Frontend holds settings the frontend reads through dedicated endpoints.
	// Configured via [frontend] in config.toml / config.local.toml.
	Frontend FrontendConfigSection `mapstructure:"frontend"`
	// DocReviews maps a review tier key (e.g. "must-review") to the list of
	// aspect item names included in that tier. Configured via [doc-reviews]
	// in config.toml / config.local.toml. When empty, the Document Review
	// feature falls back to its built-in priority-derived tiers.
	DocReviews map[string][]string `mapstructure:"doc-reviews"`
	// KnowledgeMenus maps a Wiki sidebar menu item id (e.g. "kb-doc-wiki") to
	// whether it is shown. Configured via [knowledge-menus] in config.toml /
	// config.local.toml. Ids absent from the map default to enabled. When
	// empty, the full Wiki sidebar menu is shown.
	KnowledgeMenus map[string]bool `mapstructure:"knowledge-menus"`
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

type SiteConfigSection struct {
	ConfigFilename string `mapstructure:"config_filename"`
}

var AppConfig AppConfigDef
var appConfigViper = viper.New()

func LoadConfig(ctx context.Context, logger ApiTypes.JimoLogger, configPath string) error {
	call_flow := ctx.Value(ApiTypes.CallFlowKey).(string)
	logger.Info("Loading config", "filePath", configPath)
	appVp := viper.New()
	appVp.SetConfigFile(configPath)
	appVp.SetConfigType("toml")

	// Read config file
	if err := appVp.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			return fmt.Errorf("(MID_26031001) config file not found, file:%s", configPath)
		}
		return fmt.Errorf("(MID_26031002) error reading config (%s->CWB_CFG_056): %w", call_flow, err)
	}

	// Merge optional local overrides from config.local.toml sitting next to the
	// main config file. Values present in the local file take precedence.
	localPath := filepath.Join(filepath.Dir(configPath), "config.local.toml")
	if _, statErr := os.Stat(localPath); statErr == nil {
		appVp.SetConfigFile(localPath)
		if mergeErr := appVp.MergeInConfig(); mergeErr != nil {
			return fmt.Errorf("(MID_26031006) error merging local config (%s): %w", localPath, mergeErr)
		}
		logger.Info("Merged local config overrides", "filePath", localPath)
	}

	// Override with environment variables (e.g., DATABASE_URL)
	appVp.AutomaticEnv()

	// Unmarshal into struct
	if err := appVp.Unmarshal(&AppConfig); err != nil {
		return fmt.Errorf("(MID_26031003) unable to decode app config, error:%w", err)
	}
	appConfigViper = appVp

	err := ApiUtils.LoadConfig(ctx, logger, configPath)
	if err != nil {
		return fmt.Errorf("failed to load commonConfig, error:%w", err)
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

func GetLLMConfig() LLMConfig {
	cfg := AppConfig.LLM
	if strings.TrimSpace(cfg.WorkspaceTimezone) == "" {
		cfg.WorkspaceTimezone = "America/Chicago"
	}
	if cfg.UsageRetentionDays <= 0 {
		cfg.UsageRetentionDays = 30
	}
	if strings.TrimSpace(cfg.ArchiveRoot) == "" {
		cfg.ArchiveRoot = "Data/llm-logs"
	}
	if cfg.ReconciliationRunHour <= 0 || cfg.ReconciliationRunHour > 23 {
		cfg.ReconciliationRunHour = 2
	}
	return cfg
}

func GetDocGenConfig() DocGenConfig {
	return AppConfig.DocGen
}

// GetDocReviewsConfig returns the configured Document Review tier->aspect-names
// mapping. Returns nil/empty when no [doc-reviews] section is present, in which
// case the Document Review feature uses its built-in defaults.
func GetDocReviewsConfig() map[string][]string {
	return AppConfig.DocReviews
}

// GetKnowledgeMenusConfig returns the configured Wiki sidebar menu id->enabled
// mapping. Returns nil/empty when no [knowledge-menus] section is present, in
// which case every menu item defaults to enabled.
func GetKnowledgeMenusConfig() map[string]bool {
	return AppConfig.KnowledgeMenus
}

func GetLanguages() []string {
	raw := appConfigViper.GetStringSlice("languages.languages")
	if len(raw) == 0 {
		raw = AppConfig.Languages.Languages
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(raw))
	for _, lang := range raw {
		lang = strings.ToLower(strings.TrimSpace(lang))
		if lang == "" || seen[lang] {
			continue
		}
		seen[lang] = true
		out = append(out, lang)
	}
	if len(out) == 0 {
		return []string{"en"}
	}
	return out
}

func GetSiteConfigFilename() string {
	return AppConfig.SiteConfig.ConfigFilename
}

// GetDefaultKnowledgeStoreName returns [frontend].default_knowledge_store, or
// "" when it is not configured.
func GetDefaultKnowledgeStoreName() string {
	return strings.TrimSpace(AppConfig.Frontend.DefaultKnowledgeStore)
}

// GetEnableLoginWithGithub returns [frontend].enable_login_with_github,
// defaulting to true when unset.
func GetEnableLoginWithGithub() bool {
	if AppConfig.Frontend.EnableLoginWithGithub == nil {
		return true
	}
	return *AppConfig.Frontend.EnableLoginWithGithub
}

// GetEnableLoginWithGoogle returns [frontend].enable_login_with_google,
// defaulting to true when unset.
func GetEnableLoginWithGoogle() bool {
	if AppConfig.Frontend.EnableLoginWithGoogle == nil {
		return true
	}
	return *AppConfig.Frontend.EnableLoginWithGoogle
}

func GetArtifactSearchConfig() ArtifactSearchConfig {
	cfg := toArtifactSearchConfig(AppConfig.ArtifactSearch)
	if isArtifactSearchConfigZero(cfg) {
		cfg = toArtifactSearchConfig(AppConfig.MetricSearch)
	}
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
	if !appConfigViper.IsSet("artifact_search.phrase_friendly") && !appConfigViper.IsSet("metric_search.phrase_friendly") {
		cfg.PhraseFriendly = true
	}
	return cfg
}

func GetMetricSearchConfig() ArtifactSearchConfig {
	return GetArtifactSearchConfig()
}

func GetMetricSearchWeightsConfig() ArtifactSearchWeightsConfig {
	cfg := AppConfig.MetricSearchWeights
	if cfg == (ArtifactSearchWeightsConfig{}) {
		cfg = AppConfig.ArtifactSearch.Weights
	}
	if cfg == (ArtifactSearchWeightsConfig{}) {
		cfg = AppConfig.MetricSearch.Weights
	}
	if cfg.MetricName <= 0 {
		cfg.MetricName = 1.8
	}
	if cfg.MetricSubject <= 0 {
		cfg.MetricSubject = 1.5
	}
	if cfg.MetricKeywords <= 0 {
		cfg.MetricKeywords = 2.8
	}
	if cfg.MetricDesc <= 0 {
		cfg.MetricDesc = 1.0
	}
	if cfg.MetricContext <= 0 {
		cfg.MetricContext = 0.8
	}
	if cfg.ValueClass <= 0 {
		cfg.ValueClass = 0.7
	}
	if cfg.MetricUnit <= 0 {
		cfg.MetricUnit = 0.5
	}
	if cfg.TableNameOrSection <= 0 {
		cfg.TableNameOrSection = 0.4
	}
	if cfg.CategoryPaths <= 0 {
		cfg.CategoryPaths = 0.6
	}
	return cfg
}

func GetSemanticProjectionSearchWeightsConfig() SemanticProjectionSearchWeightsConfig {
	cfg := AppConfig.SemanticProjectionSearchWeights
	if cfg.DescriptiveName <= 0 {
		cfg.DescriptiveName = 1.8
	}
	if cfg.Keywords <= 0 {
		cfg.Keywords = 2.4
	}
	if cfg.SemanticProjection <= 0 {
		cfg.SemanticProjection = 1.0
	}
	if cfg.CategoryPaths <= 0 {
		cfg.CategoryPaths = 0.6
	}
	return cfg
}

func GetInventoryItemSearchWeightsConfig() InventoryItemSearchWeightsConfig {
	cfg := AppConfig.InventoryItemSearchWeights
	if cfg.ItemName <= 0 {
		cfg.ItemName = 2.0
	}
	if cfg.CanonicalName <= 0 {
		cfg.CanonicalName = 1.8
	}
	if cfg.ItemCategories <= 0 {
		cfg.ItemCategories = 2.2
	}
	if cfg.Manufacturer <= 0 {
		cfg.Manufacturer = 0.8
	}
	if cfg.Brand <= 0 {
		cfg.Brand = 1.0
	}
	if cfg.ModelNumber <= 0 {
		cfg.ModelNumber = 1.3
	}
	if cfg.PartNumber <= 0 {
		cfg.PartNumber = 1.3
	}
	if cfg.Aliases <= 0 {
		cfg.Aliases = 1.2
	}
	if cfg.Standards <= 0 {
		cfg.Standards = 0.8
	}
	if cfg.NormalizedSpecs <= 0 {
		cfg.NormalizedSpecs = 0.7
	}
	if cfg.DedupeKey <= 0 {
		cfg.DedupeKey = 0.5
	}
	if cfg.ConfidenceReason <= 0 {
		cfg.ConfidenceReason = 0.4
	}
	return cfg
}

func GetSceneBlockSearchWeightsConfig() SceneBlockSearchWeightsConfig {
	cfg := AppConfig.SceneBlockSearchWeights
	if cfg.Title <= 0 {
		cfg.Title = 1.8
	}
	if cfg.SceneType <= 0 {
		cfg.SceneType = 1.2
	}
	if cfg.Summary <= 0 {
		cfg.Summary = 1.0
	}
	if cfg.Keywords <= 0 {
		cfg.Keywords = 2.2
	}
	return cfg
}

func GetProvisionSearchWeightsConfig() ProvisionSearchWeightsConfig {
	cfg := AppConfig.ProvisionSearchWeights
	if cfg.ProvisionName <= 0 {
		cfg.ProvisionName = 1.8
	}
	if cfg.ProvisionType <= 0 {
		cfg.ProvisionType = 1.2
	}
	if cfg.ProvisionDesc <= 0 {
		cfg.ProvisionDesc = 1.0
	}
	if cfg.Keywords <= 0 {
		cfg.Keywords = 2.2
	}
	if cfg.CategoryPaths <= 0 {
		cfg.CategoryPaths = 0.6
	}
	return cfg
}

func GetSummarySearchWeightsConfig() SummarySearchWeightsConfig {
	cfg := AppConfig.SummarySearchWeights
	if cfg.SummaryText <= 0 {
		cfg.SummaryText = 1.8
	}
	if cfg.Keywords <= 0 {
		cfg.Keywords = 2.2
	}
	if cfg.CategoryPaths <= 0 {
		cfg.CategoryPaths = 0.8
	}
	return cfg
}

func GetTopicSearchWeightsConfig() TopicSearchWeightsConfig {
	cfg := AppConfig.TopicSearchWeights
	if cfg.TopicType <= 0 {
		cfg.TopicType = 1.2
	}
	if cfg.TopicDesc <= 0 {
		cfg.TopicDesc = 1.8
	}
	if cfg.Keywords <= 0 {
		cfg.Keywords = 2.2
	}
	if cfg.CategoryPaths <= 0 {
		cfg.CategoryPaths = 0.8
	}
	return cfg
}

func GetEntitySearchWeightsConfig() EntitySearchWeightsConfig {
	cfg := AppConfig.EntitySearchWeights
	if cfg.Entity <= 0 {
		cfg.Entity = 1.8
	}
	if cfg.EntityType <= 0 {
		cfg.EntityType = 1.2
	}
	if cfg.Aliases <= 0 {
		cfg.Aliases = 1.2
	}
	if cfg.DescText <= 0 {
		cfg.DescText = 1.0
	}
	if cfg.Keywords <= 0 {
		cfg.Keywords = 2.2
	}
	return cfg
}

func GetRelationSearchWeightsConfig() RelationSearchWeightsConfig {
	cfg := AppConfig.RelationSearchWeights
	if cfg.Subject <= 0 {
		cfg.Subject = 1.6
	}
	if cfg.Predicate <= 0 {
		cfg.Predicate = 1.6
	}
	if cfg.Object <= 0 {
		cfg.Object = 1.6
	}
	if cfg.DescText <= 0 {
		cfg.DescText = 1.0
	}
	if cfg.Keywords <= 0 {
		cfg.Keywords = 2.0
	}
	return cfg
}

func toArtifactSearchConfig(cfg legacyArtifactSearchConfig) ArtifactSearchConfig {
	return ArtifactSearchConfig{
		Dictionary:      cfg.Dictionary,
		DefaultPageSize: cfg.DefaultPageSize,
		MaxPageSize:     cfg.MaxPageSize,
		PreviewMaxWords: cfg.PreviewMaxWords,
		PhraseFriendly:  cfg.PhraseFriendly,
		MinRank:         cfg.MinRank,
	}
}

func isArtifactSearchConfigZero(cfg ArtifactSearchConfig) bool {
	return strings.TrimSpace(cfg.Dictionary) == "" &&
		cfg.DefaultPageSize == 0 &&
		cfg.MaxPageSize == 0 &&
		cfg.PreviewMaxWords == 0 &&
		!cfg.PhraseFriendly &&
		cfg.MinRank == 0
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
