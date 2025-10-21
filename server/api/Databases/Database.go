package Databases

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/chendingplano/deepdoc/server/api/DataStructures"
	"github.com/chendingplano/deepdoc/server/cmd/config"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
)

type AosDBConfig struct {
    Host     string
    Port     int
}

var PG_DB_miner *sql.DB
var MySql_DB_miner *sql.DB
const sg_mysql_name = "mysql"
const sg_pg_name = "pg"

func InitDB() error {
    err := AosCreatePGDBMiner()
    if err != nil {
        log.Fatal("***** Alarm Failed creating PG connection (MID_DBS_026)", err)
        return err
    }

    err = AosCreateMySqlDBMiner()
    if err != nil {
        log.Fatal("***** Alarm Failed creating MySQL connection (MID_DBS_032)", err)
        return err    
    }
    return nil
}

func AosCreatePGDBMiner() error {
    var err error
    host := "127.0.0.1"
    port := 5432
    username := "admin"
    password := `plano4628` 
    dbname := "miner"
    connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s sslmode=disable dbname=%s",
        host, port, username, password, dbname)
    
    PG_DB_miner, err = sql.Open("postgres", connStr)
    if err != nil {
        log.Fatal("Failed to connect to database (MID_DBS_050):", err)
    }
    
    // Test the connection
    if err = PG_DB_miner.Ping(); err != nil {
        log.Fatal("***** Alarm: Failed to ping database (MID_DBS_055):", err)
    }
    
    log.Println("Connected to PostgreSQL database (MID_DBS_058)")

    // err = AosCreateTables(MySql_DB_miner)
    // if err != nil {
    //     log.Printf("***** Alarm: Failed creating tables:%s (MID_DBS_062)", err.Error())
    //     return err
    // }

    return nil
}

func AosCreateMySqlDBMiner() error {
    var err error
    // host := "192.168.0.20"
    // port := 3306
    // username := "root"
    // password := `rainsome123!@#` 
    // dbname := "miner"
    // '!' %21
    // '@' %40
    // '#' %23
    connStr := "root:rainsome123!@#@tcp(192.168.0.20:3306)/miner?tls=false&parseTime=true&loc=Local&timeout=30s&readTimeout=30s&writeTimeout=30s"
    
    log.Printf("To connect to MySQL with connStr (MID_DBS_081):%s", connStr)
    MySql_DB_miner, err = sql.Open("mysql", connStr)
    if err != nil {
        log.Fatal("Failed to connect to database (MID_DBS_084):", err)
        return err
    }
    
    // Test the connection
    if err = MySql_DB_miner.Ping(); err != nil {
        log.Fatal("***** Alarm: Failed to ping database (MID_DBS_090):", err)
        return err
    }
    
    log.Println("Connected to MySQL database")

    return nil
}

func AosCreateTables() error {
    // This function creates all the tables.
    var db *sql.DB
    database_type := config.GetDatabaseType()
    if database_type == sg_mysql_name {
        db = MySql_DB_miner
    } else if database_type == sg_pg_name {
        db = PG_DB_miner
    } else {
        return fmt.Errorf("***** Unrecognized database type (MID_DBS_124): %s", database_type)
    }

    AosCreateCustomerTable(db, database_type)
    AosCreateLoginSessionsTable(db, database_type)
    return nil
}

// InsertConfigDataWithValidation inserts a new record with additional validation
func AosInsertCustomerData(
	ctx context.Context,
	dbname string,
	tablename string,
	data *DataStructures.AosCustomer) error {

    var db *sql.DB
    db_type := config.GetDatabaseType()
    var query string
    switch db_type {
    case sg_mysql_name:
         db = MySql_DB_miner
         query = fmt.Sprintf("INSERT INTO %s (user_name, date_of_birth, email, phone_number, education, is_married, number_of_kids, created_at) " +
        				 "VALUES (?, ?, ?, ?, ?, ?, ?, ?)", tablename)
    case sg_pg_name:
         db = PG_DB_miner
         query = fmt.Sprintf("INSERT INTO %s (user_name, date_of_birth, email, phone_number, education, is_married, number_of_kids, created_at) " +
        				 "VALUES ($1, $2, $3, $4, $5, $6, $7, $8)", tablename)
    default:
         return fmt.Errorf("***** Unrecognized database type (MID_DBS_124): %s", db_type)
    }
    
    if data.CreatedAt.IsZero() {
        data.CreatedAt = DataStructures.CustomTime{Time: time.Now()}
    }
    
    log.Printf("Inserting record: %+v\n", *data)

    _, err := db.ExecContext(ctx, query,
        data.UserName,
        data.DateOfBirth,
        data.Email,
        data.PhoneNumber,
        data.Education,
        data.IsMarried,
        data.NumberOfKids,
        data.CreatedAt,
    )
    
    if err != nil {
        log.Printf("Statement:%s", query)
        return fmt.Errorf("***** Alarm failed to insert config data (MID_DBS_148): %w", err)
    }
    
    return nil
}

