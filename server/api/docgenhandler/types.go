package docgenhandler

import "github.com/chendingplano/deepdoc/server/api/appdatastores"

type SubmitJobRequest struct {
	RequestName  string            `json:"request_name"`
	Purpose      string            `json:"purpose"`
	Remarks      string            `json:"remarks"`
	SQLQueryID   *int64            `json:"sql_query_id"`
	SQLStatement string            `json:"sql_statement"`
	TemplateType string            `json:"template_type"`
	TemplateName string            `json:"template_name"`
	Converter    map[string]string `json:"converter"`
	OutputDir    string            `json:"output_dir"`
	OutputFormat string            `json:"output_format"`
}

type CreateQueryRequest struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	SQLStatement string `json:"sql_statement"`
}

type UpdateQueryRequest struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	SQLStatement string `json:"sql_statement"`
}

type SubmitJobResponse struct {
	Status bool  `json:"status"`
	JobID  int64 `json:"job_id"`
}

type JobListResponse struct {
	Status   bool                      `json:"status"`
	Jobs     []appdatastores.DocGenJob `json:"jobs"`
	Total    int64                     `json:"total"`
	Page     int                       `json:"page"`
	PageSize int                       `json:"page_size"`
}

type JobDetailResponse struct {
	Status bool                           `json:"status"`
	Job    *appdatastores.DocGenJob       `json:"job"`
	Logs   []appdatastores.DocGenLogEntry `json:"logs"`
}

type QueryListResponse struct {
	Status  bool                        `json:"status"`
	Queries []appdatastores.DocGenQuery `json:"queries"`
}

type TemplateListResponse struct {
	Status    bool     `json:"status"`
	Templates []string `json:"templates"`
}

type ErrorResponse struct {
	Status   bool   `json:"status"`
	ErrorMsg string `json:"error_msg"`
}
