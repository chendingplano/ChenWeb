// config/config.go
package config

import (
	"fmt"
	"log"

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
		SessionsTableName 	string `mapstructure:"table_name_login_sessions"`
		MaxConnections 		int    `mapstructure:"max_connections"`
		needCreateTables	bool   `mapstructure:"need_create_tables"`
	} `mapstructure:"database"`

	Auth struct {
		JWTSecret           string `mapstructure:"jwt_secret"`
		SessionDurationHours int  `mapstructure:"session_duration_hours"`
	} `mapstructure:"auth"`
}

var GlobalConfig Config

func LoadConfig(configPath string) error {
	log.Printf("Loading config from %s (MID_CFG_042)", configPath)
	viper.SetConfigFile(configPath)
	viper.SetConfigType("toml")

	// Optional: set defaults
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("debug", false)

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return fmt.Errorf("config file not found (MID_CFG_054): %s", configPath)
		}
		return fmt.Errorf("error reading config (MID_CFG_056): %w", err)
	}

	// Override with environment variables (e.g., DATABASE_URL)
	viper.AutomaticEnv()

	// Unmarshal into struct
	if err := viper.Unmarshal(&GlobalConfig); err != nil {
		return fmt.Errorf("unable to decode config (MID_CFG_064): %w", err)
	}

	if GlobalConfig.Database.DatabaseType == "" {
		err1 := fmt.Errorf("unable to decode config (MID_CFG_064)")
		log.Fatal(err1)
		panic(err1)
	}

	if GlobalConfig.Database.DatabaseType != "postgres" && GlobalConfig.Database.DatabaseType != "mysql" {
		err1 := fmt.Errorf("unsupported database type (MID_CFG_076): %s", GlobalConfig.Database.DatabaseType)
		log.Fatal(err1)
		panic(err1)
	}

	log.Printf("Loading config from %s (MID_CFG_042) ... Success!", configPath)
	return nil
}

func AosGetLoginSessionsTableName() string {
	return GlobalConfig.Database.SessionsTableName
}

func GetDatabaseType() string {
	return GlobalConfig.Database.DatabaseType
}

func NeedCreateTables() bool {
	return GlobalConfig.Database.needCreateTables
}