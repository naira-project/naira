// depl_uses_litellm scans k8s Deployments for LiteLLM API keys stored in
// referenced Secrets, then queries each configured LiteLLM host to discover
// which models that key can access, and emits deployment→model "uses_model"
// relations.
//
// # Known Issues
//
// Currently, the plugin emits Nodes for all Deployments with at least one
// secret with value matching the configured API key regexp
// (DEPL_USES_LITELLM_APIKEY_REGEXP), even if the key is not verified to be
// found in any of the configured LiteLLM hosts. This is intended to help
// detect potentially missing LiteLLM host configurations. However, it can
// accidentally leak some information about Deployments having secrets with
// values accidentally matching the regexp.
// TODO(security): add option to emit only verified deployments, to avoid leaking parts of accidentally matched secrets.
//
// Currently, for env keys referenced through ".valueFrom.secretKeyRef.name",
// all values in the referenced secret are scanned, instead of only scanning
// the specific named env variable.
//
// # Environment Variables
//
//   - DEPL_USES_LITELLM_NAMED_HOSTS - MANDATORY - comma-separated list of
//     named LiteLLM base URLs, e.g.:
//     "host1=https://litellm.example.com,host2=http://litellm2.example.com:1234/base/"
//
//   - DEPL_USES_LITELLM_KUBECONFIG (optional) - path to kubeconfig file; if
//     unset, in-cluster config is used.
//
//   - DEPL_USES_LITELLM_APIKEY_REGEXP (optional) - custom regexp to match API
//     keys; defaults to: "^sk-.{22}$" (LiteLLM API keys format as of May 2026)
//
//   - DEPL_USES_LITELLM_HTTP_TIMEOUT (optional) - HTTP request timeout for
//     LiteLLM API calls; defaults to: 5s.
//
//go:generate bash -c "goreadme -use-stdlib-markdown -title 'depl_uses_litellm plugin' | sed 's/ {#hdr-[^}]*}//g' > README.md"
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/naira-project/naira/plugins/internal/kubeutil"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/naira-project/naira/plugins/pkg/pluginmain"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const pluginName = "depl_uses_litellm"

type config struct {
	Kubeconfig   string        `env:"DEPL_USES_LITELLM_KUBECONFIG"`
	NamedHosts   []namedHost   `env:"DEPL_USES_LITELLM_NAMED_HOSTS" usage:"comma-separated list of named LiteLLM base URLs, e.g. 'host1=https://litellm.example.com,host2=http://litellm2.example.com:1234/base/'"`
	APIKeyRegexp string        `env:"DEPL_USES_LITELLM_APIKEY_REGEXP" default:"^sk-.{22}$"` // optional custom regexp to match API keys; defaults to current (May 2026) LiteLLM format
	HTTPTimeout  time.Duration `env:"DEPL_USES_LITELLM_HTTP_TIMEOUT" default:"5s"`
}

func main() {
	app := pluginmain.New[config]()
	p, err := New(app.PluginConfig)
	if err != nil {
		log.Fatalf("failed to initialize plugin: %v", err)
	}
	app.Serve(p)
}

type Plugin struct {
	httpClient   *http.Client
	config       config
	apiKeyRegexp *regexp.Regexp
}

func New(config config) (*Plugin, error) {
	re, err := regexp.Compile(config.APIKeyRegexp)
	if err != nil {
		return nil, fmt.Errorf("invalid DEPL_USES_LITELLM_APIKEY_REGEXP: %w", err)
	}
	if len(config.NamedHosts) == 0 {
		return nil, fmt.Errorf("no LiteLLM hosts configured: DEPL_USES_LITELLM_NAMED_HOSTS is empty")
	}
	return &Plugin{
		httpClient:   &http.Client{Timeout: config.HTTPTimeout},
		config:       config,
		apiKeyRegexp: re,
	}, nil
}

