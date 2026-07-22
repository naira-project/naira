package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/naira-project/naira/plugins/internal/kubeutil"
	"github.com/naira-project/naira/plugins/internal/openaicompat"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/naira-project/naira/plugins/pkg/pluginmain"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const pluginName = "depl_uses_thalamus"

type config struct {
	Kubeconfig  string        `env:"DEPL_USES_THALAMUS_KUBECONFIG"`
	BaseURL     string        `env:"DEPL_USES_THALAMUS_BASE_URL"`
	BearerToken string        `env:"DEPL_USES_THALAMUS_BEARER_TOKEN"`
	HTTPTimeout time.Duration `env:"DEPL_USES_THALAMUS_HTTP_TIMEOUT" default:"5s"`
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
	httpClient *http.Client
	config     config
}

func New(cfg config) (*Plugin, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("DEPL_USES_THALAMUS_BASE_URL is required")
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &Plugin{
		httpClient: &http.Client{Timeout: cfg.HTTPTimeout},
		config:     cfg,
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

	models, err := openaicompat.FetchModels(ctx, p.httpClient, p.config.BaseURL, p.config.BearerToken)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("fetching thalamus models: %w", err)
	}
	modelIDs := make([]string, 0, len(models))
	for _, m := range models {
		modelIDs = append(modelIDs, m.ID)
	}

	type fromTo struct{ from, to pluginapi.NodeID }
	var (
		deplsByPath   = make(map[string]pluginapi.NodeClaim)
		modelsByPath  = make(map[string]pluginapi.NodeClaim)
		relations     []pluginapi.RelationClaim
		seenRelations = make(map[fromTo]struct{})
	)

	for _, ns := range namespaces {
		depls, err := dyn.Resource(gvrDeployments).Namespace(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			log.Printf("%s: WARN: listing deployments in namespace %q: %v", pluginName, ns, err)
			continue
		}

		for _, depl := range depls.Items {
			if !deploymentReferencesBaseURL(depl.Object, p.config.BaseURL) {
				continue
			}

			deplID := pluginapi.NodeID{
				Kind: pluginapi.NodeKindDeployment,
				Path: clusterID + "/" + depl.GetNamespace() + "/" + depl.GetName(),
			}
			deplsByPath[deplID.Path] = pluginapi.NodeClaim{ID: deplID}

			for _, modelID := range modelIDs {
				mID := pluginapi.NodeID{
					Kind: pluginapi.NodeKindModel,
					Path: "thalamus/" + modelID,
				}
				modelsByPath[mID.Path] = pluginapi.NodeClaim{ID: mID}

				ft := fromTo{from: deplID, to: mID}
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

// deploymentReferencesBaseURL returns true if any plaintext *_BASE_URL or *_API_BASE
// env var value in the deployment's pod template starts with baseURL.
func deploymentReferencesBaseURL(obj map[string]any, baseURL string) bool {
	for _, section := range []string{"containers", "initContainers"} {
		containers, _, _ := unstructured.NestedSlice(obj, "spec", "template", "spec", section)
		for _, c := range containers {
			container, ok := c.(map[string]any)
			if !ok {
				continue
			}
			envList, _, _ := unstructured.NestedSlice(container, "env")
			for _, e := range envList {
				env, ok := e.(map[string]any)
				if !ok {
					continue
				}
				name, _, _ := unstructured.NestedString(env, "name")
				value, _, _ := unstructured.NestedString(env, "value")
				upper := strings.ToUpper(name)
				if (strings.HasSuffix(upper, "_BASE_URL") || strings.HasSuffix(upper, "_API_BASE")) &&
					strings.HasPrefix(value, baseURL) {
					return true
				}
			}
		}
	}
	return false
}

var gvrDeployments = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

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
