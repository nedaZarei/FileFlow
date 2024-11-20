package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/nedaZarei/FileFlow/producer/pkg/model"
	"github.com/segmentio/kafka-go"
)

type UploadHandler struct {
	db     *sql.DB
	writer *kafka.Writer
}

func NewUploadHandler(db *sql.DB, writer *kafka.Writer) *UploadHandler {
	return &UploadHandler{
		db:     db,
		writer: writer,
	}
}

func (h *UploadHandler) HandleUpload(c echo.Context) error {
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
