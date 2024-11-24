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
	retryReader *kafka.Reader
	writer      *kafka.Writer // for writing to retry topic
	minioClient *minio.Client
	httpClient  *http.Client
}

const (
	maxRetries       = 3
	retryTopicSuffix = "-retry"
)

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

	//creating writer for retry topic
	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers: []string{cfg.Kafka.Broker},
		Topic:   cfg.Kafka.Topic + retryTopicSuffix,
	})

	return &ConsumerHandler{
		cfg:        cfg,
		httpClient: httpClient,
		writer:     writer,
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

	//main topic reader
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{h.cfg.Kafka.Broker},
		Topic:   h.cfg.Kafka.Topic,
		GroupID: h.cfg.Kafka.GroupID,
	})
	h.reader = reader

	//retry topic reader
	retryReader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{h.cfg.Kafka.Broker},
		Topic:   h.cfg.Kafka.Topic + retryTopicSuffix,
		GroupID: h.cfg.Kafka.GroupID + "-retry",
	})
	h.retryReader = retryReader

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

	//start both consumers
	h.consumeMain()
	h.consumeRetry()

	return nil
}

func (h *ConsumerHandler) consumeMain() {
	fmt.Println("consumeMain")
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

		if err := h.processFile(ctx, fileUpload, 0); err != nil {
			log.Printf("error processing file: %v", err)
			h.sendToRetryTopic(ctx, fileUpload, err, 0)
		}
	}
}

func (h *ConsumerHandler) consumeRetry() {
	fmt.Println("consumeRetry")
	ctx := context.Background()
	for {
		message, err := h.retryReader.ReadMessage(ctx)
		if err != nil {
			log.Printf("error reading retry message: %v", err)
			continue
		}

		var retryFile model.RetryFileUpload
		if err := json.Unmarshal(message.Value, &retryFile); err != nil {
			log.Printf("error unmarshaling retry message: %v", err)
			continue
		}

		if retryFile.RetryCount >= maxRetries {
			log.Printf("dropping file after %d retries: %s", maxRetries, retryFile.FileURL)
			h.logFailedFile(ctx, retryFile)
			continue
		}

		if err := h.processFile(ctx, retryFile.FileUpload, retryFile.RetryCount+1); err != nil {
			log.Printf("retry %d failed for file: %v", retryFile.RetryCount+1, err)
			h.sendToRetryTopic(ctx, retryFile.FileUpload, err, retryFile.RetryCount+1)
		}
	}
}

func (h *ConsumerHandler) processFile(ctx context.Context, fileUpload model.FileUpload, retryCount int) error {
	//download file
	resp, err := h.httpClient.Get(fileUpload.FileURL)
	if err != nil {
		return fmt.Errorf("download error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	//read response body
	const maxSize = 100 * 1024 * 1024 //100MB limit
	btt, err := io.ReadAll(io.LimitReader(resp.Body, maxSize))
	if err != nil {
		return fmt.Errorf("read error: %v", err)
	}

	//upload to Minio
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
		return fmt.Errorf("minio upload error: %v", err)
	}

	result, err := h.db.ExecContext(ctx,
		`UPDATE files 
         SET etag = $1, 
             size = $2,
             retry_count = $3,
             status = 'completed'
         WHERE file_url = $4`,
		info.ETag,
		info.Size,
		retryCount,
		fileUpload.FileURL,
	)
	if err != nil {
		return fmt.Errorf("database update error: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil || rowsAffected == 0 {
		return fmt.Errorf("database verification error: %v", err)
	}

	log.Printf("successfully processed file %s on attempt %d", fileUpload.ObjectName, retryCount+1)
	return nil
}

func (h *ConsumerHandler) sendToRetryTopic(ctx context.Context, fileUpload model.FileUpload, processError error, retryCount int) {
	retryFile := model.RetryFileUpload{
		FileUpload: fileUpload,
		RetryCount: retryCount,
		Error:      processError.Error(),
	}

	message, err := json.Marshal(retryFile)
	if err != nil {
		log.Printf("error marshaling retry message: %v", err)
		return
	}

	err = h.writer.WriteMessages(ctx, kafka.Message{
		Value: message,
	})
	if err != nil {
		log.Printf("error writing to retry topic: %v", err)
	}

	_, err = h.db.ExecContext(ctx,
		`UPDATE files 
         SET retry_count = $1,
             status = 'pending_retry',
             last_error = $2
         WHERE file_url = $3`,
		retryCount,
		processError.Error(),
		fileUpload.FileURL,
	)
	if err != nil {
		log.Printf("error updating retry status in database: %v", err)
	}
}

func (h *ConsumerHandler) logFailedFile(ctx context.Context, retryFile model.RetryFileUpload) {
	_, err := h.db.ExecContext(ctx,
		`UPDATE files 
         SET status = 'failed',
             retry_count = $1,
             last_error = $2
         WHERE file_url = $3`,
		retryFile.RetryCount,
		retryFile.Error,
		retryFile.FileUpload.FileURL,
	)
	if err != nil {
		log.Printf("error updating failed status in database: %v", err)
	}
}
