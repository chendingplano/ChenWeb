package database

import (
	"github.com/chendingplano/deepdoc/server/api/appdatastores"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func CreateTables(logger ApiTypes.JimoLogger) error {
	appdatastores.CreateProcessStatusTable(logger)
	appdatastores.CreateDocumentsTable(logger)
	return nil
}
