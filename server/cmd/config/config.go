// config/config.go
package config

import (
	"context"
	"fmt"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/spf13/viper"
)

type Config struct {
	AppName string `mapstructure:"app_name"`
	Debug   bool   `mapstructure:"debug"`

	Server struct {
		Port int    `mapstructure:"port"`
		Host string `mapstructure:"host"`
	} `mapstructure:"server"`

	Database struct {
		CreateMySQL      bool   `mapstructure:"create_mysql"`
		CreatePG         bool   `mapstructure:"create_pg"`
		DatabaseType     string `mapstructure:"database_type"`
		PGHost           string `mapstructure:"pg_host"`
		PGPort           int    `mapstructure:"pg_port"`
		PGUserName       string `mapstructure:"pg_user_name"`
		PGPassword       string `mapstructure:"pg_password"`
		PGDBName         string `mapstructure:"pg_db_name"`
		MySQLHost        string `mapstructure:"mysql_host"`
		MySQLPort        int    `mapstructure:"mysql_port"`
		MySQLUserName    string `mapstructure:"mysql_user_name"`
		MySQLPassword    string `mapstructure:"mysql_password"`
		MySQLDBName      string `mapstructure:"mysql_db_name"`
		MaxConnections   int    `mapstructure:"max_connections"`
		NeedCreateTables bool   `mapstructure:"need_create_tables"`
	} `mapstructure:"database"`

	AppTableNames struct {
		TableName_ProcessStatus string `mapstructure:"table_name_process_status"`
		TableName_Schedules     string `mapstructure:"table_name_schedules"`
		TableName_Documents     string `mapstructure:"table_name_documents"`
		TableName_Flows         string `mapstructure:"table_name_flows"`
	} `mapstructure:"app_table_names"`

	Auth struct {
		JWTSecret            string `mapstructure:"jwt_secret"`
		SessionDurationHours int    `mapstructure:"session_duration_hours"`
	} `mapstructure:"auth"`

	Migration ApiTypes.MigrationConfig `mapstructure:"migration"`
}

var GlobalConfig Config
var MySQLConfig ApiTypes.DBConfig
var PGConfig ApiTypes.DBConfig

func LoadConfig(ctx context.Context, logger ApiTypes.JimoLogger, configPath string) error {
	call_flow := ctx.Value(ApiTypes.CallFlowKey).(string)
	logger.Info("Loading config", "filePath", configPath)
	viper.SetConfigFile(configPath)
	viper.SetConfigType("toml")

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return fmt.Errorf("config file not found (%s->CWB_CFG_054): %s", call_flow, configPath)
		}
		return fmt.Errorf("error reading config (%s->CWB_CFG_056): %w", call_flow, err)
	}

	// Override with environment variables (e.g., DATABASE_URL)
	viper.AutomaticEnv()

	// Unmarshal into struct
	if err := viper.Unmarshal(&GlobalConfig); err != nil {
		return fmt.Errorf("unable to decode config (%s->CWB_CFG_064): %w", call_flow, err)
	}

	MySQLConfig.Host = GlobalConfig.Database.MySQLHost
	MySQLConfig.Port = GlobalConfig.Database.MySQLPort
	MySQLConfig.DBType = "mysql"
	MySQLConfig.CreateFlag = GlobalConfig.Database.CreateMySQL
	MySQLConfig.UserName = GlobalConfig.Database.MySQLUserName
	MySQLConfig.Password = GlobalConfig.Database.MySQLPassword
	MySQLConfig.DbName = GlobalConfig.Database.MySQLDBName

	PGConfig.Host = GlobalConfig.Database.PGHost
	PGConfig.Port = GlobalConfig.Database.PGPort
	PGConfig.DBType = "pg"
	PGConfig.CreateFlag = GlobalConfig.Database.CreatePG
	PGConfig.UserName = GlobalConfig.Database.PGUserName
	PGConfig.Password = GlobalConfig.Database.PGPassword
	PGConfig.DbName = GlobalConfig.Database.PGDBName

	ApiTypes.DatabaseInfo.DBType = GlobalConfig.Database.DatabaseType
	ApiTypes.DatabaseInfo.PGDBName = GlobalConfig.Database.PGDBName
	ApiTypes.DatabaseInfo.MySQLDBName = GlobalConfig.Database.MySQLDBName
	ApiTypes.DatabaseInfo.MySQLDBHandle = ApiTypes.MySql_DB_Project
	ApiTypes.DatabaseInfo.MySQLMigrateDBHandle = ApiTypes.MySql_DB_Shared
	ApiTypes.DatabaseInfo.MySQLAutoTesterDBHandle = ApiTypes.MySql_DB_AutoTester

	ApiTypes.DatabaseInfo.PGDBHandle = ApiTypes.PG_DB_Project
	ApiTypes.DatabaseInfo.PGMigrateDBHandle = ApiTypes.PG_DB_Shared
	ApiTypes.DatabaseInfo.PGAutoTesterDBHandle = ApiTypes.PG_DB_AutoTester

	logger.Info("Config load success",
		"db_type", GlobalConfig.Database.DatabaseType,
		"create_type", GlobalConfig.Database.NeedCreateTables,
		"create_pg", GlobalConfig.Database.CreatePG,
		"create_mysql", GlobalConfig.Database.CreateMySQL,
	)

	if GlobalConfig.Database.DatabaseType == "" {
		err1 := fmt.Errorf("unable to decode config (%s->CWB_CFG_064)", call_flow)
		logger.Error("unable to decode config", "configPath", configPath)
		panic(err1)
	}

	db_type := GlobalConfig.Database.DatabaseType
	if !ApiTypes.IsValidDBType(db_type) {
		err1 := fmt.Errorf("unsupported database type (%s->CWB_CFG_076): %s, Allowed:postgres|mysql",
			call_flow, GlobalConfig.Database.DatabaseType)
		logger.Error("db_type not supported", "db_type", db_type)
		panic(err1)
	}

	logger.Info("Loading config Success!", "configPath", configPath)
	return nil
}

func GetDatabaseType() string {
	return GlobalConfig.Database.DatabaseType
}

func NeedCreateTables() bool {
	return GlobalConfig.Database.NeedCreateTables
}

func GetProcessStatusTableName() string {
	return GlobalConfig.AppTableNames.TableName_ProcessStatus
}
