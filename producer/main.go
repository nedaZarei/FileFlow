package main

import (
	"fmt"
	"log"

	"github.com/nedaZarei/FileFlow/config"
	"github.com/nedaZarei/FileFlow/handler"
	"github.com/nedaZarei/FileFlow/pkg/db"
	"github.com/nedaZarei/FileFlow/pkg/kafka"
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

	//init Kafka Writer
	writer := kafka.NewKafkaWriter(kafka.KafkaConfig{
		Brokers: []string{"kafka:9092"},
		Topic:   "file-upload-topic",
	})
	defer writer.Close()

	cfg, err := config.InitConfig("./config/config.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	fmt.Println(cfg)

	//init Handler
	uploadHandler := handler.NewUploadHandler(db, writer, cfg)

	if err := uploadHandler.Start(); err != nil {
		log.Fatalf("failed to start producer: %v", err)
	}
}
