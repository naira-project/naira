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
	"github.com/naira-project/naira/catalog/internal/pluginmanager"
)

func main() {
	logger := log.New(os.Stdout, "catalog ", log.LstdFlags)

	config, err := loadConfig()
	if err != nil {
		logger.Fatalf("invalid configuration: %v", err)
	}

	registeredPlugins, cleanup, err := pluginmanager.Register(config.PluginAddresses, logger, config.PluginConnectionTimeout)
	if err != nil {
		logger.Fatalf("failed to register plugins: %v", err)
	}
	defer cleanup()

	service := catalog.NewService(catalog.NewMemoryStore(), logger, registeredPlugins)
	router := httpapi.NewRouter(service, logger)
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", config.Port),
		Handler:           router,
		ReadHeaderTimeout: config.ReadHeadersTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Printf("starting catalog on :%d", config.Port)
		for name, addr := range config.PluginAddresses {
			logger.Printf("plugin %q -> %s", name, addr)
		}

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("server failed: %v", err)
		}
	}()

	<-ctx.Done()
	logger.Println("shutting down catalog service...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Printf("error during server shutdown: %v", err)
	}
}
