package database

import (
	"github.com/chendingplano/deepdoc/server/api/appdatastores"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func CreateTables(logger ApiTypes.JimoLogger) error {
	appdatastores.CreateProcessStatusTable(logger)
	appdatastores.CreateDocumentsTable(logger)
	if err := appdatastores.CreateFlowsTable(logger); err != nil {
		return err
	}
	appdatastores.CreateDspyPromptsTable(logger)
	if err := appdatastores.CreateDocGenQueriesTable(logger); err != nil {
		return err
	}
	if err := appdatastores.CreateDocGenJobsTable(logger); err != nil {
		return err
	}
	if err := appdatastores.CreateDocGenLogTable(logger); err != nil {
		return err
	}
	if err := appdatastores.CreateDocBenchmarkAdminConfigTable(logger); err != nil {
		return err
	}
	if err := appdatastores.CreateDocBenchmarkAdminJobsTable(logger); err != nil {
		return err
	}
	return nil
}
