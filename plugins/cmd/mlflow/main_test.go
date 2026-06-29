package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	pluginapi "github.com/naira-project/naira/plugins/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestPlugin_Collect_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	mlflowURL, cleanup := setupMLflowContainer(ctx, t)
	defer cleanup()

	runID := seedMLflowData(t, mlflowURL)

	cfg := config{
		BaseURL:     mlflowURL,
		HTTPTimeout: 10 * time.Second,
	}
	plugin := New(http.DefaultClient, cfg)

	resp, err := plugin.Collect(ctx)
	require.NoError(t, err, "expected Collect function to succeed")

	expectedResponse := pluginapi.CollectResponse{
		Nodes: []pluginapi.NodeClaim{
			{
				ID: pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: "mlflow/fraud-detector"},
				Properties: pluginapi.PropertyMap{
					propertyKeyDescription: "Fraud detection model",
				},
			},
			{
				ID: pluginapi.NodeID{Kind: pluginapi.NodeKindDataset, Path: "mlflow/training-data"},
				Properties: pluginapi.PropertyMap{
					propertyKeyDigest:     "hash123",
					propertyKeySourceType: "s3",
				},
			},
		},
		Relations: []pluginapi.RelationClaim{
			{
				Kind: pluginapi.RelationKindTrainedOn,
				From: pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: "mlflow/fraud-detector"},
				To:   pluginapi.NodeID{Kind: pluginapi.NodeKindDataset, Path: "mlflow/training-data"},
				Properties: pluginapi.PropertyMap{
					propertyKeyRunID: runID,
				},
			},
		},
	}

	assert.ElementsMatch(t, expectedResponse.Nodes, resp.Nodes, "Struktury Nodes się nie zgadzają")
	assert.ElementsMatch(t, expectedResponse.Relations, resp.Relations, "Struktury Relations się nie zgadzają")
}

// setupMLflowContainer pulls and runs a lightweight MLflow image
func setupMLflowContainer(ctx context.Context, t *testing.T) (string, func()) {
	req := testcontainers.ContainerRequest{
		Image:        "ghcr.io/mlflow/mlflow:v3.1.4",
		ExposedPorts: []string{"5000/tcp"},
		Cmd: []string{
			"mlflow", "server",
			"--host", "0.0.0.0",
			"--port", "5000",
		},
		WaitingFor: wait.ForHTTP("/health").WithPort("5000/tcp").WithStartupTimeout(2 * time.Minute),
	}

	mlflowC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start MLflow container")

	ip, err := mlflowC.Host(ctx)
	require.NoError(t, err)

	port, err := mlflowC.MappedPort(ctx, "5000")
	require.NoError(t, err)

	baseURL := fmt.Sprintf("http://%s:%s", ip, port.Port())

	cleanupFunc := func() {
		err := mlflowC.Terminate(ctx)
		require.NoError(t, err, "failed to terminate MLflow container")
	}

	return baseURL, cleanupFunc
}

// seedMLflowData generates mock data using the MLflow REST API and returns the generated runID
func seedMLflowData(t *testing.T, baseURL string) string {
	// 1. Create a new Run
	runRes := postJSON(t, baseURL+"/api/2.0/mlflow/runs/create", map[string]any{
		"experiment_id": "0",
	})
	runInfo := runRes["run"].(map[string]any)["info"].(map[string]any)
	runID := runInfo["run_id"].(string)

	// 2. Log Dataset to the attached Run
	postJSON(t, baseURL+"/api/2.0/mlflow/runs/log-inputs", map[string]any{
		"run_id": runID,
		"datasets": []map[string]any{
			{
				"dataset": map[string]any{
					"name":        "training-data",
					"digest":      "hash123",
					"source_type": "s3",
				},
			},
		},
	})

	// 3. Create a registered model
	postJSON(t, baseURL+"/api/2.0/mlflow/registered-models/create", map[string]any{
		"name":        "fraud-detector",
		"description": "Fraud detection model",
	})

	// 4. Create a new model version assigned to the run_id AND with a correct source.
	artifactSource := fmt.Sprintf("mlflow-artifacts:/0/%s/artifacts", runID)

	postJSON(t, baseURL+"/api/2.0/mlflow/model-versions/create", map[string]any{
		"name":   "fraud-detector",
		"run_id": runID,
		"source": artifactSource,
	})

	return runID
}

// postJSON is a simple helper method to perform POST requests to the MLflow API
func postJSON(t *testing.T, url string, payload map[string]any) map[string]any {
	b, err := json.Marshal(payload)
	require.NoError(t, err)

	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	require.NoError(t, err, "error during POST request to: %s", url)
	defer resp.Body.Close()

	require.True(t, resp.StatusCode >= 200 && resp.StatusCode < 300,
		"unexpected HTTP status: %s (%d) for %s", resp.Status, resp.StatusCode, url)

	var res map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&res))
	return res
}
