package appdatastores

import (
	"database/sql"
	"fmt"

	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
	_ "github.com/lib/pq"
)

// CreateFlowsTable creates the flows table and its supporting index.
// Pre-condition: confirm that users(user_id) exists in the schema
// before running this migration in production.
func CreateFlowsTable(logger ApiTypes.JimoLogger) error {
	db_type := ApiTypes.DBType
	table_name := config.AppConfig.AppTableNames.TableName_Flows
	var stmt string
	var db *sql.DB = ApiTypes.ProjectDBHandle
	const common_fields = "user_id           BIGINT           NOT NULL, " +
		"flow_name         VARCHAR(255)     NOT NULL, " +
		"flow_desc         TEXT             DEFAULT NULL, " +
		"is_default        BOOLEAN          NOT NULL DEFAULT FALSE, " +
		"is_shared         BOOLEAN          NOT NULL DEFAULT FALSE, " +
		"is_template       BOOLEAN          NOT NULL DEFAULT FALSE, " +
		"template_category VARCHAR(100)     DEFAULT NULL, " +
		"flow_data         LONGTEXT         NOT NULL DEFAULT '{\"nodes\":[],\"edges\":[]}', " +
		"thumbnail_svg     TEXT             DEFAULT NULL, " +
		"created_at        TIMESTAMP        NOT NULL DEFAULT CURRENT_TIMESTAMP, " +
		"updated_at        TIMESTAMP        NOT NULL DEFAULT CURRENT_TIMESTAMP"

	switch db_type {
	case ApiTypes.MysqlName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" +
			"flow_id BIGINT AUTO_INCREMENT PRIMARY KEY, " + common_fields +
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;"

	case ApiTypes.PgName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" +
			"flow_id BIGSERIAL PRIMARY KEY, " + common_fields + ")"

	default:
		err := fmt.Errorf("database type not supported:%s (CWB_FLOW_001)", db_type)
		return err
	}

	err := databaseutil.ExecuteStatement(db, stmt)
	if err != nil {
		error_msg := fmt.Errorf("failed creating table (CWB_FLOW_002), err: %w, stmt:%s", err, stmt)
		return error_msg
	}

	// Create partial unique index for default flows per user.
	// For MySQL, we'll create a regular index since MySQL doesn't support partial indexes the same way.
	var indexStmt string
	switch db_type {
	case ApiTypes.MysqlName:
		indexStmt = "CREATE UNIQUE INDEX IF NOT EXISTS flows_user_default_idx " +
			"ON " + table_name + " (user_id) WHERE is_default = TRUE;"
	case ApiTypes.PgName:
		indexStmt = "CREATE UNIQUE INDEX IF NOT EXISTS flows_user_default_idx " +
			"ON " + table_name + " (user_id) WHERE is_default = TRUE;"
	}

	if indexStmt != "" {
		if err := databaseutil.ExecuteStatement(db, indexStmt); err != nil {
			logger.Error("CreateFlowsTable: failed to create index: " + err.Error())
			// Don't fail on index creation - table is still usable
		}
	}

	logger.Info("Creating table success", "tablename", table_name)

	return nil
}
