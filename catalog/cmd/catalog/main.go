package main

import (
	"catalog/internal/httpapi"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"catalog/internal/catalog"
	"catalog/internal/plugins"
)

func main() {
	logger := log.New(os.Stdout, "catalog ", log.LstdFlags)

	config, err := loadConfig()
	if err != nil {
		logger.Fatalf("invalid configuration: %v", err)
	}

	httpClient := &http.Client{Timeout: config.HTTPTimeout}
	registeredPlugins := plugins.Register(config.Plugins, httpClient, logger)
	service := catalog.NewService(catalog.NewMemoryStore(), logger, registeredPlugins...)
	router := httpapi.NewRouter(service, logger)
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", config.Port),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	logger.Printf("starting catalog on :%d", config.Port)
	logger.Printf("mlflow plugin source: %s", config.Plugins.MLflow.BaseURL)
	logger.Printf("litellm plugin source: %s", config.Plugins.LiteLLM.BaseURL)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("server failed: %v", err)
	}
}
