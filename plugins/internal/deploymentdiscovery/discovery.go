package deploymentdiscovery

import (
	"context"
	"fmt"
	"log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/naira-project/naira/plugins/internal/kubeutil"
	"github.com/naira-project/naira/plugins/internal/repositoryidentity"
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
// every deployment with its container images. For the first container it also
// attempts to discover the source repository via OCI labels or image-name inference.
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

			if len(containers) > 0 {
				repo, err := sourcerepository.FromImage(ctx, containers[0].Image)
				if err != nil {
					if logger != nil {
						logger.Printf("WARN: failed to inspect image %q in deployment %s/%s: %v", containers[0].Image, namespace, deployment.Name, err)
					}
				} else if repo.URL != "" {
					if owner, name, ok := repositoryidentity.ParseGitHubRepository(repo.URL); ok {
						repo.Owner = owner
						repo.Name = name
					}
					entry.SourceRepository = repo
				}
			}
			results = append(results, entry)
		}
	}
	return results, nil
}

// Discover returns unique source repositories found in Deployments.
func Discover(ctx context.Context, client kubernetes.Interface, logger *log.Logger) ([]sourcerepository.Repository, error) {
	entries, err := DiscoverDeployments(ctx, client, logger)
	if err != nil {
		return nil, fmt.Errorf("discovering deployment repositories: %w", err)
	}
	seen := map[string]bool{}
	result := make([]sourcerepository.Repository, 0, len(entries))
	for _, entry := range entries {
		path := GitHubRepositoryNodePathFromReference(entry.SourceRepository)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		result = append(result, entry.SourceRepository)
	}
	return result, nil
}

// GitHubRepositoryNodePathFromReference returns the stable graph path for a
// GitHub repository reference, or an empty string when it is unsupported.
func GitHubRepositoryNodePathFromReference(repo sourcerepository.Repository) string {
	if repo.Owner != "" && repo.Name != "" {
		return repositoryidentity.GitHubRepositoryNodePath(repo.Owner, repo.Name)
	}
	return repositoryidentity.GitHubRepositoryNodePathFromURL(repo.URL)
}
