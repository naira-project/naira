package deploymentdiscovery

import (
	"context"
	"fmt"
	"log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/naira-project/naira/plugins/internal/kubeutil"
	"github.com/naira-project/naira/plugins/internal/sourcerepository"
)

// Deployment is a Kubernetes Deployment with its container images and,
// optionally, a source repository discovered from those images.
type Deployment struct {
	ClusterID        string
	Namespace        string
	Name             string
	Images           []string
	SourceRepository sourcerepository.Repository
}

// DiscoverDeployments scans all Deployments across target namespaces and returns
// every deployment with its container images. Source repository discovery is
// attempted only for Deployments with exactly one container; Deployments with
// multiple containers are not linked to avoid ambiguous attribution.
func DiscoverDeployments(
	ctx context.Context,
	k8sClient kubernetes.Interface,
	logger *log.Logger,
) ([]Deployment, error) {
	namespaces, clusterID, err := kubeutil.NamespacesAndClusterID(ctx, k8sClient)
	if err != nil {
		return nil, fmt.Errorf("getting namespaces and cluster ID: %w", err)
	}

	var results []Deployment
	for _, namespace := range namespaces {
		deployments, err := k8sClient.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			if logger != nil {
				logger.Printf("WARN: failed to list deployments in namespace %q: %v", namespace, err)
			}
			continue
		}

		for _, deployment := range deployments.Items {
			containers := deployment.Spec.Template.Spec.Containers
			entry := Deployment{
				ClusterID: clusterID,
				Namespace: namespace,
				Name:      deployment.GetName(),
				Images:    make([]string, 0, len(containers)),
			}
			for _, container := range containers {
				entry.Images = append(entry.Images, container.Image)
			}

			repo, err := sourcerepository.FromImages(ctx, entry.Images)
			if err != nil {
				if logger != nil {
					logger.Printf("WARN: failed to inspect image in deployment %s/%s: %v", namespace, deployment.Name, err)
				}
			} else if repo.URL != "" {
				entry.SourceRepository = repo
			}
			results = append(results, entry)
		}
	}
	return results, nil
}
