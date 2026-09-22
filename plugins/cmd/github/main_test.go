package main

import (
	"context"
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

	codeowners := base64.StdEncoding.EncodeToString([]byte("* @naira-project/team\n"))

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
	const (
		clusterUID = "test-cluster-uid"
		namespace  = "default"
	)

	githubAPI := newFakeGithubAPI(t)
	ghPath := newFakeGhCLI(t)

	repoA := pluginapi.NodeID{Kind: pluginapi.NodeKindGitRepository, Path: "github.com/naira-project/service-a"}
	repoE := pluginapi.NodeID{Kind: pluginapi.NodeKindGitRepository, Path: "github.com/naira-project/service-e"}
	ownerTeam := pluginapi.NodeID{Kind: pluginapi.NodeKindOwner, Path: "github.com/@naira-project/team"}

	deploymentNode := func(name string) pluginapi.NodeID {
		return pluginapi.NodeID{Kind: pluginapi.NodeKindDeployment, Path: clusterUID + "/" + namespace + "/" + name}
	}

	tests := []struct {
		name          string
		deployments   map[string][]string // deployment name -> container images
		wantNodes     []pluginapi.NodeClaim
		wantRelations []pluginapi.RelationClaim
	}{
		{
			name:        "org match, attestation verified: repo, owner and deployment linked",
			deployments: map[string][]string{"app": {"ghcr.io/naira-project/service-a:v1"}},
			wantNodes: []pluginapi.NodeClaim{
				{ID: repoA, Properties: pluginapi.PropertyMap{"url": "https://github.com/naira-project/service-a", "language": "Go"}},
				{ID: ownerTeam},
				{ID: deploymentNode("app")},
			},
			wantRelations: []pluginapi.RelationClaim{
				{Kind: pluginapi.RelationKindOwnedBy, From: repoA, To: ownerTeam},
				{Kind: pluginapi.RelationKindBuiltFrom, From: deploymentNode("app"), To: repoA},
			},
		},
		{
			name:        "different org: nothing produced",
			deployments: map[string][]string{"app": {"ghcr.io/other-org/service-b:v1"}},
		},
		{
			name:        "multiple containers: nothing produced",
			deployments: map[string][]string{"app": {"ghcr.io/naira-project/service-a:v1", "ghcr.io/naira-project/service-e:v1"}},
		},
		{
			name:        "org match, attestation not verified: nothing produced",
			deployments: map[string][]string{"app": {"ghcr.io/naira-project/mystery:v1"}},
		},
		{
			name: "two repos sharing an owner: owner node deduplicated, both relations kept",
			deployments: map[string][]string{
				"app-a": {"ghcr.io/naira-project/service-a:v1"},
				"app-e": {"ghcr.io/naira-project/service-e:v1"},
			},
			wantNodes: []pluginapi.NodeClaim{
				{ID: repoA, Properties: pluginapi.PropertyMap{"url": "https://github.com/naira-project/service-a", "language": "Go"}},
				{ID: repoE, Properties: pluginapi.PropertyMap{"url": "https://github.com/naira-project/service-e", "language": "Go"}},
				{ID: ownerTeam}, // only once, even though both repos share it
				{ID: deploymentNode("app-a")},
				{ID: deploymentNode("app-e")},
			},
			wantRelations: []pluginapi.RelationClaim{
				{Kind: pluginapi.RelationKindOwnedBy, From: repoA, To: ownerTeam},
				{Kind: pluginapi.RelationKindOwnedBy, From: repoE, To: ownerTeam},
				{Kind: pluginapi.RelationKindBuiltFrom, From: deploymentNode("app-a"), To: repoA},
				{Kind: pluginapi.RelationKindBuiltFrom, From: deploymentNode("app-e"), To: repoE},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system", UID: clusterUID}},
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}},
			}
			for name, images := range tt.deployments {
				objs = append(objs, deploymentWithImages(namespace, name, images...))
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

			resp, err := p.collect(context.Background(), clientset)
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
		{name: "no containers"},
		{name: "multiple containers", images: []string{"a", "b"}},
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
	repoNodeID := pluginapi.NodeID{Kind: pluginapi.NodeKindGitRepository, Path: "github.com/acme/service"}
	handles := []string{"@acme/team", "@alice"}

	nodes, relations := codeownersClaims(repoNodeID, handles, make(map[pluginapi.NodeID]bool))

	assert.ElementsMatch(t, []pluginapi.NodeClaim{
		{ID: pluginapi.NodeID{Kind: pluginapi.NodeKindOwner, Path: "github.com/@acme/team"}},
		{ID: pluginapi.NodeID{Kind: pluginapi.NodeKindOwner, Path: "github.com/@alice"}},
	}, nodes)
	assert.ElementsMatch(t, []pluginapi.RelationClaim{
		{Kind: pluginapi.RelationKindOwnedBy, From: repoNodeID, To: pluginapi.NodeID{Kind: pluginapi.NodeKindOwner, Path: "github.com/@acme/team"}},
		{Kind: pluginapi.RelationKindOwnedBy, From: repoNodeID, To: pluginapi.NodeID{Kind: pluginapi.NodeKindOwner, Path: "github.com/@alice"}},
	}, relations)
}

func TestCodeownersClaims_Empty(t *testing.T) {
	nodes, relations := codeownersClaims(pluginapi.NodeID{Kind: pluginapi.NodeKindGitRepository, Path: "github.com/acme/service"}, nil, make(map[pluginapi.NodeID]bool))

	require.Empty(t, nodes)
	require.Empty(t, relations)
}
