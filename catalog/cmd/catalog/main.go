package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/httpapi"
	"github.com/naira-project/naira/catalog/internal/plugins"
)

func main() {
	logger := log.New(os.Stdout, "catalog ", log.LstdFlags)

	config, err := loadConfig()
	if err != nil {
		logger.Fatalf("invalid configuration: %v", err)
	}

	httpClient := &http.Client{Timeout: config.HTTPTimeout}
	registeredPlugins, cleanup, err := plugins.Register(config.Plugins, httpClient, logger)
	if err != nil {
		logger.Fatalf("failed to register plugins: %v", err)
	}
	defer cleanup()

	service := catalog.NewService(catalog.NewMemoryStore(), logger, registeredPlugins...)
	router := httpapi.NewRouter(service, logger)
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", config.Port),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Printf("starting catalog on :%d", config.Port)
		logger.Printf("mlflow plugin source: %s", config.Plugins.MLflow.BaseURL)
		logger.Printf("litellm plugin source: %s", config.Plugins.LiteLLM.BaseURL)
		if config.Plugins.PluginsDir != "" {
			logger.Printf("plugins directory: %s", config.Plugins.PluginsDir)
		}

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("server failed: %v", err)
		}
	}()

	<-sigChan
	logger.Println("shutting down catalog service...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Printf("error during server shutdown: %v", err)
	}
}
