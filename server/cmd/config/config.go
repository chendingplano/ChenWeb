// config/config.go
package config

import (
	"context"
	"fmt"
	"log"

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
	} `mapstructure:"app_table_names"`

	Auth struct {
		JWTSecret            string `mapstructure:"jwt_secret"`
		SessionDurationHours int    `mapstructure:"session_duration_hours"`
	} `mapstructure:"auth"`
}

var GlobalConfig Config
var MySQLConfig ApiTypes.DBConfig
var PGConfig ApiTypes.DBConfig

func LoadConfig(ctx context.Context, configPath string) error {
	call_flow := ctx.Value(ApiTypes.CallFlowKey).(string)
	log.Printf("Loading config from %s (CWB_CFG_047)", configPath)
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
	ApiTypes.DatabaseInfo.PGDBHandle = ApiTypes.PG_DB_miner
	ApiTypes.DatabaseInfo.MySQLDBHandle = ApiTypes.MySql_DB_miner

	log.Printf("(%s->CWB_CFG_115) Config load success, database_type:%s, need_create_tables:%t, pg:%t, mysql:%t",
		call_flow,
		GlobalConfig.Database.DatabaseType,
		GlobalConfig.Database.NeedCreateTables,
		GlobalConfig.Database.CreatePG,
		GlobalConfig.Database.CreateMySQL,
	)

	/*
		if PGConfig.CreateFlag && ApiTypes.DatabaseInfo.PGDBHandle == nil {
			error_msg := fmt.Sprintf("(%s->CWB_CFG_125) pg db not set", call_flow)
			log.Printf("***** Alarm:%s", error_msg)
			panic(error_msg)
		}

		if MySQLConfig.CreateFlag && ApiTypes.DatabaseInfo.MySQLDBHandle == nil {
			error_msg := fmt.Sprintf("(%s->CWB_CFG_129) mysql db is set", call_flow)
			log.Printf("***** Alarm:%s", error_msg)
			panic(error_msg)
		}
	*/

	if GlobalConfig.Database.DatabaseType == "" {
		err1 := fmt.Errorf("unable to decode config (%s->CWB_CFG_064)", call_flow)
		log.Fatal(err1)
		panic(err1)
	}

	db_type := GlobalConfig.Database.DatabaseType
	if !ApiTypes.IsValidDBType(db_type) {
		err1 := fmt.Errorf("unsupported database type (%s->CWB_CFG_076): %s, Allowed:postgres|mysql",
			call_flow, GlobalConfig.Database.DatabaseType)
		log.Printf("***** Alarm %s", err1)
		panic(err1)
	}

	log.Printf("Loading config from %s (CWB_CFG_096) ... Success!", configPath)
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
