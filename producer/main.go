package main

import (
	"fmt"
	"log"

	"github.com/nedaZarei/FileFlow/config"
	"github.com/nedaZarei/FileFlow/handler"
)

func main() {
	cfg, err := config.InitConfig("/app/config.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	fmt.Println(cfg)

	//init handler
	uploadHandler := handler.NewUploadHandler(cfg)

	if err := uploadHandler.Start(); err != nil {
		log.Fatalf("failed to start producer: %v", err)
	}
}
