package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Nerzal/gocloak/v13"
	openfga "github.com/openfga/go-sdk"
	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/httpapi"
	"github.com/naira-project/naira/catalog/internal/plugins"
	"github.com/naira-project/naira/catalog/internal/auth"
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

	fgaClient, err := auth.SetupOpenfgaClient(config.OpenfgaBaseURL, config.OpenfgaStoreName, config.OpenfgaSchemaPath)
	if err != nil {
		logger.Fatalf("OpenFGA client could not be configured: %v", err)
	}

	seedTuples := []openfga.TupleKey{
		{User: "user:sample-user-id", Relation: "viewer", Object: "naira_io_model:model/mlflow/fraud-detector"},
	}
	if err := fgaClient.WriteTuples(seedTuples); err != nil {
		logger.Fatalf("writing openfga tuples: %v", err)
	}

	keycloak := gocloak.NewClient(config.KeycloakBaseURL)
	router := httpapi.NewRouter(service, logger, auth.KeycloakConfig{
		Client: keycloak,
		Realm:  config.KeycloakRealm,
	}, fgaClient)
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