func AosExecuteStatement(db_type string, stmt string) error {
    switch db_type {
    case sg_mysql_name:
         return ExecuteStatement(MySql_DB_miner, stmt)
    case sg_pg_name:
         return ExecuteStatement(PG_DB_miner, stmt)
    default:
         return fmt.Errorf("unsupported database type (MID_DBS_153): %s", db_type)
    }
}

func ExecuteStatement(db *sql.DB, stmt string) error {
    tx, err := db.Begin()
    if err != nil {
        return fmt.Errorf("failed to begin transaction (MID_DBS_158): %w", err);
    }

    defer func() {
        _ = tx.Rollback(); // Rollback if not committed
    } ()

    _, err1 := tx.Exec(stmt)
    if err1 != nil {
        return fmt.Errorf("failed to execute query in transaction (MID_DBS_166): %w", err1)
    }
    
    // Commit the transaction
    if err := tx.Commit(); err != nil {
        return fmt.Errorf("failed to commit transaction (MID_DBS_171): %w", err)
    }
    
    log.Println("Statement executed successfully (MID_DBS_175)")
    return nil
}

func SaveSession(
            login_method string,
            session_id string, 
            user_name string, 
            user_name_type string,
            user_reg_id string,
            expiry time.Time) error {
    db_type := config.GlobalConfig.Database.DatabaseType
    switch db_type {
    case sg_mysql_name:
         stmt := fmt.Sprintf(`INSERT INTO %s (login_method, session_id, status,
                    user_name, user_name_type, user_reg_id, expires_at)
              VALUES (?, ?, ?, ?, ?, ?, ?)
              ON DUPLICATE KEY UPDATE 
              session_id = VALUES(session_id), 
              status = "active",
              expires_at = VALUES(expires_at)`, config.AosGetLoginSessionsTableName())
    
         _, err := MySql_DB_miner.Exec(stmt, login_method, session_id, "active",
                user_name, user_name_type, user_reg_id, expiry)
         if err != nil {
            error_msg := fmt.Sprintf("failed to save session (mysql) (MID_DBS_195): %v, stmt:%s", err, stmt)
            log.Printf("***** Alarm %s", error_msg)
            return fmt.Errorf("***** Alarm:%s", error_msg)
         }
         return nil
    case sg_pg_name:
         stmt := fmt.Sprintf(`INSERT INTO %s (login_method, session_id, status,
                    user_name, user_name_type, user_re_id, expires_at)
            VALUES ($1, $2, $3, $4, $5, $6, $7)
            ON DUPLICATE KEY UPDATE 
            session_id = VALUES(session_id), 
            status = "active",
            expires_at = VALUES(expires_at)`, config.AosGetLoginSessionsTableName())
    
         _, err := PG_DB_miner.Exec(stmt, login_method, session_id, "active",
            user_name, user_name_type, user_reg_id, expiry)
         if err != nil {
            error_msg := fmt.Sprintf("failed to save session (pg) (MID_DBS_213): %v, stmt:%s", err, stmt)
            log.Printf("***** Alarm %s", error_msg)
            return fmt.Errorf("***** Alarm:%s", error_msg)
         }
         return nil
    default:
         return fmt.Errorf("unsupported database type (MID_DBS_234): %s", db_type)
    }
}

func IsValidSession(session_id string) (bool, error) {
    db_type := config.GlobalConfig.Database.DatabaseType
    log.Printf("Check IsValidSession (MID_DBS_251), db_type:%s", db_type)
    var db *sql.DB
    switch db_type {
    case sg_mysql_name:
         db = MySql_DB_miner

    case sg_pg_name:
         db = PG_DB_miner

    default:
         log.Printf("***** Alarm unrecognized database type (MID_DBS_234): %s", db_type)
         return false, fmt.Errorf("unsupported database type (MID_DBS_234): %s", db_type)
    }

    tablename := config.AosGetLoginSessionsTableName()
    query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE session_id = ? AND expires_at > NOW()", tablename)
    var count int
    err := db.QueryRow(query, session_id).Scan(&count)
    if err != nil {
        error_msg := fmt.Errorf("failed to validate session (MID_DBS_195): %w", err)
        log.Printf("***** Alarm:%s", error_msg)
        return false, error_msg
    }
    log.Printf("Check session (MID_DBS_271), stmt: %s, count:%d", query, count)
    return count > 0, nil
}