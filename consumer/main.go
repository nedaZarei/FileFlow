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

	consumerHandler := handler.NewHandler(cfg)

	if err := consumerHandler.Start(); err != nil {
		log.Fatalf("failed to start consumer: %v", err)
	}
}
