package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/nedaZarei/FileFlow/config"
	"github.com/nedaZarei/FileFlow/pkg/db"
	"github.com/nedaZarei/FileFlow/pkg/model"
	"github.com/segmentio/kafka-go"
)

type UploadHandler struct {
	cfg    *config.Config
	e      *echo.Echo
	db     *sql.DB
	writer *kafka.Writer
}

func NewUploadHandler(cfg *config.Config) *UploadHandler {
	return &UploadHandler{
		e:   echo.New(),
		cfg: cfg,
	}
}

func (h *UploadHandler) Start() error {
	//init db
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
	h.db = db

	//init Kafka writer
	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers: []string{h.cfg.Kafka.Broker},
		Topic:   h.cfg.Kafka.Topic,
	})
	defer writer.Close()
	h.writer = writer

	//setting up echo server with middleware
	h.e.Use(middleware.Logger())
	h.e.Use(middleware.Recover())

	v1 := h.e.Group("/api/v1")
	v1.POST("/upload", h.uploadFile)

	if err := h.e.Start("0.0.0.0" + h.cfg.Server.Port); err != nil {
		return fmt.Errorf("failed to start server: %v", err)
	}
	return nil
}

func (h *UploadHandler) uploadFile(c echo.Context) error {
	var fileRequest model.FileUpload

	//binding request body to model
	if err := c.Bind(&fileRequest); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
	}

	//inserting into database
	var id int64
	err := h.db.QueryRow(
		`INSERT INTO files (file_url, bucket_name, object_name) 
        VALUES ($1, $2, $3) RETURNING id`,
		fileRequest.FileURL, fileRequest.BucketName, fileRequest.ObjectName,
	).Scan(&id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	fileRequest.ID = id
	//send to Kafka
	message, err := json.Marshal(fileRequest)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	err = h.writer.WriteMessages(c.Request().Context(), kafka.Message{
		Value: message,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusAccepted, map[string]interface{}{
		"message": "file upload initiated successfully",
		"id":      strconv.FormatInt(id, 10),
	})
}
