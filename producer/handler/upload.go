package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/nedaZarei/FileFlow/config"
	"github.com/nedaZarei/FileFlow/pkg/model"
	"github.com/segmentio/kafka-go"
)

type UploadHandler struct {
	cfg    *config.Config
	e      *echo.Echo
	db     *sql.DB
	writer *kafka.Writer
}

func NewUploadHandler(db *sql.DB, writer *kafka.Writer, cfg *config.Config) *UploadHandler {
	return &UploadHandler{
		e:      echo.New(),
		cfg:    cfg,
		db:     db,
		writer: writer,
	}
}

func (h *UploadHandler) Start() error {
	//setting up echo server with middleware
	h.e.Use(middleware.Logger())
	h.e.Use(middleware.Recover())

	//api routes (for backward compatability)
	v1 := h.e.Group("/api/v1")
	v1.POST("/upload", h.uploadFile)

	if err := h.e.Start("localhost" + h.cfg.Server.Port); err != nil {
		return fmt.Errorf("failed to start server: %v", err)
	}
	return nil
}

func (h *UploadHandler) uploadFile(c echo.Context) error {
	var fileUpload model.FileUpload
	if err := c.Bind(&fileUpload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
	}

	var id int64
	err := h.db.QueryRow(`
		INSERT INTO files (file_url, bucket_name, object_name)
		VALUES ($1, $2, $3)
		RETURNING id`,
		fileUpload.FileURL, fileUpload.BucketName, fileUpload.ObjectName,
	).Scan(&id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	fileUpload.ID = id

	//send to Kafka
	message, err := json.Marshal(fileUpload)
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
