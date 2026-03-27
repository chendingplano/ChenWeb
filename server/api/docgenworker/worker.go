// server/api/docgenworker/worker.go
package docgenworker

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/appdatastores"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

// JobChannel is the shared buffered channel for job IDs.
var JobChannel chan int64

// Start initialises the worker pool. Call once after DB is ready.
func Start(db *sql.DB, jobCh chan int64, workerCount int) {
	JobChannel = jobCh
	logger := loggerutil.CreateDefaultLogger("CWB_DGW_100")
	for i := 0; i < workerCount; i++ {
		go func(workerID int) {
			logger.Info("doc gen worker started", "worker_id", workerID)
			for jobID := range jobCh {
				func() {
					defer func() {
						if r := recover(); r != nil {
							logger.Error("worker panic", "worker_id", workerID, "job_id", jobID, "panic", r)
							if err := appdatastores.UpdateDocGenJobStatus(db, jobID,
								appdatastores.DocGenJobStatusFailed,
								fmt.Sprintf("worker panic: %v", r)); err != nil {
								logger.Error("update job status after panic failed", "job_id", jobID, "err", err)
							}
						}
					}()
					if err := processJob(db, jobID); err != nil {
						logger.Error("processJob failed", "job_id", jobID, "err", err)
					}
				}()
			}
		}(i)
	}
}

// RequeueStalledJobs pushes any pending/processing jobs back onto the channel
// so they are retried after a server restart.
func RequeueStalledJobs(db *sql.DB, jobCh chan int64) error {
	ids, err := appdatastores.ListStalledDocGenJobs(db)
	if err != nil {
		return fmt.Errorf("RequeueStalledJobs: %w (CWB_DGW_110)", err)
	}
	for _, id := range ids {
		jobCh <- id
	}
	return nil
}

func processJob(db *sql.DB, jobID int64) error {
	logger := loggerutil.CreateDefaultLogger("CWB_DGW_120")

	job, err := appdatastores.GetDocGenJob(db, jobID)
	if err != nil {
		return fmt.Errorf("job %d fetch failed (CWB_DGW_125): %w", jobID, err)
	}
	if job == nil {
		return fmt.Errorf("job %d not found (CWB_DGW_126)", jobID)
	}

	if err := appdatastores.UpdateDocGenJobStatus(db, jobID, appdatastores.DocGenJobStatusProcessing, ""); err != nil {
		return err
	}

	converter, err := ValidateConverter(job.Converter)
	if err != nil {
		appdatastores.UpdateDocGenJobStatus(db, jobID, appdatastores.DocGenJobStatusFailed, err.Error())
		return err
	}

	rows, err := db.Query(job.SQLStatement)
	if err != nil {
		msg := fmt.Sprintf("SQL error: %v", err)
		appdatastores.UpdateDocGenJobStatus(db, jobID, appdatastores.DocGenJobStatusFailed, msg)
		return fmt.Errorf("%s (CWB_DGW_135)", msg)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		msg := fmt.Sprintf("columns error: %v", err)
		appdatastores.UpdateDocGenJobStatus(db, jobID, appdatastores.DocGenJobStatusFailed, msg)
		return fmt.Errorf("%s (CWB_DGW_142)", msg)
	}

	outDir := filepath.Join(job.OutputDir, job.RequestName)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		msg := fmt.Sprintf("mkdir error: %v", err)
		appdatastores.UpdateDocGenJobStatus(db, jobID, appdatastores.DocGenJobStatusFailed, msg)
		return fmt.Errorf("%s (CWB_DGW_140)", msg)
	}

	var total, success, fail int
	for rows.Next() {
		total++
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			fail++
			if logErr := appdatastores.InsertDocGenLogEntry(db, appdatastores.DocGenLogEntry{
				JobID: jobID, RequestName: job.RequestName, Purpose: job.Purpose,
				Filename: "", Status: "failed", ErrorMsg: err.Error(),
				Remarks: job.Remarks, CreatedBy: job.CreatedBy,
			}); logErr != nil {
				logger.Error("insert log entry failed", "job_id", jobID, "err", logErr)
			}
			continue
		}

		rowMap := make(map[string]string, len(cols))
		for i, col := range cols {
			if vals[i] != nil {
				rowMap[col] = fmt.Sprintf("%v", vals[i])
			}
		}

		// Build template tokens: converter maps sqlCol → tokenName
		tokens := make(map[string]string, len(converter))
		for sqlCol, tokenName := range converter {
			tokens[tokenName] = rowMap[sqlCol]
		}

		customerID := rowMap[findKeyByValue(converter, "customer_id")]
		customerName := rowMap[findKeyByValue(converter, "customer_name")]
		email := rowMap[findKeyByValue(converter, "email")]
		phoneNum := rowMap[findKeyByValue(converter, "phone_num")]

		filename := fmt.Sprintf("%s_%04d.%s", job.RequestName, total, job.OutputFormat)
		outPath := filepath.Join(outDir, filename)

		var renderErr error
		switch strings.ToLower(job.TemplateType) {
		case "word":
			renderErr = RenderDocx(job.TemplatePath, outPath, tokens)
		default:
			renderErr = fmt.Errorf("template_type %q not supported (CWB_DGW_155)", job.TemplateType)
		}

		entry := appdatastores.DocGenLogEntry{
			JobID: jobID, RequestName: job.RequestName,
			CustomerID: customerID, CustomerName: customerName,
			Email: email, PhoneNum: phoneNum,
			Purpose: job.Purpose, Filename: filename,
			Remarks: job.Remarks, CreatedBy: job.CreatedBy,
		}
		if renderErr != nil {
			fail++
			entry.Status = "failed"
			entry.ErrorMsg = renderErr.Error()
		} else {
			success++
			entry.Status = "generated"
		}
		if logErr := appdatastores.InsertDocGenLogEntry(db, entry); logErr != nil {
			logger.Error("insert log entry failed", "job_id", jobID, "err", logErr)
		}
	}

	finalStatus := appdatastores.DocGenJobStatusCompleted
	if success == 0 && (fail > 0 || total == 0) {
		finalStatus = appdatastores.DocGenJobStatusFailed
	}
	if err := appdatastores.UpdateDocGenJobCounts(db, jobID, total, success, fail); err != nil {
		logger.Error("update job counts failed", "job_id", jobID, "err", err)
	}
	if err := appdatastores.UpdateDocGenJobStatus(db, jobID, finalStatus, ""); err != nil {
		logger.Error("update job status failed", "job_id", jobID, "err", err)
	}
	logger.Info("job done", "job_id", jobID, "total", total, "success", success, "fail", fail)
	return nil
}

// findKeyByValue returns the first key in m whose value equals val.
func findKeyByValue(m map[string]string, val string) string {
	for k, v := range m {
		if v == val {
			return k
		}
	}
	return ""
}
