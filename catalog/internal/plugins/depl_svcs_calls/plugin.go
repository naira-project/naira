// Package deplsvcscalls implements a plugin scanning k8s Deployments and
// Services, then claiming "calls" relation from each Deployment that mentions
// a Service's name in any plaintext Env value.
package deplsvcscalls

import (
	"context"
	"fmt"
	"regexp"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/naira-project/naira/catalog/internal/kubeconn"
	"github.com/naira-project/naira/catalog/pluginapi"
)

const pluginName = "depl-svcs-calls"

var (
	gvrDeployments = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	gvrServices    = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}
)

type Config struct {
	Enabled    bool
	Kubeconfig string // explicit path; empty = auto-detect
	Namespace  string // empty = all namespaces
}

type Plugin struct {
	config Config
}

func New(config Config) *Plugin {
	return &Plugin{config: config}
}

func (*Plugin) Name() string { return pluginName }

func (p *Plugin) Collect(ctx context.Context) (pluginapi.IngestionRequest, error) {
	dyn, err := p.connect()
	if err != nil {
		return pluginapi.IngestionRequest{}, fmt.Errorf("connecting to cluster: %w", err)
	}

	ns := p.config.Namespace

	svcList, err := dyn.Resource(gvrServices).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return pluginapi.IngestionRequest{}, fmt.Errorf("listing services: %w", err)
	}

	depList, err := dyn.Resource(gvrDeployments).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return pluginapi.IngestionRequest{}, fmt.Errorf("listing deployments: %w", err)
	}

	var nodes []pluginapi.NodeClaim
	var relations []pluginapi.RelationClaim

	// Build per-namespace service index and precompile patterns.
	//
	// TODO(akavel-reply): configurable filter (blocklist)
	// TODO(akavel-reply): configurable list of extra names
	// TODO(akavel-reply): feature for loading service names from Naira
	svcsByNs := map[string]map[string]pluginapi.NodeID{} // ns → name → NodeID
	patterns := map[string]*regexp.Regexp{}              // name → pattern

	for _, svc := range svcList.Items {
		svcNs := svc.GetNamespace()
		svcName := svc.GetName()
		id := pluginapi.NodeID{Kind: pluginapi.NodeKindService, Path: svcNs + "/" + svcName}
		nodes = append(nodes, pluginapi.NodeClaim{
			ID: id,
			Properties: pluginapi.PropertyMap{
				"namespace": svcNs,
				"name":      svcName,
			},
		})
		if svcsByNs[svcNs] == nil {
			svcsByNs[svcNs] = make(map[string]pluginapi.NodeID)
		}
		svcsByNs[svcNs][svcName] = id
		if _, ok := patterns[svcName]; !ok {
			patterns[svcName] = regexp.MustCompile(`\b` + regexp.QuoteMeta(svcName) + `\b`)
		}
	}

	// Scan Deployments' Env values, find references to Service names collected
	// above, interpret them as calls from the Deployment to the Service.
	for _, dep := range depList.Items {
		depNs := dep.GetNamespace()
		depName := dep.GetName()
		depID := pluginapi.NodeID{Kind: pluginapi.NodeKindDeployment, Path: depNs + "/" + depName}
		nodes = append(nodes, pluginapi.NodeClaim{
			ID: depID,
			Properties: pluginapi.PropertyMap{
				"namespace": depNs,
				"name":      depName,
			},
		})

		for svcName, svcID := range svcsByNs[depNs] {
			if envEnv, ok := findEnvRef(dep.Object, patterns[svcName]); ok {
				relations = append(relations, pluginapi.RelationClaim{
					Kind: pluginapi.RelationKindCalls,
					From: depID,
					To:   svcID,
					Properties: pluginapi.PropertyMap{
						"env": envEnv,
					},
				})
			}
		}
	}

	return pluginapi.IngestionRequest{Nodes: nodes, Relations: relations}, nil
}

// findEnvRef returns the first env var name whose value matches pat in any
// container (or initContainer) of the deployment's pod template, and true if found.
func findEnvRef(obj map[string]interface{}, pat *regexp.Regexp) (string, bool) {
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
				envName, _, _ := unstructured.NestedString(env, "name")
				envValue, _, _ := unstructured.NestedString(env, "value")
				if pat.MatchString(envValue) {
					return envName, true
				}
			}
		}
	}
	return "", false
}

func (p *Plugin) connect() (dynamic.Interface, error) {
	cfg, err := kubeconn.RestConfig(p.config.Kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("loading k8s config: %w", err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating k8s dynamic client: %w", err)
	}
	return dyn, nil
}
