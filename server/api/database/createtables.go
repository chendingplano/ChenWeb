package database

import (
	"github.com/chendingplano/deepdoc/server/api/appdatastores"
)

func CreateTables() error {
    appdatastores.CreateProcessStatusTable()
    return nil
}