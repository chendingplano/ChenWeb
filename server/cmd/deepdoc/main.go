package main

import (
	"fmt"
	"log"

	"github.com/chendingplano/deepdoc/server/api"
	"github.com/chendingplano/deepdoc/server/api/Auth"
	"github.com/chendingplano/deepdoc/server/api/Databases"
	"github.com/chendingplano/deepdoc/server/cmd/config"
)

func main() {
	// Make sure main.go is run in ChenWeb
	err := config.LoadConfig("./config.toml")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Starting %s on %s:%d (MID_DDM_020)\n",
		config.GlobalConfig.AppName,
		config.GlobalConfig.Server.Host,
		config.GlobalConfig.Server.Port)

	e, err := api.RegisterRoutes()
	if err != nil {
	 	panic(err)
	}

	log.Printf("To Init DB")
	Databases.InitDB()

	// Create necessary tables
	if config.NeedCreateTables() {
		log.Printf("To Create Tables")
		err = Databases.AosCreateTables()
		if err != nil {
			log.Fatal("***** Alarm: Failed creating tables (MID_DDM_030)", err)
		}
	}

	Auth.RegisterRoutes(e)

	// Start the HTTP server on port 8080
	log.Println("[API] ⇨ http server started on (MID_DDM_035) [::]:8080")
	e.Logger.Fatal(e.Start(":8080"))
}
