package handler

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	_ "github.com/lib/pq"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/segmentio/kafka-go"

	"github.com/nedaZarei/FileFlow/config"
	"github.com/nedaZarei/FileFlow/pkg/db"
	"github.com/nedaZarei/FileFlow/pkg/model"
)

type ConsumerHandler struct {
	cfg         *config.Config
	db          *sql.DB
	reader      *kafka.Reader
	minioClient *minio.Client //implements amazon S3 compatible method
	httpClient  *http.Client
}

func NewHandler(cfg *config.Config) *ConsumerHandler {
	//creating http client with redirect handling and longer timeout
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:    &tls.Config{InsecureSkipVerify: true},
			MaxIdleConns:       100,
			IdleConnTimeout:    90 * time.Second,
			DisableCompression: true,
		},
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			//up to 10 redirects
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return nil
		},
	}

	return &ConsumerHandler{
		cfg:        cfg,
		httpClient: httpClient,
	}
}

func (h *ConsumerHandler) Start() error {
	//init db
	dbConn, dbErr := db.NewPostgresConnection(db.PostgresConfig{
		Host:     "db",
		Port:     5432,
		User:     "postgres",
		Password: "postgres",
		DBName:   "simpleapi_database",
	})
	if dbErr != nil {
		log.Fatalf("failed to connect to database: %v", dbErr)
	}
	defer dbConn.Close()
	h.db = dbConn

	//kafka init
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{h.cfg.Kafka.Broker},
		Topic:   h.cfg.Kafka.Topic,
		GroupID: h.cfg.Kafka.GroupID,
	})
	defer reader.Close()
	h.reader = reader

	h.httpClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		MaxIdleConns:       100,
		IdleConnTimeout:    90 * time.Second,
		DisableCompression: true,
	}

	//minio init
	var err error
	h.minioClient, err = minio.New(h.cfg.Minio.Endpoint, &minio.Options{
		Creds:     credentials.NewStaticV4(h.cfg.Minio.AccessKey, h.cfg.Minio.SecretKey, ""),
		Secure:    true,
		Transport: transport,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize Minio client: %v", err)
	}
	log.Println("connected to Minio")

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

		//download file
		resp, err := h.httpClient.Get(fileUpload.FileURL)
		if err != nil {
			log.Printf("error downloading file from %s: %v", fileUpload.FileURL, err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			log.Printf("unexpected status code downloading file: %d", resp.StatusCode)
			continue
		}

		// Read response body
		const maxSize = 100 * 1024 * 1024 // 100MB limit
		btt, err := io.ReadAll(io.LimitReader(resp.Body, maxSize))
		if err != nil {
			log.Printf("error reading response body: %v", err)
			continue
		}

		//upload to Minio with content type detection
		contentType := http.DetectContentType(btt)
		info, err := h.minioClient.PutObject(
			ctx,
			fileUpload.BucketName,
			fileUpload.ObjectName,
			bytes.NewReader(btt),
			int64(len(btt)),
			minio.PutObjectOptions{
				ContentType: contentType,
			},
		)
		if err != nil {
			log.Printf("error uploading file to Minio: %v", err)
			log.Printf("upload details - bucket: %s, object: %s, size: %d, content-type: %s",
				fileUpload.BucketName, fileUpload.ObjectName, len(btt), contentType)
			continue
		}

		_, err = h.db.Exec(`
		UPDATE files 
		SET etag = $1, 
			size = $2
		WHERE id = $3`,
			info.ETag,
			info.Size,
			fileUpload.ID,
		)
		if err != nil {
			log.Printf("error updating database: %v", err)
			log.Printf("failed update for file ID: %d, ETag: %s, Size: %d",
				fileUpload.ID, info.ETag, info.Size)
			continue
		}

		//generating presigned URL
		url, err := h.minioClient.PresignedGetObject(ctx, fileUpload.BucketName,
			fileUpload.ObjectName, time.Hour, nil)
		if err != nil {
			log.Printf("error generating presigned URL: %v", err)
			continue
		}

		log.Printf("successfully processed file %s. presigned URL: %s", fileUpload.ObjectName, url.String())
	}
}
