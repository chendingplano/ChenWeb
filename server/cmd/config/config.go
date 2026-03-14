// config/config.go
package config

import (
	"context"
	"fmt"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/spf13/viper"
)

type AppConfigDef struct {
	AppTableNames struct {
		TableName_ProcessStatus string `mapstructure:"table_name_process_status"`
		TableName_Schedules     string `mapstructure:"table_name_schedules"`
		TableName_Documents     string `mapstructure:"table_name_documents"`
		TableName_Flows         string `mapstructure:"table_name_flows"`
		TableName_DspyPrompts   string `mapstructure:"table_name_dspy_prompts"`
	} `mapstructure:"app_table_names"`
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
		"migration_db", ApiTypes.CommonConfig.PGConf.MigrationDBName,
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
