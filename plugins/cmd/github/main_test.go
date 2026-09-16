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

# $1=attestation $2=verify $3=oci://<image> ...
case "$3" in
  *service-a*)
    printf '[{"verificationResult":{"signature":{"certificate":{"sourceRepositoryURI":"https://github.com/naira-project/service-a"}}}}]'
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

	clientset := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system", UID: clusterUID}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}},
		deploymentWithImages(namespace, "app-a", "ghcr.io/naira-project/service-a:v1"),                 // org match, attestation verified
		deploymentWithImages(namespace, "app-b", "ghcr.io/other-org/service-b:v1"),                     // different org, skipped early
		deploymentWithImages(namespace, "app-c", "ghcr.io/naira-project/x", "ghcr.io/naira-project/y"), // two images, skipped early
		deploymentWithImages(namespace, "app-d", "ghcr.io/naira-project/mystery:v1"),                   // org match, attestation NOT verified
	)

	githubAPI := newFakeGithubAPI(t)
	ghPath := newFakeGhCLI(t)

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

	repoNode := pluginapi.NodeID{Kind: pluginapi.NodeKindGitRepository, Path: "github.com/naira-project/service-a"}
	ownerNode := pluginapi.NodeID{Kind: pluginapi.NodeKindOwner, Path: "@naira-project/team"}
	deploymentNode := pluginapi.NodeID{Kind: pluginapi.NodeKindDeployment, Path: clusterUID + "/" + namespace + "/app-a"}

	// Only app-a should have produced anything: app-b (wrong org), app-c
	// (multiple containers) and app-d (attestation not verified) must be
	// entirely absent from the result.
	assert.ElementsMatch(t, []pluginapi.NodeClaim{
		{ID: repoNode, Properties: pluginapi.PropertyMap{"url": "https://github.com/naira-project/service-a", "language": "Go"}},
		{ID: ownerNode},
		{ID: deploymentNode},
	}, resp.Nodes)

	assert.ElementsMatch(t, []pluginapi.RelationClaim{
		{Kind: pluginapi.RelationKindOwnedBy, From: repoNode, To: ownerNode},
		{Kind: pluginapi.RelationKindBuiltFrom, From: deploymentNode, To: repoNode},
	}, resp.Relations)
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

func TestImageReferencesOrg(t *testing.T) {
	tests := []struct {
		name  string
		image string
		org   string
		want  bool
	}{
		{
			name:  "matches organization",
			image: "ghcr.io/naira-project/service:latest",
			org:   "naira-project",
			want:  true,
		},
		{
			name:  "matches case insensitively",
			image: "ghcr.io/Naira-Project/service:latest",
			org:   "naira-project",
			want:  true,
		},
		{
			name:  "does not match another organization",
			image: "ghcr.io/other-org/service:latest",
			org:   "naira-project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, imageReferencesOrg(tt.image, tt.org))
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

	nodes, relations := codeownersClaims(repoNodeID, handles)

	assert.ElementsMatch(t, []pluginapi.NodeClaim{
		{ID: pluginapi.NodeID{Kind: pluginapi.NodeKindOwner, Path: "@acme/team"}},
		{ID: pluginapi.NodeID{Kind: pluginapi.NodeKindOwner, Path: "@alice"}},
	}, nodes)
	assert.ElementsMatch(t, []pluginapi.RelationClaim{
		{Kind: pluginapi.RelationKindOwnedBy, From: repoNodeID, To: pluginapi.NodeID{Kind: pluginapi.NodeKindOwner, Path: "@acme/team"}},
		{Kind: pluginapi.RelationKindOwnedBy, From: repoNodeID, To: pluginapi.NodeID{Kind: pluginapi.NodeKindOwner, Path: "@alice"}},
	}, relations)
}

func TestCodeownersClaims_Empty(t *testing.T) {
	nodes, relations := codeownersClaims(pluginapi.NodeID{Kind: pluginapi.NodeKindGitRepository, Path: "github.com/acme/service"}, nil)

	require.Empty(t, nodes)
	require.Empty(t, relations)
}
