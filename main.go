package main

import (
	"log"

	"lazervaultGo/configs"
	"lazervaultGo/database"
)

func main() {

	// load config
	config, err := configs.LoadConfig(".")
	if err != nil {
		log.Fatal("Cannot load config:", err)
	}

	// Debug print
	log.Printf("Loaded config: %+v", config)

	// connect to database
	db, err := database.ConnectDB(config)
	if err != nil {
		log.Fatal("Cannot connect to database: ", err)
	}

	// auto migrate
	err = database.AutoMigrateDB(db)
	if err != nil {
		log.Fatal("Cannot auto migrate database: ", err)
	}

	log.Println("Successfully connected to database")

}
