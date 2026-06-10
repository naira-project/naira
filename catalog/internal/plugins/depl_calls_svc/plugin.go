// Package depl_calls_svc implements a plugin scanning k8s Deployments and
// Services, then claiming "calls" relation from each Deployment that mentions
// a Service's name in any plaintext Env value.
package depl_calls_svc

import (
	"context"
	"fmt"
	"iter"
	"regexp"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/naira-project/naira/catalog/internal/kubeconn"
	"github.com/naira-project/naira/catalog/pluginapi"
)

const pluginName = "depl_calls_svc"

var (
	gvrDeployments = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	gvrServices    = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}
)

type Config struct {
	Enabled    bool   `env:"ENABLED" default:"true"`
	Kubeconfig string `env:"KUBECONFIG"`
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

	var claims pluginapi.IngestionRequest

	// Build per-namespace service index and precompile patterns.
	//
	// TODO(akavel-reply): configurable filter (blocklist)
	// TODO(akavel-reply): configurable list of extra names
	// TODO(akavel-reply): feature for loading service names from Naira
	svcsByNs, err := collectServicesByNamespace(ctx, dyn)
	if err != nil {
		return pluginapi.IngestionRequest{}, fmt.Errorf("collecting services: %w", err)
	}
	for _, svcs := range svcsByNs {
		for _, s := range svcs {
			claims.Nodes = append(claims.Nodes, pluginapi.NodeClaim{
				ID: s.nodeID,
			})
		}
	}

	// Scan Deployments' Env values, find references to Service names collected
	// above, interpret them as calls from the Deployment to the Service.
	depls, err := dyn.Resource(gvrDeployments).List(ctx, metav1.ListOptions{})
	if err != nil {
		return pluginapi.IngestionRequest{}, fmt.Errorf("listing deployments: %w", err)
	}
	for _, depl := range depls.Items {
		ns, name := depl.GetNamespace(), depl.GetName()
		nodeID := pluginapi.NodeID{
			Kind: pluginapi.NodeKindDeployment,
			Path: ns + "/" + name,
		}
		claims.Nodes = append(claims.Nodes, pluginapi.NodeClaim{
			ID: nodeID,
		})

		// Find references to services in the same namespace
		for _, svc := range svcsByNs[ns] {
			env, found := findEnvRef(depl.Object, svc.localPattern)
			if !found {
				continue
			}
			claims.Relations = append(claims.Relations, pluginapi.RelationClaim{
				Kind: pluginapi.RelationKindCalls,
				From: nodeID,
				To:   svc.nodeID,
				Properties: pluginapi.PropertyMap{
					"env": env,
				},
			})
		}

		// Find references to services with explicit namespace
		for svcNs, svcs := range svcsByNs {
			if svcNs == ns {
				continue // already checked above with shorter local pattern
			}
			for _, svc := range svcs {
				env, found := findEnvRef(depl.Object, svc.clusterPattern)
				if !found {
					continue
				}
				claims.Relations = append(claims.Relations, pluginapi.RelationClaim{
					Kind: pluginapi.RelationKindCalls,
					From: nodeID,
					To:   svc.nodeID,
					Properties: pluginapi.PropertyMap{
						"env": env,
					},
				})
			}
		}
	}

	return claims, nil
}

type serviceDetails struct {
	nodeID         pluginapi.NodeID
	localPattern   *regexp.Regexp // localPattern matches the service name as a whole word
	clusterPattern *regexp.Regexp // clusterPattern matches the service name in the form "${serviceName}.${namespace}"
}

func collectServicesByNamespace(ctx context.Context, dyn dynamic.Interface) (map[string][]serviceDetails, error) {
	svcs, err := dyn.Resource(gvrServices).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing services: %w", err)
	}

	svcsByNs := make(map[string][]serviceDetails) // ns -> services
	for _, svc := range svcs.Items {
		ns, name := svc.GetNamespace(), svc.GetName()
		localPattern, err := regexp.Compile(`\b` + regexp.QuoteMeta(name) + `\b`)
		if err != nil {
			return nil, fmt.Errorf("compiling local pattern for service %s/%s: %w", ns, name, err)
		}
		clusterPattern, err := regexp.Compile(`\b` + regexp.QuoteMeta(name+`.`+ns) + `\b`)
		if err != nil {
			return nil, fmt.Errorf("compiling cluster pattern for service %s/%s: %w", ns, name, err)
		}

		svcsByNs[ns] = append(svcsByNs[ns], serviceDetails{
			nodeID: pluginapi.NodeID{
				Kind: pluginapi.NodeKindService,
				Path: ns + "/" + name,
			},
			localPattern:   localPattern,
			clusterPattern: clusterPattern,
		})
	}
	return svcsByNs, nil
}

// findEnvRef returns the first env var name whose value matches pat in any
// container (or initContainer) of the deployment's pod template, and true if found.
func findEnvRef(obj map[string]interface{}, pat *regexp.Regexp) (string, bool) {
	for env, value := range iterDeploymentEnvs(obj) {
		if pat.MatchString(value) {
			return env, true
		}
	}
	return "", false
}

// iterDeploymentEnvs iterates all Env vars in a Deployment obj.
func iterDeploymentEnvs(obj map[string]interface{}) iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {

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

					if !yield(envName, envValue) {
						return
					}
				}
			}
		}

	}
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
