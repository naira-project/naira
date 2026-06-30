// Similar test to one in litellm/main_test.go.
// One difference it do not uses real k8s, but still uses real postgres and litellm
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	pluginapi "github.com/naira-project/naira/plugins/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

const (
	litellmMasterKey2 = "sk-master-key-12345" // Mock master key for testing API setup
	testModelName2    = "gpt-4"               // Name of the model configured in LiteLLM mock config
)

func TestPlugin2_Collect_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// 1. Setup Docker Network (allows LiteLLM to connect directly to Postgres)
	net, err := network.New(ctx, network.WithCheckDuplicate())
	require.NoError(t, err, "failed to create docker network")
	defer net.Remove(ctx)

	// 2. Start PostgreSQL Container
	t.Log("Starting PostgreSQL container...")
	setupPostgresContainer2(ctx, t, net.Name)

	// 3. Start LiteLLM Container (connected to Postgres)
	t.Log("Starting LiteLLM container...")
	litellmURL, cleanupLiteLLM := setupLiteLLMContainer2(ctx, t, net.Name)
	defer cleanupLiteLLM()

	// 4. Seed Data in LiteLLM: Create a virtual key that allows access to 'gpt-4'
	t.Log("Seeding LiteLLM data (generating virtual key)...")
	virtualKey := seedLiteLLMKey2(t, litellmURL)

	// 5. Build a fake Kubernetes client pre-populated with an AppIdentity object.
	//    This replaces the real k3s cluster — no container needed.
	t.Log("Building fake k8s client with AppIdentity...")
	fakeK8sClient := buildFakeK8sClient(virtualKey)

	// --- SETUP COMPLETE, STARTING PLUGIN LOGIC ---

	cfg := config{
		BaseURL:     litellmURL,
		APIKey:      litellmMasterKey2, // Master key is used to authenticate to /v1/models
		HTTPTimeout: 10 * time.Second,
	}

	plugin := &Plugin{
		httpClient:          http.DefaultClient,
		config:              cfg,
		appIdentityProvider: NewKubernetesAppIdentityProvider(fakeK8sClient),
	}

	// Execute Collect
	t.Log("Executing plugin.Collect()...")
	resp, err := plugin.Collect(ctx)
	require.NoError(t, err, "expected Collect function to succeed")

	// --- ASSERTIONS ---

	expectedNodes := []pluginapi.NodeClaim{
		{
			ID: pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: "litellm/" + testModelName2},
			Properties: pluginapi.PropertyMap{
				propertyKeyOwnedBy: "openai",
			},
		},
		{
			ID: pluginapi.NodeID{Kind: pluginapi.NodeKindApplication, Path: "litellm/default/fraud-detector-agent"},
			Properties: pluginapi.PropertyMap{
				propertyKeyNamespace:         "default",
				propertyKeyTeam:              "risk",
				propertyKeyLiteLLMVirtualKey: virtualKey,
			},
		},
	}

	expectedRelations := []pluginapi.RelationClaim{
		{
			Kind: pluginapi.RelationKindUsesModel,
			From: pluginapi.NodeID{Kind: pluginapi.NodeKindApplication, Path: "litellm/default/fraud-detector-agent"},
			To:   pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: "litellm/" + testModelName2},
			Properties: pluginapi.PropertyMap{
				propertyKeyLiteLLMVirtualKey: virtualKey,
			},
		},
	}

	assert.ElementsMatch(t, expectedNodes, resp.Nodes, "Nodes structure does not match expectations")
	assert.ElementsMatch(t, expectedRelations, resp.Relations, "Relations structure does not match expectations")
}

// buildFakeK8sClient returns a fake dynamic client pre-seeded with a single AppIdentity
// object that carries the given LiteLLM virtual key. It does not require any running cluster.
func buildFakeK8sClient(virtualKey string) *dynamicfake.FakeDynamicClient {
	// The fake client needs the list kind registered in the scheme so that
	// List() calls on the tracker work correctly.
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "naira.io", Version: "v1alpha1", Kind: "AppIdentity"},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "naira.io", Version: "v1alpha1", Kind: "AppIdentityList"},
		&unstructured.UnstructuredList{},
	)

	appIdentity := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "naira.io/v1alpha1",
			"kind":       "AppIdentity",
			"metadata": map[string]any{
				"name":      "fraud-detector-agent",
				"namespace": "default",
			},
			"spec": map[string]any{
				"team":              "risk",
				"litellmVirtualKey": virtualKey,
			},
		},
	}

	return dynamicfake.NewSimpleDynamicClient(scheme, appIdentity)
}

// setupPostgresContainer2 starts a Postgres database required by LiteLLM to store keys.
func setupPostgresContainer2(ctx context.Context, t *testing.T, networkName string) {
	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_DB":       "litellm",
			"POSTGRES_USER":     "litellm",
			"POSTGRES_PASSWORD": "litellm-local-password",
		},
		Networks: []string{networkName},
		NetworkAliases: map[string][]string{
			networkName: {"postgres"}, // Allows LiteLLM to resolve 'postgres:5432'
		},
		WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(1 * time.Minute),
	}

	_, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start Postgres container")
}

// setupLiteLLMContainer2 starts the LiteLLM proxy and connects it to Postgres.
func setupLiteLLMContainer2(ctx context.Context, t *testing.T, networkName string) (string, func()) {
	liteLLMConfig := `
model_list:
  - model_name: ` + testModelName2 + `
    litellm_params:
      model: openai/gpt-4
      api_key: sk-mock-openai-key
`
	req := testcontainers.ContainerRequest{
		Image:        "ghcr.io/berriai/litellm:main-latest",
		ExposedPorts: []string{"4000/tcp"},
		Env: map[string]string{
			"DATABASE_URL":       "postgresql://litellm:litellm-local-password@postgres:5432/litellm",
			"LITELLM_MASTER_KEY": litellmMasterKey2,
			"STORE_MODEL_IN_DB":  "True",
		},
		Networks: []string{networkName},
		Files: []testcontainers.ContainerFile{
			{
				HostFilePath:      "",
				ContainerFilePath: "/app/config.yaml",
				FileMode:          0644,
				Reader:            strings.NewReader(liteLLMConfig),
			},
		},
		Cmd:        []string{"--config", "/app/config.yaml"},
		WaitingFor: wait.ForHTTP("/health/readiness").WithPort("4000/tcp").WithStartupTimeout(2 * time.Minute),
	}

	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start LiteLLM container")

	ip, err := c.Host(ctx)
	require.NoError(t, err)

	port, err := c.MappedPort(ctx, "4000")
	require.NoError(t, err)

	baseURL := fmt.Sprintf("http://%s:%s", ip, port.Port())

	cleanup := func() {
		require.NoError(t, c.Terminate(ctx))
	}

	return baseURL, cleanup
}

// seedLiteLLMKey2 creates a new virtual key in LiteLLM allowing access to gpt-4.
func seedLiteLLMKey2(t *testing.T, baseURL string) string {
	payload := map[string]any{
		"models": []string{testModelName2},
	}
	b, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/key/generate", bytes.NewReader(b))
	require.NoError(t, err)

	req.Header.Set("Authorization", "Bearer "+litellmMasterKey2)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "POST /key/generate failed")
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var res struct {
		Key string `json:"key"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&res))
	require.NotEmpty(t, res.Key, "LiteLLM virtual key should not be empty")

	return res.Key
}
