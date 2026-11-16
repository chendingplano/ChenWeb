// doc-recovery is a short-lived, timer-driven recovery sweep. It never loads
// prompt/model runtime and relies on the application's centralized DB pool.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"strings"
	"time"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/joho/godotenv"
	"github.com/nats-io/nats.go"
)

func main() {
	configFile := flag.String("config", "", "application configuration path")
	flag.Parse()
	_ = godotenv.Load(".env")
	logger := loggerutil.CreateDefaultLogger("20261116-418")
	defer logger.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, ApiTypes.CallFlowKey, "20261116-419")
	path := *configFile
	if path == "" {
		path = os.Getenv("DOC_PROCESSOR_CONFIG")
	}
	if path == "" {
		path = "config.toml"
	}
	if err := config.LoadConfig(ctx, logger, path); err != nil {
		logger.Error("doc recovery config failed", "error", err)
		os.Exit(1)
	}
	if err := databaseutil.InitDB(ctx, ApiTypes.CommonConfig); err != nil {
		logger.Error("doc recovery database failed", "error", err)
		os.Exit(1)
	}
	defer databaseutil.CloseDatabase(ApiTypes.CommonConfig)
	url := os.Getenv("NATS_URL")
	if url == "" {
		url = nats.DefaultURL
	}
	nc, err := nats.Connect(url, nats.Timeout(5*time.Second))
	if err != nil {
		logger.Error("doc recovery NATS failed", "error", err)
		os.Exit(1)
	}
	defer nc.Close()
	js, err := nc.JetStream()
	if err != nil {
		logger.Error("doc recovery JetStream failed", "error", err)
		os.Exit(1)
	}
	report, err := docprocessing.RecoverPendingEvents(ctx, ApiTypes.ProjectDBHandle, js, docprocessing.RecoveryOptions{Sources: recoverySources()})
	if err != nil {
		logger.Error("doc recovery sweep failed", "error", err)
		os.Exit(1)
	}
	raw, _ := json.Marshal(report)
	logger.Info("doc recovery sweep", "report", string(raw))
	if len(report.MissingEvents) > 0 {
		logger.Warn("doc recovery requires operator attention: original event or line file unavailable", "record_ids", report.MissingEvents)
		writer := docprocessing.RoutingAlarmSQLWriter{DB: ApiTypes.ProjectDBHandle}
		for _, id := range report.MissingEvents {
			if err := writer.WriteAlarm(ctx, docprocessing.RoutingAlarm{Kind: "doc_recovery_original_unavailable", Severity: docprocessing.RoutingAlarmSeverityError, RecordID: id, Message: "Document recovery cannot proceed: retained original event, user attribution, or line file is unavailable; operator attention is required"}); err != nil {
				logger.Error("persist document recovery operator alarm failed", "record_id", id, "error", err)
			}
		}
	}
}

func recoverySources() []docprocessing.RecoverySource {
	primary := strings.TrimSpace(os.Getenv("DOC_PROCESSOR_EVENT_SUBJECT"))
	if primary == "" {
		primary = docprocessing.DefaultEventSubject
	}
	durable := strings.TrimSpace(os.Getenv("DOC_PROCESSOR_EVENT_DURABLE"))
	if durable == "" {
		durable = "doc-processor"
	}
	result := []docprocessing.RecoverySource{}
	seen := map[string]bool{}
	for _, subject := range []string{primary, docprocessing.DefaultEventSubject, "kb.line-file-generated"} {
		if seen[subject] {
			continue
		}
		seen[subject] = true
		name := durable
		if subject != primary {
			if subject == docprocessing.DefaultEventSubject {
				name += "-start-doc-processing"
			} else if subject == "kb.line-file-generated" {
				name += "-line-file-generated"
			} else {
				name += "-compat"
			}
		}
		result = append(result, docprocessing.RecoverySource{Subject: subject, Durable: name})
	}
	return result
}
