package kubeutil

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const systemNamespace = "kube-system"

var gvrNamespaces = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}

// NamespacesAndClusterIDDynamic returns all namespaces names and a clusterID.
//
// The clusterID returned is really the UID of the "kube-system" system
// namespace. This is a common workaround for the absence of a builtin explicit
// cluster identifier property.
// See e.g.: https://opentelemetry.io/docs/specs/semconv/resource/k8s/#cluster
func NamespacesAndClusterIDDynamic(ctx context.Context, dyn dynamic.Interface) (namespaces []string, clusterID string, err error) {
	list, err := dyn.Resource(gvrNamespaces).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, "", fmt.Errorf("listing namespaces: %w", err)
	}
	for _, ns := range list.Items {
		namespaces = append(namespaces, ns.GetName())
		if ns.GetName() == systemNamespace {
			clusterID = string(ns.GetUID())
		}
	}
	if clusterID == "" {
		// should never happen - "kube-system" namespace is expected to always be present
		return nil, "", fmt.Errorf("namespace %q not found, cannot determine cluster ID", systemNamespace)
	}
	return namespaces, clusterID, nil
}

// NamespacesAndClusterID returns all namespaces names and a clusterID.
//
// The clusterID returned is really the UID of the "kube-system" system
// namespace. This is a common workaround for the absence of a builtin explicit
// cluster identifier property.
// See e.g.: https://opentelemetry.io/docs/specs/semconv/resource/k8s/#cluster
func NamespacesAndClusterID(ctx context.Context, client kubernetes.Interface) (namespaces []string, clusterID string, err error) {
	list, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, "", fmt.Errorf("listing namespaces: %w", err)
	}

	for _, ns := range list.Items {
		namespaces = append(namespaces, ns.Name)
		if ns.Name == systemNamespace {
			clusterID = string(ns.UID)
		}
	}

	if clusterID == "" {
		// should never happen - "kube-system" namespace is expected to always be present
		return nil, "", fmt.Errorf("namespace %q not found, cannot determine cluster ID", systemNamespace)
	}

	return namespaces, clusterID, nil
}
