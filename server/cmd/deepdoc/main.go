package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/chendingplano/deepdoc/server/api"
	"github.com/chendingplano/deepdoc/server/api/database"
	"github.com/chendingplano/deepdoc/server/cmd/config"
	shared_api "github.com/chendingplano/shared/go/api"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/libmanager"
	"github.com/chendingplano/shared/go/api/sysdatastores"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func ExitApp() {
	databaseutil.CloseDatabase()
	libmanager.ExitLib()
}

func main() {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	new_ctx := context.WithValue(ctx, ApiTypes.CallFlowKey, "CWB_DDM_035")

	log.Printf("load .env (CWB_DDM_198)")
	// Load .env from project root (../../.env relative to cmd/mirai)
	// err := godotenv.Load("../../../.env")
	env_path := "./.env"
	err := godotenv.Load(env_path)
	if err != nil {
		error_msg := fmt.Sprintf("Warning: Could not load .env file from:%s, err: %v (CWB_DDM_200)", env_path, err)
		log.Printf("***** Alarm:%s", error_msg)

		// Try loading from current directory as fallback
		err = godotenv.Load()
		if err != nil {
			error_msg = fmt.Sprintf("Warning: Could not load .env file from current directory (CWB_DDM_210): %v", err)
			log.Printf("***** %s", error_msg)
		} else {
			log.Printf("load .env file from current directory (CWB_DDM_212)")
		}
	}

	// Verify Google OAuth environment variables are loaded
	log.Printf("Verifying Google OAuth env vars (CWB_DMN_052):")
	log.Printf("  GOOGLE_OAUTH_CLIENT_ID:%s", os.Getenv("GOOGLE_OAUTH_CLIENT_ID"))
	log.Printf("  GOOGLE_OAUTH_REDIRECT_URL:%s", os.Getenv("GOOGLE_OAUTH_REDIRECT_URL"))
	log.Printf("  APP_DOMAIN_NAME:%s", os.Getenv("APP_DOMAIN_NAME"))

	err = config.LoadConfig(new_ctx, "./config.toml")
	if err != nil {
		log.Fatal(err)
	}

	// load libconfig.toml
	libmanager.LoadLibConfig(new_ctx, "../Shared/libconfig.toml")

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

	log.Printf("To Init DB (CWB_DDM_095)")
	db_type := ApiTypes.DatabaseInfo.DBType
	databaseutil.InitDB(new_ctx, config.MySQLConfig, config.PGConfig)
	log.Printf("To Init DB (CWB_DDM_098)... Finished, db_type:%s", db_type)

	// Make sure the db is created and valid.
	var db *sql.DB
	switch db_type {
	case ApiTypes.MysqlName:
		log.Println("Set Mysql db (CWB_MAN_104)")
		db = ApiTypes.DatabaseInfo.MySQLDBHandle

	case ApiTypes.PgName:
		log.Println("Set PG db (CWB_MAN_105)")
		db = ApiTypes.DatabaseInfo.PGDBHandle

	default:
		error_msg := fmt.Sprintf("unrecognized db_type (CWB_MAN_112):%s", db_type)
		log.Printf("***** %s", error_msg)
		os.Exit(1)
	}

	if db == nil {
		error_msg := fmt.Sprintf("db is nil (CWB_MAN_077), db_type:%s. Check the config!", db_type)
		log.Printf("***** %s", error_msg)
		os.Exit(1)
	}

	// Init the library
	new_ctx1 := ApiUtils.AddCallFlow(new_ctx, "CWB_MAN_115")
	libmanager.InitLib(new_ctx1, "../Shared/libconfig.toml")

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

	var server_port = os.Getenv("SERVER_PORT")
	if server_port == "" {
		log.Fatal("***** Alarm: missing SERVER_PORT env var (CWB_MAN_145)")
		os.Exit(1)
	}
	var pp = fmt.Sprintf(":%s", server_port)
	log.Printf("[API] ⇨ http server started on (CWB_DDM_143): %s", pp)
	e.Logger.Fatal(e.Start(pp))
}
