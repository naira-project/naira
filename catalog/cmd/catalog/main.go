package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/Nerzal/gocloak/v13"
	"github.com/naira-project/naira/catalog/internal/auth/keycloak"
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

	registeredPlugins, cleanup, err := pluginmanager.Register(config.PluginAddresses, config.PluginConnectionTimeout, logger)
	if err != nil {
		logger.Fatalf("failed to register plugins: %v", err)
	}
	defer cleanup()

	keycloakClient := gocloak.NewClient(config.KeycloakBaseURL)
	service := catalog.NewService(catalog.NewMemoryStore(), registeredPlugins, logger)
	router, err := httpapi.NewRouter(service, logger, keycloak.Config{
		Client: keycloakClient,
		Realm:  config.KeycloakRealm,
	})
	if err != nil {
		logger.Fatalf("failed to create router: %v", err)
	}
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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), config.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Printf("error during server shutdown: %v", err)
	}
}
