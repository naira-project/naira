package kubeutil

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const systemNamespace = "kube-system"

var gvrNamespaces = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}

// NamespacesAndClusterID returns all namespaces names and a clusterID.
//
// The clusterID returned is really the UID of the "kube-system" system
// namespace. This is a common workaround for the absence of a builtin explicit
// cluster identifier property.
// See e.g.: https://opentelemetry.io/docs/specs/semconv/resource/k8s/#cluster
func NamespacesAndClusterID(ctx context.Context, dyn dynamic.Interface) (namespaces []string, clusterID string, err error) {
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
