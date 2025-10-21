package Dashboard01

import (
	"fmt"
	"log"
	"net/http"

	"github.com/chendingplano/deepdoc/server/api/Databases"
	"github.com/labstack/echo/v4"
)

// DashboardData represents a row in your dashboard_data table
type DashboardData struct {
	RecordUUID				string  `json:"record_uuid"`
	MD5						string  `json:"md5"`
	DocName					string 	`json:"doc_name"`
	DocNum					string  `json:"doc_num"`
	FileURL					string  `json:"file_url"`
	Status					string  `json:"status"`
	DataProcStatus			string  `json:"data_proc_status"`
	DataProcAt				string  `json:"data_proc_at"`
	ChunkProcStatus 		string  `json:"chunk_proc_status"`
	ChunkProcAt 			string  `json:"chunk_proc_at"`
	EmbeddingProcStatus 	string  `json:"embedding_proc_status"`
	EmbeddingProcAt 		string  `json:"embedding_proc_at"`
	EsProcStatus 			string  `json:"es_proc_status"`
	EsProcAt 				string  `json:"es_proc_at"`
	ParserSourceTname 		string  `json:"parser_source_tname"`
	ParserContentProcTname 	string  `json:"parser_content_proc_tname"`
	ChunkTablename 			string  `json:"chunk_tablename"`
	ErrorMsg 				string  `json:"error_msg"`
	KnowledgeBaseId 		string  `json:"knowledge_base_id"`
	CreateAt 				string  `json:"create_at"`
	ProcStatusJson 			string  `json:"proc_status_json"`
}

// HandleConfigSubmission handles the config form submission
func RetrieveDataForDashboard01(c echo.Context) error {
	// Query the database for dashboard data
	log.Printf("To retrieve data for dashboard01 (MID_001_022)")

	baseStmt := `SELECT id as record_uuid, 
				IFNULL(md5, ''),
				IFNULL(std_name, ''),
				IFNULL(std_no, ''),
				IFNULL(rel_file_path, ''), 
				IFNULL(status, ''), 
				IFNULL(data_proc_status, ''),
				IFNULL(data_proc_at, ''),
				IFNULL(chunk_proc_status, ''),
				IFNULL(chunk_proc_at, ''),
				IFNULL(embedding_proc_status, ''),
				IFNULL(embedding_proc_at, ''),
				IFNULL(es_proc_status, ''),
				IFNULL(es_proc_at, ''),
				IFNULL(parser_source_tname, ''),
				IFNULL(parser_content_proc_tname, ''),
				IFNULL(chunk_tablename, ''),
				IFNULL(error_msg, ''),
				IFNULL(knowledge_base_id, ''),
				IFNULL(create_at, ''),
				IFNULL(proc_status_json, '') FROM xk_parse_file_process`

	i := 0
	for {
		log.Printf("Processing filter index: %d (MID_001_030)", i)
  		field := c.QueryParam(fmt.Sprintf("field_%d", i))
  		if field == "" { 
			break 
		}
		switch field {
			case "doc_name":
				field = "std_name"
			case "doc_num":
				field = "std_no"
		}

  		op := c.QueryParam(fmt.Sprintf("op_%d", i))
  		val := c.QueryParam(fmt.Sprintf("val_%d", i))
		logic_opr := c.QueryParam(fmt.Sprintf("logic_opr_%d", i))
		log.Printf("Received filter - field: %s, op: %s, val: %s, logic_opr: %s (MID_001_035)", field, op, val, logic_opr)
		if i == 0 {
			baseStmt += " WHERE "
		} else {
			baseStmt += " " + logic_opr + " "
		}
		baseStmt += fmt.Sprintf(" %s %s '%s' ", field, op, val)
  		i++
	}

	var stmt string
	stmt = baseStmt
	stmt += " LIMIT 100"

	log.Printf("Constructed query: %s (MID_001_060)", stmt)
	rows, err := Databases.MySql_DB_miner.Query(stmt)
	if err != nil {
		log.Printf("DB error: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to retrieve data from database (MID_001_029)",
		})
	}
	defer rows.Close()

	var results []DashboardData
	for rows.Next() {
		var row DashboardData
		if err := rows.Scan(&row.RecordUUID, 
						&row.MD5, 
						&row.DocName,
						&row.DocNum, 
						&row.FileURL, 
						&row.Status, 
						&row.DataProcStatus,
						&row.DataProcAt,
						&row.ChunkProcStatus,
						&row.ChunkProcAt,
						&row.EmbeddingProcStatus,
						&row.EmbeddingProcAt,
						&row.EsProcStatus,
						&row.EsProcAt,
						&row.ParserSourceTname,
						&row.ParserContentProcTname,
						&row.ChunkTablename,
						&row.ErrorMsg,
						&row.KnowledgeBaseId,
						&row.CreateAt,
						&row.ProcStatusJson); err != nil {
			log.Printf("Row scan error: %v (MID_001_039)", err)
			continue
		}
		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		log.Printf("Rows error: %v (MID_001_046)", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Error reading data from database (MID_001_048)",
		})
	}

	log.Printf("Retrieved %d rows (MID_001_052)", len(results))
	return c.JSON(http.StatusOK, results)
}
