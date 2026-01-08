package confighandler

import (
	"net/http"

	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/labstack/echo/v4"
)

// ConfigResponse represents the configuration data sent to the frontend
type ConfigResponse struct {
	AppName       string              `json:"app_name"`
	Debug         bool                `json:"debug"`
	Server        ServerConfig        `json:"server"`
	Database      DatabaseConfig      `json:"database"`
	AppTableNames AppTableNamesConfig `json:"app_table_names"`
	Auth          AuthConfig          `json:"auth"`
}

type ServerConfig struct {
	Port int    `json:"port"`
	Host string `json:"host"`
}

type DatabaseConfig struct {
	CreateMySQL string `json:"create_mysql"`
	CreatePG    string `json:"create_pg"`
	PGHost      string `json:"pg_host"`
	PGPort      int    `json:"pg_port"`
	PGUserName  string `json:"pg_user_name"`
	// PGPassword excluded for security
	PGDBName      string `json:"pg_db_name"`
	MySQLHost     string `json:"mysql_host"`
	MySQLPort     int    `json:"mysql_port"`
	MySQLUserName string `json:"mysql_user_name"`
	// MySQLPassword excluded for security
	MySQLDBName      string `json:"mysql_db_name"`
	MaxConnections   int    `json:"max_connections"`
	DatabaseType     string `json:"database_type"`
	NeedCreateTables string `json:"need_create_tables"`
}

type AppTableNamesConfig struct {
	TableNameDocuments     string `json:"table_name_documents"`
	TableNameProcessStatus string `json:"table_name_process_status"`
	TableNameSchedules     string `json:"table_name_schedules"`
}

type AuthConfig struct {
	// JWTSecret excluded for security
	SessionDurationHours int `json:"session_duration_hours"`
}

// GetConfig returns the application configuration
// Endpoint: GET /api/config
func GetConfig(c echo.Context) error {
	cfg := config.GlobalConfig

	response := ConfigResponse{
		AppName: cfg.AppName,
		Debug:   cfg.Debug,
		Server: ServerConfig{
			Port: cfg.Server.Port,
			Host: cfg.Server.Host,
		},
		Database: DatabaseConfig{
			CreateMySQL: boolToString(cfg.Database.CreateMySQL),
			CreatePG:    boolToString(cfg.Database.CreatePG),
			PGHost:      cfg.Database.PGHost,
			PGPort:      cfg.Database.PGPort,
			PGUserName:  cfg.Database.PGUserName,
			// PGPassword excluded for security
			PGDBName:      cfg.Database.PGDBName,
			MySQLHost:     cfg.Database.MySQLHost,
			MySQLPort:     cfg.Database.MySQLPort,
			MySQLUserName: cfg.Database.MySQLUserName,
			// MySQLPassword excluded for security
			MySQLDBName:      cfg.Database.MySQLDBName,
			MaxConnections:   cfg.Database.MaxConnections,
			DatabaseType:     cfg.Database.DatabaseType,
			NeedCreateTables: boolToString(cfg.Database.NeedCreateTables),
		},
		AppTableNames: AppTableNamesConfig{
			TableNameDocuments:     cfg.AppTableNames.TableName_Documents,
			TableNameProcessStatus: cfg.AppTableNames.TableName_ProcessStatus,
			TableNameSchedules:     cfg.AppTableNames.TableName_Schedules,
		},
		Auth: AuthConfig{
			// JWTSecret excluded for security
			SessionDurationHours: cfg.Auth.SessionDurationHours,
		},
	}

	return c.JSON(http.StatusOK, response)
}

// Helper function to convert bool to string
func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
