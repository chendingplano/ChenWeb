// config/config.go
package config

import (
	"fmt"
	"log"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/spf13/viper"
)

type Config struct {
	AppName 		string `mapstructure:"app_name"`
	Debug   		bool   `mapstructure:"debug"`
	HomeURL 		string `mapstructure:"home_url"`

	Server struct {
		Port 		int    `mapstructure:"port"`
		Host 		string `mapstructure:"host"`
	} `mapstructure:"server"`

	Database struct {
		CreateMySQL 		string `mapstructure:"create_mysql"`
		CreatePG 			string `mapstructure:"create_pg"`
		DatabaseType		string `mapstructure:"database_type"`
		PGHost         		string `mapstructure:"pg_host"`
		PGPort         		int    `mapstructure:"pg_port"`
		PGUserName     		string `mapstructure:"pg_user_name"`
		PGPassword     		string `mapstructure:"pg_password"`
		PGDBName       		string `mapstructure:"pg_db_name"`
		MySQLHost      		string `mapstructure:"mysql_host"`
		MySQLPort      		int    `mapstructure:"mysql_port"`
		MySQLUserName  		string `mapstructure:"mysql_user_name"`
		MySQLPassword  		string `mapstructure:"mysql_password"`
		MySQLDBName    		string `mapstructure:"mysql_db_name"`
		MaxConnections 		int    `mapstructure:"max_connections"`
		NeedCreateTables	string `mapstructure:"need_create_tables"`
	} `mapstructure:"database"`

	SystemTableNames struct {
		TableName_Sessions 			string `mapstructure:"table_name_login_sessions"`
		TableName_SessionLog 		string `mapstructure:"table_name_session_log"`
		TableName_Users 			string `mapstructure:"table_name_users"`
		TableName_IDMgr 			string `mapstructure:"table_name_id_mgr"`
		TableName_ActivityLog 		string `mapstructure:"table_name_activity_log"`
		TableName_EmailStore		string `mapstructure:"table_name_email_store"`
		TableName_PromptStore		string `mapstructure:"table_name_prompt_store"`
	} `mapstructure:"system_table_names"`

	AppTableNames struct {
		TableName_ProcessStatus 	string `mapstructure:"table_name_process_status"`
	} `mapstructure:"app_table_names"`

	Auth struct {
		JWTSecret           string `mapstructure:"jwt_secret"`
		SessionDurationHours int  `mapstructure:"session_duration_hours"`
	} `mapstructure:"auth"`
}

var GlobalConfig Config
var MySQLConfig ApiTypes.DBConfig
var PGConfig ApiTypes.DBConfig

func LoadConfig(configPath string) error {
	log.Printf("Loading config from %s (CWB_CFG_047)", configPath)
	viper.SetConfigFile(configPath)
	viper.SetConfigType("toml")

	// Optional: set defaults
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("debug", false)

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return fmt.Errorf("config file not found (CWB_CFG_054): %s", configPath)
		}
		return fmt.Errorf("error reading config (CWB_CFG_056): %w", err)
	}

	// Override with environment variables (e.g., DATABASE_URL)
	viper.AutomaticEnv()

	// Unmarshal into struct
	if err := viper.Unmarshal(&GlobalConfig); err != nil {
		return fmt.Errorf("unable to decode config (CWB_CFG_064): %w", err)
	}

	MySQLConfig.Host = GlobalConfig.Database.MySQLHost
	MySQLConfig.Port = GlobalConfig.Database.MySQLPort
	MySQLConfig.DBType = "mysql"
	MySQLConfig.CreateFlag = GlobalConfig.Database.CreateMySQL == "true"
	MySQLConfig.UserName = GlobalConfig.Database.MySQLUserName
	MySQLConfig.Password = GlobalConfig.Database.MySQLPassword
	MySQLConfig.DbName = GlobalConfig.Database.MySQLDBName

	PGConfig.Host = GlobalConfig.Database.PGHost
	PGConfig.Port = GlobalConfig.Database.PGPort
	PGConfig.DBType = "pg"
	PGConfig.CreateFlag = GlobalConfig.Database.CreatePG == "true"
	PGConfig.UserName = GlobalConfig.Database.PGUserName
	PGConfig.Password = GlobalConfig.Database.PGPassword
	PGConfig.DbName = GlobalConfig.Database.PGDBName

	ApiTypes.DatabaseInfo.DBType 				= GlobalConfig.Database.DatabaseType
	ApiTypes.DatabaseInfo.PGDBName 				= GlobalConfig.Database.PGDBName
	ApiTypes.DatabaseInfo.MySQLDBName 			= GlobalConfig.Database.MySQLDBName
	ApiTypes.DatabaseInfo.PGDBHandle 			=	ApiTypes.PG_DB_miner
	ApiTypes.DatabaseInfo.MySQLDBHandle 		=	ApiTypes.MySql_DB_miner
	ApiTypes.DatabaseInfo.HomeURL 				= GlobalConfig.HomeURL

	log.Printf("(CWB_CFG_115) Config load success, database_type:%s, need_create_tables:%s",
	 		GlobalConfig.Database.DatabaseType,
			GlobalConfig.Database.NeedCreateTables)

	if	ApiTypes.DatabaseInfo.PGDBHandle != nil {
		log.Println("(CWB_CFG_125) pg db is set")
	}

	if	ApiTypes.DatabaseInfo.MySQLDBHandle!= nil {
		log.Println("(CWB_CFG_129) mysql db is set")
	}

	if GlobalConfig.Database.DatabaseType == "" {
		err1 := fmt.Errorf("unable to decode config (CWB_CFG_064)")
		log.Fatal(err1)
		panic(err1)
	}

	db_type := GlobalConfig.Database.DatabaseType
	if !ApiTypes.IsValidDBType(db_type) {
		err1 := fmt.Errorf("unsupported database type (CWB_CFG_076): %s, Allowed:postgres|mysql",
			GlobalConfig.Database.DatabaseType)
		log.Printf("***** Alarm %s", err1)
		panic(err1)
	}

	log.Printf("Loading config from %s (CWB_CFG_096) ... Success!", configPath)
	return nil
}

func GetLoginSessionsTableName() string {
	return GlobalConfig.SystemTableNames.TableName_Sessions
}

func GetDatabaseType() string {
	return GlobalConfig.Database.DatabaseType
}

func GetUsersTableName() string {
	return GlobalConfig.SystemTableNames.TableName_Users
}

func NeedCreateTables() bool {
	log.Printf("NeedCreateTables:%s", GlobalConfig.Database.NeedCreateTables)
	return GlobalConfig.Database.NeedCreateTables == "true"
}

func GetProcessStatusTableName() string {
	return GlobalConfig.AppTableNames.TableName_ProcessStatus
}