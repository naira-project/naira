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
	"log"
	"net/http"
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

	deplsWithSecrets, err := scanDeploymentsWithMatchingSecrets(ctx, dyn, namespaces, p.apiKeyRegexp)
	if err != nil {
		return pluginapi.IngestionRequest{}, fmt.Errorf("scanning deployments: %w", err)
	}

	// Query each unique API key against every host once, cache results.
	keyToHostsToModels := make(map[string]map[string][]string) // apiKey -> host -> []modelID
	for _, d := range deplsWithSecrets {
		if _, seen := keyToHostsToModels[d.secret]; seen {
			continue
		}
		hostToModels := make(map[string][]string)
		for _, host := range p.config.Hosts {
			models, err := fetchModels(p.httpClient, host, d.secret)
			if err != nil {
				// non-fatal: warn and skip this host
				log.Printf("%s: WARN: %s (key ...%s): %v",
					pluginName, host, d.secret[len(d.secret)-4:], err)
				continue
			}
			if len(models) > 0 {
				hostToModels[host] = models
			}
		}
		keyToHostsToModels[d.secret] = hostToModels
	}

	// Build Node & Relation claims.
	type fromTo struct{ from, to pluginapi.NodeID }
	var (
		deplsByPath   = make(map[string]pluginapi.NodeClaim) // key: "clusterID/namespace/name"
		modelsByPath  = make(map[string]pluginapi.NodeClaim) // key: "host/modelID"
		relations     []pluginapi.RelationClaim
		seenRelations = make(map[fromTo]struct{})
	)

	for _, d := range deplsWithSecrets {
		deplID := pluginapi.NodeID{
			Kind: pluginapi.NodeKindDeployment,
			Path: clusterID + "/" + d.namespace + "/" + d.deployment,
		}
		deplsByPath[deplID.Path] = pluginapi.NodeClaim{ID: deplID}

		for host, models := range keyToHostsToModels[d.secret] {
			for _, m := range models {
				modelID := pluginapi.NodeID{
					Kind: pluginapi.NodeKindModel,
					// TODO: somehow resolve the hostname, it could be local
					Path: "litellm/" + host + "/" + m,
				}
				// TODO: deterministic behavior in case of multiple matches (e.g. when same instance of litellm has multiple hostnames)
				modelsByPath[modelID.Path] = pluginapi.NodeClaim{
					ID: modelID,
					Properties: pluginapi.PropertyMap{
						"host": host,
						"id":   m,
					},
				}

				ft := fromTo{from: deplID, to: modelID}
				if _, seen := seenRelations[ft]; seen {
					continue
				}
				seenRelations[ft] = struct{}{}
				relations = append(relations, pluginapi.RelationClaim{
					Kind: pluginapi.RelationKindUsesModel,
					From: ft.from,
					To:   ft.to,
				})
			}
		}
	}

	nodes := make([]pluginapi.NodeClaim, 0, len(deplsByPath)+len(modelsByPath))
	for _, n := range deplsByPath {
		nodes = append(nodes, n)
	}
	for _, n := range modelsByPath {
		nodes = append(nodes, n)
	}

	return pluginapi.IngestionRequest{Nodes: nodes, Relations: relations}, nil
}

type deploymentWithSecret struct {
	namespace  string
	deployment string
	secret     string
}

var (
	gvrDeployments = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	gvrSecrets     = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
)

// scanDeploymentsWithMatchingSecrets scans all Deployments in the given namespaces, returning a list of those that
// use Secrets with values matching given pattern.
func scanDeploymentsWithMatchingSecrets(ctx context.Context, dyn dynamic.Interface, namespaces []string, pattern *regexp.Regexp) ([]deploymentWithSecret, error) {
	var result []deploymentWithSecret
	for _, ns := range namespaces {
		depls, err := dyn.Resource(gvrDeployments).Namespace(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			log.Printf("%s: WARN: listing deployments in namespace %q: %v", pluginName, ns, err)
			continue
		}
		for _, depl := range depls.Items {
			ns, name := depl.GetNamespace(), depl.GetName()
			seenValues := make(map[string]struct{})

			for secretName := range referencedSecretsNames(depl.Object) {
				secret, err := dyn.Resource(gvrSecrets).Namespace(ns).Get(ctx, secretName, metav1.GetOptions{})
				if err != nil {
					log.Printf("%s: WARN: %s/%s: cannot read secret %q: %v",
						pluginName, ns, name, secretName, err)
					continue
				}

				dataMap, _, _ := unstructured.NestedStringMap(secret.Object, "data")
				for _, encoded := range dataMap {
					decoded, err := base64.StdEncoding.DecodeString(encoded)
					if err != nil {
						log.Printf("%s: WARN: %s/%s: secret %q: cannot base64-decode", pluginName, ns, name, secretName)
						continue
					}
					val := strings.TrimSpace(string(decoded))
					if pattern.MatchString(val) {
						if _, seen := seenValues[val]; seen {
							continue
						}
						seenValues[val] = struct{}{}
						result = append(result, deploymentWithSecret{
							namespace:  ns,
							deployment: name,
							secret:     val,
						})
					}
				}
			}
		}
	}
	return result, nil
}

// referencedSecretsNames returns names of every Secret referenced by the Deployment's pod spec.
func referencedSecretsNames(obj map[string]any) map[string]struct{} {
	names := make(map[string]struct{})

	volumes, _, _ := unstructured.NestedSlice(obj, "spec", "template", "spec", "volumes")
	for _, v := range volumes {
		v, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if name, ok, _ := unstructured.NestedString(v, "secret", "secretName"); ok && name != "" {
			names[name] = struct{}{}
		}
	}

	for _, section := range []string{"containers", "initContainers"} {
		containers, _, _ := unstructured.NestedSlice(obj, "spec", "template", "spec", section)
		for _, c := range containers {
			container, ok := c.(map[string]any)
			if !ok {
				continue
			}
			envs, _, _ := unstructured.NestedSlice(container, "env")
			for _, e := range envs {
				env, ok := e.(map[string]any)
				if !ok {
					continue
				}
				if name, ok, _ := unstructured.NestedString(env, "valueFrom", "secretKeyRef", "name"); ok && name != "" {
					names[name] = struct{}{}
				}
			}
			envFroms, _, _ := unstructured.NestedSlice(container, "envFrom")
			for _, ef := range envFroms {
				envFrom, ok := ef.(map[string]any)
				if !ok {
					continue
				}
				if name, ok, _ := unstructured.NestedString(envFrom, "secretRef", "name"); ok && name != "" {
					names[name] = struct{}{}
				}
			}
		}
	}
	return names
}

// fetchModels calls GET https://<host>/v1/models with the given key and returns a list of model IDs.
// A 401/403 response means the key is not valid for that host and empty results are returned.
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
		return nil, fmt.Errorf("%q: unexpected HTTP status %d", addr, resp.StatusCode)
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("parsing %q response: %w", addr, err)
	}
	models := make([]string, 0, len(payload.Data))
	for _, m := range payload.Data {
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