func (p *Plugin) Collect(ctx context.Context) (pluginapi.CollectResponse, error) {
	dyn, err := p.connect()
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("connecting to cluster: %w", err)
	}

	namespaces, clusterID, err := kubeutil.NamespacesAndClusterID(ctx, dyn)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("listing namespaces and cluster ID: %w", err)
	}

	deplsWithAPIKeys, err := findDeploymentsWithMatchingSecrets(ctx, dyn, namespaces, p.apiKeyRegexp)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("scanning deployments: %w", err)
	}

	// Query each unique API key against every host once, cache results.
	keyToHostsToModels := make(map[string]map[string][]string) // apiKey -> hostName -> []modelID
	for _, d := range deplsWithAPIKeys {
		if _, seen := keyToHostsToModels[d.secret]; seen {
			continue
		}
		hostToModels := make(map[string][]string)
		for _, nh := range p.config.NamedHosts {
			models, err := fetchModels(ctx, p.httpClient, nh.baseURL, d.secret)
			if err != nil {
				log.Printf("%s: WARN: fetching models from %s=%s (key ...%s): %v",
					pluginName, nh.name, nh.baseURL, d.secret[len(d.secret)-4:], err)
				continue
			}
			if len(models) > 0 {
				hostToModels[nh.name] = models
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

	for _, d := range deplsWithAPIKeys {
		deplID := pluginapi.NodeID{
			Kind: pluginapi.NodeKindDeployment,
			Path: clusterID + "/" + d.namespace + "/" + d.deployment,
		}
		deplsByPath[deplID.Path] = pluginapi.NodeClaim{ID: deplID}

		for name, models := range keyToHostsToModels[d.secret] {
			for _, m := range models {
				modelID := pluginapi.NodeID{
					Kind: pluginapi.NodeKindModel,
					Path: "litellm/" + name + "/" + m,
				}
				modelsByPath[modelID.Path] = pluginapi.NodeClaim{ID: modelID}

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

	return pluginapi.CollectResponse{Nodes: nodes, Relations: relations}, nil
}

type namedHost struct {
	name    string
	baseURL string
}

func (nh *namedHost) UnmarshalText(text []byte) error {
	name, baseURL, ok := bytes.Cut(text, []byte("="))
	name = bytes.TrimSpace(name)
	baseURL = bytes.TrimSpace(baseURL)
	if !ok || len(name) == 0 || len(baseURL) == 0 {
		return fmt.Errorf(`invalid DEPL_USES_LITELLM_NAMED_HOSTS entry %q: expected "name=baseURL" format`, string(text))
	}
	if bytes.Contains(name, []byte("/")) {
		// We'll use the name as a path segment in the NodeID, so it cannot contain slashes.
		return fmt.Errorf(`invalid DEPL_USES_LITELLM_NAMED_HOSTS entry %q: name cannot contain "/"`, string(text))
	}
	nh.name = string(name)
	nh.baseURL = string(bytes.TrimRight(baseURL, "/"))
	return nil
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

// findDeploymentsWithMatchingSecrets scans all Deployments in the given namespaces, returning a list of those that
// use Secrets with values matching given pattern.
func findDeploymentsWithMatchingSecrets(ctx context.Context, dyn dynamic.Interface, namespaces []string, pattern *regexp.Regexp) ([]deploymentWithSecret, error) {
	var result []deploymentWithSecret
	type nameWithNs struct{ name, ns string }
	secretsCache := make(map[nameWithNs]*unstructured.Unstructured)
	for _, ns := range namespaces {
		depls, err := dyn.Resource(gvrDeployments).Namespace(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			log.Printf("%s: WARN: listing deployments in namespace %q: %v", pluginName, ns, err)
			continue
		}
		for _, depl := range depls.Items {
			ns, name := depl.GetNamespace(), depl.GetName()
			seenSecretValues := make(map[string]struct{})

			for secretName := range referencedSecretsNames(depl.Object) {
				nameWithNs := nameWithNs{name: secretName, ns: ns}
				secret := secretsCache[nameWithNs]
				if secret == nil {
					secret, err = dyn.Resource(gvrSecrets).Namespace(ns).Get(ctx, secretName, metav1.GetOptions{})
					if err != nil {
						log.Printf("%s: WARN: %s/%s: cannot read secret %q: %v",
							pluginName, ns, name, secretName, err)
						continue
					}
					secretsCache[nameWithNs] = secret
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
						if _, seen := seenSecretValues[val]; seen {
							continue
						}
						seenSecretValues[val] = struct{}{}
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

// fetchModels calls GET <baseURL>/v1/models with the given key and returns a list of model IDs.
// A 401/403 response means the key is not valid for that host and empty results are returned.
func fetchModels(ctx context.Context, client *http.Client, baseURL, apiKey string) ([]string, error) {
	addr := baseURL + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr, nil)
	if err != nil {
		return nil, fmt.Errorf("preparing %q request: %w", addr, err)
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
