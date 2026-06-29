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
	"github.com/testcontainers/testcontainers-go/modules/k3s"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	litellmMasterKey = "sk-master-key-12345" // Mock master key for testing API setup
	testModelName    = "gpt-4"               // Name of the model configured in LiteLLM mock config
)

func TestPlugin_Collect_Integration(t *testing.T) {
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
	setupPostgresContainer(ctx, t, net.Name)

	// 3. Start LiteLLM Container (connected to Postgres)
	t.Log("Starting LiteLLM container...")
	litellmURL, cleanupLiteLLM := setupLiteLLMContainer(ctx, t, net.Name)
	defer cleanupLiteLLM()

	// 4. Seed Data in LiteLLM: Create a virtual key that allows access to 'gpt-4'
	t.Log("Seeding LiteLLM data (generating virtuall key)...")
	virtualKey := seedLiteLLMKey(t, litellmURL)

	// 5. Start K3s (Lightweight Kubernetes) Container
	t.Log("Starting K3s container...")
	k3sClient, k3sContainer, cleanupK3s := setupK3sContainer(ctx, t)
	defer cleanupK3s()

	// 6. Seed Data in K8s: Apply CRD and AppIdentity manifest with the generated virtual key
	t.Log("Applying CRD and AppIdentity manifests to K3s...")
	applyK8sManifests(ctx, t, k3sContainer, virtualKey)

	// --- SETUP COMPLETE, STARTING PLUGIN LOGIC ---

	cfg := config{
		BaseURL:     litellmURL,
		APIKey:      litellmMasterKey, // Master key is used to authenticate to /v1/models
		HTTPTimeout: 10 * time.Second,
	}

	plugin := &Plugin{
		httpClient: http.DefaultClient,
		config:     cfg,
		// Injecting the dynamic client pointing to our Testcontainers K3s instance
		appIdentityProvider: NewKubernetesAppIdentityProvider(k3sClient),
	}

	// Execute Collect
	t.Log("Executing plugin.Collect()...")
	resp, err := plugin.Collect(ctx)
	require.NoError(t, err, "expected Collect function to succeed")

	// --- ASSERTIONS ---

	expectedNodes := []pluginapi.NodeClaim{
		{
			ID: pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: "litellm/" + testModelName},
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
			To:   pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: "litellm/" + testModelName},
			Properties: pluginapi.PropertyMap{
				propertyKeyLiteLLMVirtualKey: virtualKey,
			},
		},
	}

	// Use ElementsMatch for clean, readable output if slices differ
	assert.ElementsMatch(t, expectedNodes, resp.Nodes, "Nodes structure does not match expectations")
	assert.ElementsMatch(t, expectedRelations, resp.Relations, "Relations structure does not match expectations")
}

// setupPostgresContainer starts a Postgres database required by LiteLLM to store keys
func setupPostgresContainer(ctx context.Context, t *testing.T, networkName string) {
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

// setupLiteLLMContainer starts the LiteLLM proxy and connects it to Postgres
func setupLiteLLMContainer(ctx context.Context, t *testing.T, networkName string) (string, func()) {
	// A minimal LiteLLM config that registers a mock model (gpt-4)
	liteLLMConfig := `
model_list:
  - model_name: ` + testModelName + `
    litellm_params:
      model: openai/gpt-4
      api_key: sk-mock-openai-key
`
	req := testcontainers.ContainerRequest{
		Image:        "ghcr.io/berriai/litellm:main-latest",
		ExposedPorts: []string{"4000/tcp"},
		Env: map[string]string{
			"DATABASE_URL":       "postgresql://litellm:litellm-local-password@postgres:5432/litellm",
			"LITELLM_MASTER_KEY": litellmMasterKey,
			"STORE_MODEL_IN_DB":  "True", // Force LiteLLM to use Postgres
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
		Cmd: []string{"--config", "/app/config.yaml"},
		// Wait until the health check endpoint returns 200
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

	cleanupFunc := func() {
		require.NoError(t, c.Terminate(ctx))
	}

	return baseURL, cleanupFunc
}

// setupK3sContainer starts a lightweight Kubernetes cluster and returns a dynamic client
func setupK3sContainer(ctx context.Context, t *testing.T) (dynamic.Interface, *k3s.K3sContainer, func()) {
	k3sC, err := k3s.Run(ctx, "rancher/k3s:v1.28.2-k3s1")
	require.NoError(t, err, "failed to start K3s container")

	kubeConfigBytes, err := k3sC.GetKubeConfig(ctx)
	require.NoError(t, err)

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeConfigBytes)
	require.NoError(t, err)

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	require.NoError(t, err)

	cleanupFunc := func() {
		require.NoError(t, k3sC.Terminate(ctx))
	}

	return dynamicClient, k3sC, cleanupFunc
}

// seedLiteLLMKey creates a new virtual key in LiteLLM allowing access to gpt-4
func seedLiteLLMKey(t *testing.T, baseURL string) string {
	payload := map[string]any{
		"models": []string{testModelName},
	}
	b, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/key/generate", bytes.NewReader(b))
	require.NoError(t, err)

	req.Header.Set("Authorization", "Bearer "+litellmMasterKey)
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

// applyK8sManifests copies CRD and AppIdentity YAML to the K3s container and executes kubectl apply
func applyK8sManifests(ctx context.Context, t *testing.T, k3sC *k3s.K3sContainer, virtualKey string) {
	// 1. Apply CRD
	crdManifest := `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: appidentities.naira.io
spec:
  group: naira.io
  names:
    kind: AppIdentity
    plural: appidentities
    singular: appidentity
  scope: Namespaced
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                team:
                  type: string
                litellmVirtualKey:
                  type: string
`
	err := k3sC.CopyToContainer(ctx, []byte(crdManifest), "/tmp/crd.yaml", 0644)
	require.NoError(t, err)

	exitCode, _, err := k3sC.Exec(ctx, []string{"kubectl", "apply", "-f", "/tmp/crd.yaml"})
	require.NoError(t, err)
	require.Equal(t, 0, exitCode)

	// Wait briefly for K8s API to register the CRD properly
	time.Sleep(3 * time.Second)

	// 2. Apply Custom Resource AppIdentity with the dynamically generated key
	appManifest := fmt.Sprintf(`
apiVersion: naira.io/v1alpha1
kind: AppIdentity
metadata:
  name: fraud-detector-agent
  namespace: default
spec:
  team: risk
  litellmVirtualKey: %s
`, virtualKey)

	err = k3sC.CopyToContainer(ctx, []byte(appManifest), "/tmp/app.yaml", 0644)
	require.NoError(t, err)

	exitCode, _, err = k3sC.Exec(ctx, []string{"kubectl", "apply", "-f", "/tmp/app.yaml"})
	require.NoError(t, err)
	require.Equal(t, 0, exitCode)
}
