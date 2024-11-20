package main

import (
	"log"

	"github.com/labstack/echo/v4"
	"github.com/nedaZarei/FileFlow/producer/handler"
	"github.com/nedaZarei/FileFlow/producer/pkg/db"
	"github.com/nedaZarei/FileFlow/producer/pkg/kafka"
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

	//init Handler
	uploadHandler := handler.NewUploadHandler(db, writer)

	//echo server
	e := echo.New()
	e.POST("/upload", uploadHandler.HandleUpload)
	log.Fatal(e.Start(":8000"))
}
