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

// NamespacesAndClusterIDFromDynamic returns all namespaces names and a clusterID.
//
// The clusterID returned is really the UID of the "kube-system" system
// namespace. This is a common workaround for the absence of a builtin explicit
// cluster identifier property.
// See e.g.: https://opentelemetry.io/docs/specs/semconv/resource/k8s/#cluster
func NamespacesAndClusterIDFromDynamic(ctx context.Context, dyn dynamic.Interface) (namespaces []string, clusterID string, err error) {
	list, err := dyn.Resource(gvrNamespaces).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, "", fmt.Errorf("listing namespaces: %w", err)
	}

	entries := make([]nsEntry, 0, len(list.Items))
	for _, ns := range list.Items {
		entries = append(entries, nsEntry{name: ns.GetName(), uid: string(ns.GetUID())})
	}
	return namespacesAndClusterID(entries)
}

// NamespacesAndClusterIDFromClientset returns all namespaces names and a clusterID.
//
// The clusterID returned is really the UID of the "kube-system" system
// namespace. This is a common workaround for the absence of a builtin explicit
// cluster identifier property.
// See e.g.: https://opentelemetry.io/docs/specs/semconv/resource/k8s/#cluster
func NamespacesAndClusterIDFromClientset(ctx context.Context, client kubernetes.Interface) (namespaces []string, clusterID string, err error) {
	list, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, "", fmt.Errorf("listing namespaces: %w", err)
	}

	entries := make([]nsEntry, 0, len(list.Items))
	for _, ns := range list.Items {
		entries = append(entries, nsEntry{name: ns.Name, uid: string(ns.UID)})
	}
	return namespacesAndClusterID(entries)
}

type nsEntry struct {
	name string
	uid  string
}

// namespacesAndClusterID contains the logic shared by NamespacesAndClusterID
// and NamespacesAndClusterIDDynamic: collecting namespace names and picking
// the clusterID from the "kube-system" namespace's UID.
func namespacesAndClusterID(entries []nsEntry) (namespaces []string, clusterID string, err error) {
	for _, e := range entries {
		namespaces = append(namespaces, e.name)
		if e.name == systemNamespace {
			clusterID = e.uid
		}
	}
	if clusterID == "" {
		// should never happen - "kube-system" namespace is expected to always be present
		return nil, "", fmt.Errorf("namespace %q not found, cannot determine cluster ID", systemNamespace)
	}
	return namespaces, clusterID, nil
}
