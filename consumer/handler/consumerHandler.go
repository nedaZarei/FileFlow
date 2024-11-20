package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	_ "github.com/lib/pq"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/segmentio/kafka-go"

	"github.com/nedaZarei/FileFlow/consumer/config"
	"github.com/nedaZarei/FileFlow/consumer/pkg/model"
)

type ConsumerHandler struct {
	cfg         *config.Config
	e           *echo.Echo
	db          *sql.DB
	reader      *kafka.Reader
	minioClient *minio.Client //implements amazon S3 compatible method
}

func NewHandler(cfg *config.Config) *ConsumerHandler {
	return &ConsumerHandler{
		e:   echo.New(),
		cfg: cfg}
}

func (h *ConsumerHandler) Start() error {
	//kafka init
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{"kafka:9092"},
		Topic:   "file-upload-topic",
		GroupID: "file-processor-group",
	})
	defer reader.Close()
	h.reader = reader

	//minio init
	var err error
	h.minioClient, err = minio.New(h.cfg.Minio.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(h.cfg.Minio.AccessKey, h.cfg.Minio.SecretKey, ""),
		Secure: true,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize Minio client: %v", err)
	}
	log.Println("connected to Minio")

	//setting up echo server with middleware
	h.e.Use(middleware.Logger())
	h.e.Use(middleware.Recover())

	//api routes (for backward compatability)
	v1 := h.e.Group("/api/v1")
	v1.POST("/store", h.storingFile)

	if err := h.e.Start("localhost" + h.cfg.Server.Port); err != nil {
		return fmt.Errorf("failed to start server: %v", err)
	}

	return nil
}

func (h *ConsumerHandler) storingFile(c echo.Context) error {
	ctx := context.Background()
	for {
		message, err := h.reader.ReadMessage(ctx)
		if err != nil {
			log.Printf("error reading message: %v", err)
			continue
		}

		var fileUpload model.FileUpload
		if err := json.Unmarshal(message.Value, &fileUpload); err != nil {
			log.Printf("error unmarshaling message: %v", err)
			continue
		}

		//downloading file from URL
		resp, err := http.Get(fileUpload.FileURL)
		if err != nil {
			log.Printf("error downloading file: %v", err)
			continue
		}

		//uploading it to Minio
		info, err := h.minioClient.PutObject(ctx, fileUpload.BucketName, fileUpload.ObjectName,
			resp.Body, -1, minio.PutObjectOptions{ContentType: "application/octet-stream"})
		if err != nil {
			log.Printf("error uploading file: %v", err)
			continue
		}

		_, err = h.db.Exec(`
            UPDATE files 
            SET etag = $1, size = $2 
            WHERE id = $3`,
			info.ETag, info.Size, fileUpload.ID,
		)
		if err != nil {
			log.Printf("error updating database: %v", err)
			continue
		}

		//generating presigned URL
		url, err := h.minioClient.PresignedGetObject(ctx, fileUpload.BucketName,
			fileUpload.ObjectName, time.Hour, nil)
		if err != nil {
			log.Printf("error generating presigned URL: %v", err)
			continue
		}

		log.Printf("successfully processed file. presigned URL: %s", url.String())
	}
}
