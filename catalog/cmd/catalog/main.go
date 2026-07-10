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
	"github.com/openfga/language/pkg/go/transformer"
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

	dsl := `
model
  schema 1.1

type user

type naira_io_model
      relations
        define owner: [user]
        define member: [user] or owner
        define viewer: [user] or member

        define get: viewer
        define list: member
        define watch: member
        
        define can_list_litellm: viewer
        define can_list_mlflow: viewer
        define can_update_mlflow: owner
        define can_update_litellm: owner
`
	jsonStr, err := transformer.TransformDSLToJSON(dsl)
	if err != nil {
		log.Fatalf("failed to transform DSL to JSON: %v", err)
	}

	_, _, err = auth.SetupOpenfgaClient(config.OpenfgaBaseURL, "Naira", jsonStr)
	if err != nil {
		logger.Fatalf("OpenFGA client could not configured: %v", err)
	}

	seedTuples := []openfga.TupleKey{
		{User: "user:sample-user-id", Relation: "viewer", Object: "naira_io_model:model/mlflow/fraud-detector"},
	}
	if err := auth.WriteTuples(seedTuples); err != nil {
		logger.Fatalf("writing openfga tuples: %v", err)
	}

	keycloak := gocloak.NewClient(config.KeycloakBaseURL)
	router := httpapi.NewRouter(service, logger, auth.KeycloakConfig{
		Client: keycloak,
		Realm:  config.KeycloakRealm,
	})
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
