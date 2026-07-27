package confighandler

import (
	"net/http"

	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

// ConfigResponse represents the configuration data sent to the frontend
type ConfigResponse struct {
	AppName               string                  `json:"app_name"`
	Debug                 bool                    `json:"debug"`
	Server                ServerConfig            `json:"server"`
	Database              ApiTypes.DatabaseConfig `mapstructure:"database"`
	AppTableNames         AppTableNamesConfig     `json:"app_table_names"`
	Auth                  AuthConfig              `json:"auth"`
	EnableLoginWithGithub bool                    `json:"enable_login_with_github"`
	EnableLoginWithGoogle bool                    `json:"enable_login_with_google"`
	EnablePhoneLogin      bool                    `json:"enable_phone_login"`
}

type ServerConfig struct {
	Port int    `json:"port"`
	Host string `json:"host"`
}

/*
type DatabaseConf struct {
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
*/

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
	response := ConfigResponse{
		AppName: ApiTypes.CommonConfig.AppInfo.AppName,
		Server: ServerConfig{
			Port: ApiTypes.CommonConfig.AppInfo.AppPort,
			Host: ApiTypes.CommonConfig.AppInfo.AppHost,
		},
		AppTableNames: AppTableNamesConfig{
			TableNameDocuments:     config.AppConfig.AppTableNames.TableName_Documents,
			TableNameProcessStatus: config.AppConfig.AppTableNames.TableName_ProcessStatus,
			TableNameSchedules:     config.AppConfig.AppTableNames.TableName_Schedules,
		},
		Auth: AuthConfig{
			// JWTSecret excluded for security
			SessionDurationHours: ApiTypes.CommonConfig.Auth.SessionDurationHours,
		},
		EnableLoginWithGithub: config.GetEnableLoginWithGithub(),
		EnableLoginWithGoogle: config.GetEnableLoginWithGoogle(),
		EnablePhoneLogin:      config.GetEnablePhoneLogin(),
	}

	return c.JSON(http.StatusOK, response)
}

// Helper function to convert bool to string
func BoolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
