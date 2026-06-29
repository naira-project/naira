// Package depl_uses_litellm scans k8s Deployments for LiteLLM API keys stored
// in referenced Secrets, then queries each configured LiteLLM host to discover
// which models that key can access, and emits deployment→model "uses_model"
// relations.
package depl_uses_litellm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/naira-project/naira/catalog/internal/kubeutil"
	"github.com/naira-project/naira/catalog/pluginapi"
)

const pluginName = "depl_uses_litellm"

type Config struct {
	Enabled      bool     `env:"ENABLED" default:"true"`
	Kubeconfig   string   `env:"KUBECONFIG"`
	Hosts        []string `env:"HOSTS"`                               // bare hostnames; "https://" is prepended automatically
	APIKeyRegexp string   `env:"API_KEY_REGEXP" default:"^sk-.{22}$"` // optional custom regexp to match API keys; defaults to current (May 2026) LiteLLM format
}

type Plugin struct {
	httpClient   *http.Client
	config       Config
	apiKeyRegexp *regexp.Regexp
}

func New(httpClient *http.Client, config Config) (*Plugin, error) {
	re, err := regexp.Compile(config.APIKeyRegexp)
	if err != nil {
		return nil, fmt.Errorf("invalid API_KEY_REGEXP: %w", err)
	}
	return &Plugin{
		httpClient:   httpClient,
		config:       config,
		apiKeyRegexp: re,
	}, nil
}

func (*Plugin) Name() string { return pluginName }

func (p *Plugin) Collect(ctx context.Context) (pluginapi.IngestionRequest, error) {
	dyn, err := p.connect()
	if err != nil {
		return pluginapi.IngestionRequest{}, fmt.Errorf("connecting to cluster: %w", err)
	}

	namespaces, clusterID, err := kubeutil.NamespacesAndClusterID(ctx, dyn)
	if err != nil {
		return pluginapi.IngestionRequest{}, fmt.Errorf("listing namespaces and cluster ID: %w", err)
	}

	findings, err := scanDeployments(ctx, dyn, namespaces, p.apiKeyRegexp)
	if err != nil {
		return pluginapi.IngestionRequest{}, fmt.Errorf("scanning deployments: %w", err)
	}

	// Query each unique API key against every host once, cache results.
	// map[apiKey]map[host][]modelID
	keyModels := make(map[string]map[string][]string)
	for _, f := range findings {
		if _, seen := keyModels[f.apiKey]; seen {
			continue
		}
		hostMap := make(map[string][]string)
		for _, host := range p.config.Hosts {
			models, err := fetchModels(p.httpClient, host, f.apiKey)
			if err != nil {
				// non-fatal: warn and skip this host
				fmt.Fprintf(os.Stderr, "warning: depl-litellm-models: %s (key ...%s): %v\n",
					host, f.apiKey[len(f.apiKey)-4:], err)
				continue
			}
			if len(models) > 0 {
				hostMap[host] = models
			}
		}
		keyModels[f.apiKey] = hostMap
	}

	// Build node+relation claims.
	// Deployment nodes are keyed by "namespace/name".
	// Model nodes are keyed by "host/modelID".
	deployNodes := make(map[string]pluginapi.NodeClaim)
	modelNodes := make(map[string]pluginapi.NodeClaim)
	var relations []pluginapi.RelationClaim
	type relKey struct{ from, to pluginapi.NodeID }
	seenRelations := make(map[relKey]struct{})

	for _, f := range findings {
		deplPath := clusterID + "/" + f.namespace + "/" + f.deployment
		if _, ok := deployNodes[deplPath]; !ok {
			deployNodes[deplPath] = pluginapi.NodeClaim{
				ID: pluginapi.NodeID{Kind: pluginapi.NodeKindDeployment, Path: deplPath},
				Properties: pluginapi.PropertyMap{
					"namespace": f.namespace,
					"name":      f.deployment,
				},
			}
		}
		deplID := deployNodes[deplPath].ID

		for host, models := range keyModels[f.apiKey] {
			for _, modelID := range models {
				modelPath := pluginName + "/" + host + "/" + modelID
				if _, ok := modelNodes[modelPath]; !ok {
					modelNodes[modelPath] = pluginapi.NodeClaim{
						ID: pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: modelPath},
						Properties: pluginapi.PropertyMap{
							"host": host,
							"id":   modelID,
						},
					}
				}
				modID := modelNodes[modelPath].ID

				rk := relKey{deplID, modID}
				if _, seen := seenRelations[rk]; seen {
					continue
				}
				seenRelations[rk] = struct{}{}
				relations = append(relations, pluginapi.RelationClaim{
					Kind: pluginapi.RelationKindUsesModel,
					From: deplID,
					To:   modID,
				})
			}
		}
	}

	nodes := make([]pluginapi.NodeClaim, 0, len(deployNodes)+len(modelNodes))
	for _, n := range deployNodes {
		nodes = append(nodes, n)
	}
	for _, n := range modelNodes {
		nodes = append(nodes, n)
	}

	return pluginapi.IngestionRequest{Nodes: nodes, Relations: relations}, nil
}

