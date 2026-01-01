package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/chendingplano/deepdoc/server/api"
	"github.com/chendingplano/deepdoc/server/api/database"
	"github.com/chendingplano/deepdoc/server/cmd/config"
	shared_api "github.com/chendingplano/shared/go/api"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/libmanager"
	"github.com/chendingplano/shared/go/api/sysdatastores"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func ExitApp() {
	databaseutil.CloseDatabase()
	libmanager.ExitLib()
}

func main() {
	// Make sure main.go is run in ChenWeb
	err := config.LoadConfig("./config.toml")
	if err != nil {
		log.Fatal(err)
	}

	defer ExitApp()

	fmt.Printf("Starting %s on %s:%d (CWB_DDM_020)\n",
		config.GlobalConfig.AppName,
		config.GlobalConfig.Server.Host,
		config.GlobalConfig.Server.Port)

	e := echo.New()

	// ✅ Enable CORS
	log.Printf("Configure CORS (CWB_DDM_045)")
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     []string{"http://localhost:5173"}, // frontend
		AllowMethods:     []string{echo.GET, echo.POST, echo.PUT, echo.DELETE, echo.OPTIONS},
		AllowHeaders:     []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
		AllowCredentials: true,
	}))

	log.Printf("Register api Routes (CWB_DDM_052)")
	err = api.RegisterRoutes(e)
	if err != nil {
		panic(err)
	}

	log.Printf("To Init DB")
	db_type := ApiTypes.DatabaseInfo.DBType
	databaseutil.InitDB(config.MySQLConfig, config.PGConfig)
	log.Printf("To Init DB ... Finished, db_type:%s", db_type)

	// Make sure the db is created and valid.
	var db *sql.DB
	switch db_type {
	case ApiTypes.MysqlName:
		log.Println("set mysql db")
		db = ApiTypes.DatabaseInfo.MySQLDBHandle

	case ApiTypes.PgName:
		log.Println("set pg db")
		db = ApiTypes.DatabaseInfo.PGDBHandle

	default:
		error_msg := fmt.Sprintf("unrecognized db_type:%s", db_type)
		log.Printf("***** %s", error_msg)
		os.Exit(1)
	}

	if db == nil {
		error_msg := fmt.Sprintf("db is nil (CWB_MAN_077), db_type:%s. Check the config!", db_type)
		log.Printf("***** %s", error_msg)
		os.Exit(1)
	}

	// Init the library
	libmanager.InitLib("../Shared/libconfig.toml")

	// Create tables
	if config.NeedCreateTables() {
		log.Printf("To Create Tables (CWB_DDM_036)")
		sysdatastores.CreateTables()
		err = database.CreateTables()
		if err != nil {
			log.Fatal("***** Alarm: Failed creating tables (CWB_DDM_030)", err)
		}
	} else {
		log.Printf("Not creating tables (CWB_DDM_041)")
	}

	log.Printf("Register Shared Routes (CWB_DDM_055)")
	shared_api.RegisterRoutes(e)

	// Start the HTTP server on port 8080
	log.Println("[API] ⇨ http server started on (CWB_DDM_035) [::]:8080")
	e.Logger.Fatal(e.Start(":8080"))
}
