package main

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type AppIdentity struct {
	Name              string `json:"name"`
	Namespace         string `json:"namespace"`
	Team              string `json:"team"`
	LiteLLMVirtualKey string `json:"litellm_virtual_key"`
}

type AppIdentityProvider interface {
	ListAppIdentities(context.Context) ([]AppIdentity, error)
}

type KubernetesAppIdentityProvider struct {
	client dynamic.Interface
}

func NewKubernetesAppIdentityProvider(client dynamic.Interface) *KubernetesAppIdentityProvider {
	return &KubernetesAppIdentityProvider{client: client}
}

// Custom AppIdentity CRD allows applications to declare their identity and relationship to LLMs,
// which can be ingested by the catalog and used to populate model adopter information.
// This is an optional plugin component, and the plugin will function without it.
func (k *KubernetesAppIdentityProvider) ListAppIdentities(ctx context.Context) ([]AppIdentity, error) {
	gvr := schema.GroupVersionResource{Group: "naira.io", Version: "v1alpha1", Resource: "appidentities"}

	list, err := k.client.Resource(gvr).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list app identities: %w", err)
	}

	apps := make([]AppIdentity, 0, len(list.Items))
	for _, item := range list.Items {
		app := AppIdentity{
			Name:      item.GetName(),
			Namespace: item.GetNamespace(),
		}

		specMap, ok := item.Object["spec"].(map[string]any)
		if ok {
			if team, ok := specMap["team"].(string); ok {
				app.Team = team
			}
			if key, ok := specMap["litellmVirtualKey"].(string); ok {
				app.LiteLLMVirtualKey = key
			}
		}

		apps = append(apps, app)
	}

	return apps, nil
}
