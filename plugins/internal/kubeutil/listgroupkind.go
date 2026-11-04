package kubeutil

import (
	"context"
	"log"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// ListGroupKind returns all resources of the given API group + kind across all
// provided namespaces. Returns an empty slice (not an error) if a queried CRD
// is not installed in the cluster. Failures to list in a particular namespace
// are logged and skipped. For apiLists, see e.g. the result of
// "ServerResourcesInterface.ServerGroupsAndResources" from
// "k8s.io/client-go/discovery" package.
func ListGroupKind(ctx context.Context, apiLists []*metav1.APIResourceList, dyn dynamic.Interface, group, kind string, namespaces []string) ([]unstructured.Unstructured, error) {
	for _, apiList := range apiLists {
		gv, err := schema.ParseGroupVersion(apiList.GroupVersion)
		if err != nil {
			log.Printf("kubeutil: WARN: skipping API group %q due to parsing error: %v", apiList.GroupVersion, err)
			continue
		}
		if gv.Group != group {
			continue
		}
		for _, res := range apiList.APIResources {
			if res.Kind != kind {
				continue
			}
			if !slices.Contains(res.Verbs, "list") {
				continue
			}
			gvr := schema.GroupVersionResource{Group: gv.Group, Version: gv.Version, Resource: res.Name}
			var items []unstructured.Unstructured
			for _, ns := range namespaces {
				// TODO[PERF]: try scanning `.Namespace(metav1.NamespaceAll)` first, only fall back to per-namespace if that fails due to RBAC.
				list, err := dyn.Resource(gvr).Namespace(ns).List(ctx, metav1.ListOptions{})
				if err != nil {
					log.Printf("kubeutil: WARN: listing %s/%s in namespace %q: %v", group, kind, ns, err)
					continue
				}
				items = append(items, list.Items...)
			}
			return items, nil
		}
	}
	return nil, nil
}
