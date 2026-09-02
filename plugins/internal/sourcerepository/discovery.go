package sourcerepository

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// Repository is a source repository discovered from a container image.
type Repository struct {
	URL    string
	Owner  string
	Name   string
	Method string
}

var sourceLabelKeys = []string{
	"org.opencontainers.image.source",
}

// FromImage discovers a source repository from OCI image metadata, falling back
// to inference from a GitHub Container Registry image name.
func FromImage(ctx context.Context, image string) (Repository, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return Repository{}, fmt.Errorf("parsing image reference: %w", err)
	}
	img, err := remote.Image(ref, remote.WithContext(ctx), remote.WithAuthFromKeychain(authn.DefaultKeychain), remote.WithTransport(http.DefaultTransport))
	if err != nil {
		if inferred := inferGitHubRepository(image); inferred != "" {
			return Repository{URL: inferred, Method: "INFERRED"}, nil
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

	if result.URL == "" {
		if inferred := inferGitHubRepository(image); inferred != "" {
			result.URL = inferred
			result.Method = "INFERRED"
		}
	}

	return result, nil
}

func inferGitHubRepository(image string) string {
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
