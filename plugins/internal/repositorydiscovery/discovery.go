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

// DeploymentRepository associates a discovered repository with its Deployment details.
type DeploymentRepository struct {
	ClusterID  string
	Namespace  string
	Deployment string
	Image      string
	Repository Repository
}

var sourceLabelKeys = []string{
	"org.opencontainers.image.source",
}

// DiscoverDeploymentRepositories scans Deployments across target namespaces (or all namespaces)
// and returns discovered repository associations for each deployment's first discoverable container.
func DiscoverDeploymentRepositories(
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
			for _, container := range dep.Spec.Template.Spec.Containers {
				repo, err := InspectImage(ctx, container.Image)
				if err != nil {
					if logger != nil {
						logger.Printf("WARN: failed to inspect image %q in deployment %s/%s: %v", container.Image, ns, dep.Name, err)
					}
					continue
				}
				if repo.URL == "" {
					continue
				}

				if owner, name, ok := ParseGitHubRepository(repo.URL); ok {
					repo.Owner = owner
					repo.Name = name
				}

				results = append(results, DeploymentRepository{
					ClusterID:  clusterID,
					Namespace:  ns,
					Deployment: dep.Name,
					Image:      container.Image,
					Repository: repo,
				})
				break // Stop after finding the first container with a valid repository
			}
		}
	}

	return results, nil
}

// Discover returns unique repositories found in Deployments, including non-GitHub repositories.
func Discover(ctx context.Context, client kubernetes.Interface, logger *log.Logger) ([]Repository, error) {
	entries, err := DiscoverDeploymentRepositories(ctx, client, logger)
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

// InspectImage reads source and revision labels from an image config,
// falling back to repository inference
func InspectImage(ctx context.Context, image string) (Repository, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return Repository{}, fmt.Errorf("parsing image reference: %w", err)
	}
	img, err := remote.Image(ref, remote.WithContext(ctx), remote.WithAuthFromKeychain(authn.DefaultKeychain), remote.WithTransport(http.DefaultTransport))
	if err != nil {
		// Fallback to infer repository from GHCR image name
		if inferred := InferRepository(image); inferred != "" {
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
		if inferred := InferRepository(image); inferred != "" {
			result.URL = inferred
			result.Method = "INFERRED"
		}
	}

	return result, nil
}

// InferRepository infers a GitHub repository from a GHCR image.
func InferRepository(image string) string {
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
	owner, name, ok := ParseGitHubRepository(rawURL)
	if !ok {
		log.Printf("WARN: failed to parse GitHub repository from URL %q", rawURL)
		return ""
	}

	return githubCanonicalPath(owner, name)
}

func githubCanonicalPath(owner, name string) string {
	return "github.com/" + strings.ToLower(owner+"/"+name)
}

// ParseGitHubRepository returns the owner and repository name from a GitHub URL.
func ParseGitHubRepository(rawURL string) (owner, name string, ok bool) {
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
