package main

import (
	"flag"
	"log"

	"novelstudio/internal/config"
	"novelstudio/internal/database"
)

func main() {
	settings, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	databasePath := flag.String("db", settings.DatabasePath, "path to the SQLite database")
	outputPath := flag.String("out", "", "backup destination; defaults to a timestamped sibling file")
	flag.Parse()

	backupPath, err := database.CreateBackup(*databasePath, *outputPath)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("SQLite backup created: %s", backupPath)
}
