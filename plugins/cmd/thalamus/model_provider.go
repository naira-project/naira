package main

import (
	"context"
	"fmt"
	"log"

	thalamusv1alpha1 "github.com/cobaltcore-dev/thalamus/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

type ModelProvider interface {
	ListModels(context.Context) ([]thalamusv1alpha1.Model, error)
}

type KubernetesModelProvider struct {
	client dynamic.Interface
}

func NewKubernetesModelProvider(client dynamic.Interface) *KubernetesModelProvider {
	return &KubernetesModelProvider{client: client}
}

func newModelProvider(logger *log.Logger) ModelProvider {
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		logger.Printf("not running in cluster, thalamus CRD source disabled: %v", err)
		return nil
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		logger.Printf("failed to create dynamic k8s client: %v", err)
		return nil
	}

	return NewKubernetesModelProvider(dynamicClient)
}

func (k *KubernetesModelProvider) ListModels(ctx context.Context) ([]thalamusv1alpha1.Model, error) {
	gvr := schema.GroupVersionResource{Group: "thalamus.cloud", Version: "v1alpha1", Resource: "models"}

	list, err := k.client.Resource(gvr).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing thalamus models: %w", err)
	}

	models := make([]thalamusv1alpha1.Model, 0, len(list.Items))
	for _, item := range list.Items {
		var m thalamusv1alpha1.Model
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, &m); err != nil {
			return nil, fmt.Errorf("converting thalamus model %q: %w", item.GetName(), err)
		}
		models = append(models, m)
	}

	return models, nil
}
