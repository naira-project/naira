package repositorydiscovery

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/naira-project/naira/plugins/internal/kubeutil"
)

// Repository is a repository reference discovered from a Kubernetes Deployment.
type Repository struct {
	URL    string
	Owner  string
	Name   string
	Method string
}

// DeploymentRepository associates a Deployment with its container images and,
// optionally, a discovered source repository. Repository may be zero-value
// (empty struct) when no repository was discoverable for the deployment's images.
type DeploymentRepository struct {
	ClusterID  string
	Namespace  string
	Deployment string
	Images     []string
	Repository Repository
}

var sourceLabelKeys = []string{
	"org.opencontainers.image.source",
}

// DiscoverDeployments scans all Deployments across target namespaces and returns
// every deployment with its container images. For the first container it also
// attempts to discover the source repository via OCI labels or image-name inference;
// the Repository field is populated only when discovery succeeds.
func DiscoverDeployments(
	ctx context.Context,
	k8sClient kubernetes.Interface,
	logger *log.Logger,
) ([]DeploymentRepository, error) {
	var namespaces []string

	namespaces, clusterID, err := kubeutil.NamespacesAndClusterID(ctx, k8sClient)
	if err != nil {
		return nil, fmt.Errorf("getting namespaces and cluster ID: %w", err)
	}

	var results []DeploymentRepository

	for _, ns := range namespaces {
		deployments, err := k8sClient.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			if logger != nil {
				logger.Printf("WARN: failed to list deployments in namespace %q: %v", ns, err)
			}
			continue
		}

		for _, dep := range deployments.Items {
			containers := dep.Spec.Template.Spec.Containers

			entry := DeploymentRepository{
				ClusterID:  clusterID,
				Namespace:  ns,
				Deployment: dep.GetName(),
				Images:     make([]string, 0, len(containers)),
			}

			for _, container := range containers {
				entry.Images = append(entry.Images, container.Image)
			}

			// Try to discover the source repository from the primary container.
			// A failed or empty discovery is non-fatal — the deployment is still
			// emitted without a Repository link.
			if len(containers) > 0 {
				repo, err := inspectImage(ctx, containers[0].Image)
				if err != nil {
					if logger != nil {
						logger.Printf("WARN: failed to inspect image %q in deployment %s/%s: %v", containers[0].Image, ns, dep.Name, err)
					}
				} else if repo.URL != "" {
					if owner, name, ok := parseGitHubRepository(repo.URL); ok {
						repo.Owner = owner
						repo.Name = name
					}
					entry.Repository = repo
				}
			}

			results = append(results, entry)
		}
	}

	return results, nil
}

// Discover returns unique repositories found in Deployments, including non-GitHub repositories.
// It only returns entries where a repository was successfully discovered.
func Discover(ctx context.Context, client kubernetes.Interface, logger *log.Logger) ([]Repository, error) {
	entries, err := DiscoverDeployments(ctx, client, logger)
	if err != nil {
		return nil, fmt.Errorf("discovering deployment repositories: %w", err)
	}
	seen := map[string]bool{}
	result := make([]Repository, 0, len(entries))
	for _, entry := range entries {
		key := CanonicalPathFromRepo(entry.Repository)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, entry.Repository)
	}
	return result, nil
}

// inspectImage reads source and revision labels from an image config,
// falling back to repository inference
func inspectImage(ctx context.Context, image string) (Repository, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return Repository{}, fmt.Errorf("parsing image reference: %w", err)
	}
	img, err := remote.Image(ref, remote.WithContext(ctx), remote.WithAuthFromKeychain(authn.DefaultKeychain), remote.WithTransport(http.DefaultTransport))
	if err != nil {
		// Fallback to infer repository from GHCR image name
		if inferred := inferRepository(image); inferred != "" {
			return Repository{
				URL:    inferred,
				Method: "INFERRED",
			}, nil
		}
		return Repository{}, fmt.Errorf("fetching image metadata: %w", err)
	}

	manifest, err := img.ConfigFile()
	if err != nil {
		return Repository{}, fmt.Errorf("reading image metadata: %w", err)
	}

	result := Repository{}
	if manifest.Config.Labels != nil {
		for _, key := range sourceLabelKeys {
			if value := manifest.Config.Labels[key]; value != "" {
				result.URL = value
				result.Method = "OCI_STANDARD"
				break
			}
		}
	}

	// Fallback to infer repository from GHCR image name
	if result.URL == "" {
		if inferred := inferRepository(image); inferred != "" {
			result.URL = inferred
			result.Method = "INFERRED"
		}
	}

	return result, nil
}

// inferRepository infers a GitHub repository from a GHCR image.
func inferRepository(image string) string {
	if !strings.HasPrefix(image, "ghcr.io/") {
		return ""
	}
	parts := strings.Split(strings.TrimPrefix(image, "ghcr.io/"), "/")
	if len(parts) < 2 {
		return ""
	}
	repo := strings.Split(strings.Split(parts[1], ":")[0], "@")[0]
	if repo == "" {
		return ""
	}
	return "https://github.com/" + parts[0] + "/" + repo
}

func CanonicalPathFromRepo(repo Repository) string {
	if repo.Owner != "" && repo.Name != "" {
		return githubCanonicalPath(repo.Owner, repo.Name)
	}
	return CanonicalPath(repo.URL)
}

// CanonicalPath returns the canonical NodeID path for a GitHub repository URL.
// URLs for other Git hosts are not supported and return an empty path.
func CanonicalPath(rawURL string) string {
	owner, name, ok := parseGitHubRepository(rawURL)
	if !ok {
		log.Printf("WARN: failed to parse GitHub repository from URL %q", rawURL)
		return ""
	}

	return githubCanonicalPath(owner, name)
}

func githubCanonicalPath(owner, name string) string {
	return "github.com/" + strings.ToLower(owner+"/"+name)
}

// parseGitHubRepository returns the owner and repository name from a GitHub URL.
func parseGitHubRepository(rawURL string) (owner, name string, ok bool) {
	value := strings.TrimSpace(rawURL)
	value = strings.TrimPrefix(value, "git@github.com:")
	value = strings.TrimPrefix(value, "https://")
	value = strings.TrimPrefix(value, "http://")
	value = strings.TrimPrefix(value, "github.com/")
	if strings.Contains(value, "://") || strings.Contains(value, "@") {
		return "", "", false
	}
	value = strings.TrimSuffix(value, ".git")
	parts := strings.Split(strings.Trim(value, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
