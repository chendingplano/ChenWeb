package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/Databases"
)

// RunSQLPeriodically executes the given SQL statement repeatedly with a sleep interval between executions.
// This function runs infinitely until the program is terminated.
// Parameters:
//   - db: *sql.DB database connection
//   - sqlStmt: string containing the SQL statement to execute
//   - sleepSeconds: number of seconds to sleep between executions
func RunSQLPeriodically(process_table_name string, tags string, sleepSeconds int) {
    ticker := time.NewTicker(time.Duration(sleepSeconds) * time.Second)
    defer ticker.Stop()

    log.Printf("Starting periodic execution of SQL statement every %d seconds (MID_005_023).\n", sleepSeconds)
    
    for {
        // Execute the SQL statement
        // Wait for the specified duration
        generateStatusRecords(process_table_name, "status", tags)
        generateStatusRecords(process_table_name, "data_proc_status", tags)
        generateStatusRecords(process_table_name, "chunk_proc_status", tags)
        generateStatusRecords(process_table_name, "embedding_proc_status", tags)
        generateStatusRecords(process_table_name, "es_proc_status", tags)
        <-ticker.C
    }
}

type ProcessStatusRecord struct {
    StatusName string
    StatusValue string
    RcdCount int64
    Tags string
}

func generateStatusRecords(
        process_table_name string,
        status_name string,
        tags string) error {
    stmt := fmt.Sprintf("SELECT %s, count(*) as count FROM %s GROUP BY %s", status_name, process_table_name, status_name)
    log.Printf("Executing SQL statement (MID_005_041): %s\n", stmt)

    rows, err := Databases.MySql_DB_miner.Query(stmt)
    if err != nil {
		// log.Printf("***** Failed querying DB (MID_005_037): %v, stmt: %s", err, stmt)
        return fmt.Errorf("***** Alarm failed query DB (MID_005_038): %w, stmt:%s", err, stmt)
    }

    proc_status_tablename := "process_status"
	defer rows.Close()

    type RowData struct {
	    Status      sql.NullString
	    RcdCount    sql.NullInt64
    }

	var records [] ProcessStatusRecord
	for rows.Next() {
		var row RowData
		if err := rows.Scan(&row.Status, &row.RcdCount); err != nil {
			log.Printf("Row scan error: %v (MID_005_052)", err)
        }

        var rcd_count int64 = 0
        var status_value string = ""
        if row.RcdCount.Valid {
            rcd_count = row.RcdCount.Int64
        } else {
            rcd_count = 0
        }

        if row.Status.Valid {
            status_value = row.Status.String
        } else {
            status_value = "null"
        }

        record := ProcessStatusRecord{
            StatusName: status_name,
            StatusValue: status_value,
            RcdCount: rcd_count,
            Tags: tags,
        }
        log.Printf("Generated record: %+v\n", record)
        records = append(records, record)
    }

    if len(records) == 0 {
        log.Printf("***** No records found to insert into process status table (MID_005_088).")
        return nil
    }

    // Build the query
    placeholders := strings.Repeat("(?, ?, ?, ?),", len(records))
    placeholders = placeholders[:len(placeholders)-1] // Remove trailing comma
    
    insert_stmt := fmt.Sprintf("INSERT INTO %s (status_name, status_value, rcd_count, tags) VALUES %s", proc_status_tablename, placeholders)

    // Build args slice
    args := make([]interface{}, 0, len(records)*3)
    for _, record := range records {
        args = append(args, record.StatusName, record.StatusValue, record.RcdCount, record.Tags)
    }

    _, err1 := Databases.MySql_DB_miner.Exec(insert_stmt, args...)
    if err1 != nil {
        log.Printf("***** Failed inserting process status records (MID_005_080): %v, stmt: %s", err1, insert_stmt)
        return fmt.Errorf("***** Alarm failed query DB (MID_005_038): %w, stmt:%s", err, insert_stmt)
    }
    log.Printf("Inserted %d process status records into %s table (MID_005_081).\n", len(records), proc_status_tablename)
    return nil
}