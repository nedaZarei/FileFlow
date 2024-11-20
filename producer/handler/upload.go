package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

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

func (h *UploadHandler) HandleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var fileUpload model.FileUpload
	if err := json.NewDecoder(r.Body).Decode(&fileUpload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var id int64
	err := h.db.QueryRow(`
        INSERT INTO files (file_url, bucket_name, object_name)
        VALUES ($1, $2, $3)
        RETURNING id`,
		fileUpload.FileURL, fileUpload.BucketName, fileUpload.ObjectName,
	).Scan(&id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fileUpload.ID = id

	//send to Kafka
	message, err := json.Marshal(fileUpload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = h.writer.WriteMessages(r.Context(), kafka.Message{
		Value: message,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "file upload initiated successfully",
		"id":      fmt.Sprintf("%d", id),
	})
}
