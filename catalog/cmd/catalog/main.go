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
	"github.com/naira-project/naira/catalog/internal/operations"
	"github.com/naira-project/naira/catalog/internal/pluginmanager"
	"github.com/naira-project/naira/catalog/internal/pluginrun"
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

	var keycloakClient keycloak.TokenDecoder
	if config.AuthEnabled {
		keycloakClient = gocloak.NewClient(config.KeycloakBaseURL)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store := catalog.NewMemoryStore()
	catalogService := catalog.NewService(store)
	runner := pluginrun.NewRunner(ctx, store, operations.NewMemoryStore(), registeredPlugins, config.PluginTimeout, logger)

	router, err := httpapi.NewRouter(catalogService, runner, logger, config.AuthEnabled, keycloak.Config{
		Client: keycloakClient,
		Realm:  config.KeycloakRealm,
		Issuer: config.KeycloakIssuer,
	})
	if err != nil {
		logger.Fatalf("failed to create router: %v", err)
	}
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", config.Port),
		Handler:           router,
		ReadHeaderTimeout: config.ReadHeadersTimeout,
	}

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

	// Wait for any in-flight plugin runs to finish before exiting.
	runner.Wait()
	logger.Println("catalog service stopped")
}