type finding struct {
	namespace  string
	deployment string
	apiKey     string
}

var (
	gvrDeployments = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	gvrSecrets     = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
)

func scanDeployments(ctx context.Context, dyn dynamic.Interface, namespaces []string, litellmKey *regexp.Regexp) ([]finding, error) {
	var findings []finding
	for _, namespace := range namespaces {
		depList, err := dyn.Resource(gvrDeployments).Namespace(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: listing deployments in namespace %q: %v\n", pluginName, namespace, err)
			continue
		}
		for _, dep := range depList.Items {
			depNs := dep.GetNamespace()
			depName := dep.GetName()
			seenKeys := make(map[string]struct{})

			for secretName := range referencedSecrets(dep.Object) {
				secret, err := dyn.Resource(gvrSecrets).Namespace(depNs).Get(ctx, secretName, metav1.GetOptions{})
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: %s: %s/%s: cannot read secret %q: %v\n",
						pluginName, depNs, depName, secretName, err)
					continue
				}

				dataMap, _, _ := unstructured.NestedStringMap(secret.Object, "data")
				for _, encoded := range dataMap {
					decoded, err := base64.StdEncoding.DecodeString(encoded)
					if err != nil {
						continue
					}
					val := strings.TrimSpace(string(decoded))
					if litellmKey.MatchString(val) {
						if _, seen := seenKeys[val]; !seen {
							seenKeys[val] = struct{}{}
							findings = append(findings, finding{
								namespace:  depNs,
								deployment: depName,
								apiKey:     val,
							})
						}
					}
				}
			}
		}
	}
	return findings, nil
}

// referencedSecrets returns names of every Secret referenced by the Deployment's pod spec.
func referencedSecrets(obj map[string]interface{}) map[string]struct{} {
	refs := make(map[string]struct{})

	volumes, _, _ := unstructured.NestedSlice(obj, "spec", "template", "spec", "volumes")
	for _, v := range volumes {
		vol, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		if name, ok, _ := unstructured.NestedString(vol, "secret", "secretName"); ok && name != "" {
			refs[name] = struct{}{}
		}
	}

	for _, section := range []string{"containers", "initContainers"} {
		containers, _, _ := unstructured.NestedSlice(obj, "spec", "template", "spec", section)
		for _, c := range containers {
			container, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			envList, _, _ := unstructured.NestedSlice(container, "env")
			for _, e := range envList {
				env, ok := e.(map[string]interface{})
				if !ok {
					continue
				}
				if name, ok, _ := unstructured.NestedString(env, "valueFrom", "secretKeyRef", "name"); ok && name != "" {
					refs[name] = struct{}{}
				}
			}
			envFromList, _, _ := unstructured.NestedSlice(container, "envFrom")
			for _, ef := range envFromList {
				envFrom, ok := ef.(map[string]interface{})
				if !ok {
					continue
				}
				if name, ok, _ := unstructured.NestedString(envFrom, "secretRef", "name"); ok && name != "" {
					refs[name] = struct{}{}
				}
			}
		}
	}
	return refs
}

type modelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// fetchModels calls GET https://<host>/v1/models with the given key.
// A 401/403 response means the key is not valid for that host; returns nil, nil.
func fetchModels(client *http.Client, host, apiKey string) ([]string, error) {
	addr := "https://" + host + "/v1/models"
	req, err := http.NewRequest(http.MethodGet, addr, nil)
	if err != nil {
		return nil, fmt.Errorf("preparing HTTPS request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing %q request: %w", addr, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		break // continue to read the body
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, nil
	default:
		return nil, fmt.Errorf("unexpected HTTP %d", resp.StatusCode)
	}

	var mr modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	models := make([]string, 0, len(mr.Data))
	for _, m := range mr.Data {
		models = append(models, m.ID)
	}
	return models, nil
}

func (p *Plugin) connect() (dynamic.Interface, error) {
	cfg, err := kubeutil.RestConfig(p.config.Kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("loading k8s config: %w", err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating k8s dynamic client: %w", err)
	}
	return dyn, nil
}
