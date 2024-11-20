package db

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

type PostgresConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
}

func NewPostgresConnection(config PostgresConfig) (*sql.DB, error) {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		config.Host, config.Port, config.User, config.Password, config.DBName)

	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		return nil, fmt.Errorf("error connecting to the database: %w", err)
	}

	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("error pinging the database: %w", err)
	}

	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS files (
            id SERIAL PRIMARY KEY,
            file_url VARCHAR(500) NOT NULL,
            bucket_name VARCHAR(100) NOT NULL,
            object_name VARCHAR(100) NOT NULL,
            etag VARCHAR(100),
            size BIGINT
        )
    `)
	if err != nil {
		return nil, fmt.Errorf("error creating tables: %w", err)
	}

	return db, nil
}
