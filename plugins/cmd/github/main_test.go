package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

// newFakeGhCLI writes a stand-in "gh" binary that:
//   - answers "gh --version" with success,
//   - answers "gh attestation verify oci://<image> ..." based on the image
//     name, so different deployments can exercise different outcomes
//     (verified / not verified) within a single test.
func newFakeGhCLI(t *testing.T) string {
	t.Helper()

	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "gh version 2.63.2 (fake)"
  exit 0
fi

case "$3" in
  *service-a*)
    printf '[{"verificationResult":{"signature":{"certificate":{"sourceRepositoryURI":"https://github.com/naira-project/service-a"}}}}]'
    ;;
  *service-e*)
    printf '[{"verificationResult":{"signature":{"certificate":{"sourceRepositoryURI":"https://github.com/naira-project/service-e"}}}}]'
    ;;
  *)
    printf '[]'
    ;;
esac
exit 0
`
	path := filepath.Join(t.TempDir(), "gh")
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

func newFakeGithubAPI(t *testing.T) *httptest.Server {
	t.Helper()

	codeowners := base64.StdEncoding.EncodeToString([]byte("* @naira-project/team"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/naira-project/service-a":
			fmt.Fprint(w, `{"html_url":"https://github.com/naira-project/service-a","language":"Go"}`)
		case "/repos/naira-project/service-a/contents/.github/CODEOWNERS":
			fmt.Fprintf(w, `{"content":%q,"encoding":"base64"}`, codeowners)
		case "/repos/naira-project/service-e":
			fmt.Fprint(w, `{"html_url":"https://github.com/naira-project/service-e","language":"Go"}`)
		case "/repos/naira-project/service-e/contents/.github/CODEOWNERS":
			fmt.Fprintf(w, `{"content":%q,"encoding":"base64"}`, codeowners)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestPlugin_Collect(t *testing.T) {
	githubAPI := newFakeGithubAPI(t)
	ghPath := newFakeGhCLI(t)

	var (
		repoA     = nodeID("git_repository", "github.com/naira-project/service-a")
		repoE     = nodeID("git_repository", "github.com/naira-project/service-e")
		ownerTeam = nodeID("owner", "github.com/@naira-project/team")
	)

	deployment := func(name string) pluginapi.NodeID {
		return nodeID("deployment", "test-cluster-uid/default/"+name)
	}

	tests := []struct {
		name               string
		imagesByDeployment map[string][]string
		wantNodes          []pluginapi.NodeClaim
		wantRelations      []pluginapi.RelationClaim
	}{
		{
			name:               "org match, attestation verified: repo, owner and deployment linked",
			imagesByDeployment: map[string][]string{"app": {"ghcr.io/naira-project/service-a:v1"}},
			wantNodes: []pluginapi.NodeClaim{
				{
					ID:         repoA,
					Properties: pluginapi.PropertyMap{"url": "https://github.com/naira-project/service-a", "language": "Go"},
				},
				{ID: ownerTeam},
				{ID: deployment("app")},
			},
			wantRelations: []pluginapi.RelationClaim{
				{Kind: "owned_by",
					From: repoA,
					To:   ownerTeam,
				},
				{Kind: "built_from",
					From: deployment("app"),
					To:   repoA,
				},
			},
		},
		{
			name:               "different org: nothing produced",
			imagesByDeployment: map[string][]string{"app": {"ghcr.io/other-org/service-b:v1"}},
		},
		{
			name:               "multiple images on one deployment: nothing produced",
			imagesByDeployment: map[string][]string{"app": {"ghcr.io/naira-project/service-a:v1", "ghcr.io/naira-project/service-e:v1"}},
		},
		{
			name:               "org match, attestation not verified: nothing produced",
			imagesByDeployment: map[string][]string{"app": {"ghcr.io/naira-project/mystery:v1"}},
		},
		{
			name: "two repos sharing an owner: owner node deduplicated, both relations kept",
			imagesByDeployment: map[string][]string{
				"app-a": {"ghcr.io/naira-project/service-a:v1"},
				"app-e": {"ghcr.io/naira-project/service-e:v1"},
			},
			wantNodes: []pluginapi.NodeClaim{
				{
					ID:         repoA,
					Properties: pluginapi.PropertyMap{"url": "https://github.com/naira-project/service-a", "language": "Go"},
				},
				{
					ID:         repoE,
					Properties: pluginapi.PropertyMap{"url": "https://github.com/naira-project/service-e", "language": "Go"},
				},
				{ID: ownerTeam}, // only once, even though both repos share it
				{ID: deployment("app-a")},
				{ID: deployment("app-e")},
			},
			wantRelations: []pluginapi.RelationClaim{
				{Kind: "owned_by",
					From: repoA,
					To:   ownerTeam,
				},
				{Kind: "owned_by",
					From: repoE,
					To:   ownerTeam,
				},
				{Kind: "built_from",
					From: deployment("app-a"),
					To:   repoA,
				},
				{Kind: "built_from",
					From: deployment("app-e"),
					To:   repoE,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system", UID: "test-cluster-uid"}},
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			}
			for name, images := range tt.imagesByDeployment {
				objs = append(objs, deploymentWithImages("default", name, images...))
			}
			clientset := fake.NewSimpleClientset(objs...)

			p := New(config{
				GitHubOrg:          "naira-project",
				GitHubToken:        "test-token",
				GitHubBaseURL:      githubAPI.URL,
				HTTPTimeout:        5 * time.Second,
				GHCLIPath:          ghPath,
				AttestationTimeout: 5 * time.Second,
			}, log.New(io.Discard, "", 0))

			resp, err := p.collect(t.Context(), clientset)
			require.NoError(t, err)

			assert.ElementsMatch(t, tt.wantNodes, resp.Nodes)
			assert.ElementsMatch(t, tt.wantRelations, resp.Relations)
		})
	}
}

func deploymentWithImages(namespace, name string, images ...string) *appsv1.Deployment {
	containers := make([]corev1.Container, 0, len(images))
	for i, image := range images {
		containers = append(containers, corev1.Container{
			Name:  fmt.Sprintf("container-%d", i),
			Image: image,
		})
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: containers},
			},
		},
	}
}

func TestShouldVerifyImage(t *testing.T) {
	tests := []struct {
		name      string
		image     string
		githubOrg string
		want      bool
	}{
		{
			name:      "ghcr.io matching organization",
			image:     "ghcr.io/naira-project/service:latest",
			githubOrg: "naira-project",
			want:      true,
		},
		{
			name:      "ghcr.io matching case-insensitively",
			image:     "ghcr.io/Naira-Project/service:latest",
			githubOrg: "naira-project",
			want:      true,
		},
		{
			name:      "ghcr.io belonging to another organization",
			image:     "ghcr.io/other-org/service:latest",
			githubOrg: "naira-project",
			want:      false,
		},
		{
			name:      "non-ghcr registry returns true",
			image:     "123456789.dkr.ecr.eu-west-1.amazonaws.com/my-service:v1.0",
			githubOrg: "naira-project",
			want:      true,
		},
		{
			name:      "docker hub registry returns true",
			image:     "docker.io/library/redis:alpine",
			githubOrg: "naira-project",
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Plugin{
				config: config{
					GitHubOrg: tt.githubOrg,
				},
			}
			assert.Equal(t, tt.want, p.shouldVerifyImage(tt.image))
		})
	}
}

func TestSingleContainerImage(t *testing.T) {
	tests := []struct {
		name      string
		images    []string
		wantImage string
		wantOK    bool
	}{
		{
			name:   "no containers",
			wantOK: false,
		},
		{
			name:   "multiple containers",
			images: []string{"a", "b"},
			wantOK: false,
		},
		{
			name:      "one container",
			images:    []string{"ghcr.io/acme/service:v1"},
			wantImage: "ghcr.io/acme/service:v1",
			wantOK:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotImage, gotOK := singleContainerImage(Deployment{Images: tt.images})
			assert.Equal(t, tt.wantImage, gotImage)
			assert.Equal(t, tt.wantOK, gotOK)
		})
	}
}

func TestCodeownersClaims(t *testing.T) {
	repoNodeID := nodeID("git_repository", "github.com/acme/service")
	handles := []string{"@acme/team", "@alice"}

	nodes, relations := codeownersClaims(repoNodeID, handles, make(map[pluginapi.NodeID]bool))

	assert.ElementsMatch(t, []pluginapi.NodeClaim{
		{ID: nodeID("owner", "github.com/@acme/team")},
		{ID: nodeID("owner", "github.com/@alice")},
	}, nodes)
	assert.ElementsMatch(t, []pluginapi.RelationClaim{
		{Kind: "owned_by",
			From: repoNodeID,
			To:   nodeID("owner", "github.com/@acme/team"),
		},
		{Kind: "owned_by",
			From: repoNodeID,
			To:   nodeID("owner", "github.com/@alice"),
		},
	}, relations)
}

func TestCodeownersClaims_Empty(t *testing.T) {
	repoNodeID := nodeID("git_repository", "github.com/acme/service")

	nodes, relations := codeownersClaims(repoNodeID, nil, make(map[pluginapi.NodeID]bool))

	require.Empty(t, nodes)
	require.Empty(t, relations)
}

func nodeID(kind, path string) pluginapi.NodeID {
	return pluginapi.NodeID{Kind: kind, Path: path}
}
