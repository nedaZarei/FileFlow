package main

import (
	"fmt"
	"log"

	"github.com/nedaZarei/FileFlow/consumer/config"
	"github.com/nedaZarei/FileFlow/consumer/handler"
	"github.com/nedaZarei/FileFlow/consumer/pkg/db"
)

func main() {
	db, err := db.NewPostgresConnection(db.PostgresConfig{
		Host:     "db",
		Port:     5432,
		User:     "postgres",
		Password: "postgres",
		DBName:   "simpleapi_database",
	})
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	cfg, err := config.InitConfig("./config/config.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	fmt.Println(cfg)

	consumerHandler := handler.NewHandler(cfg)

	if err := consumerHandler.Start(); err != nil {
		log.Fatalf("failed to start consumer handler: %v", err)
	}
}
