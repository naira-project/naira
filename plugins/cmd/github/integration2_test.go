// Integration test for the github plugin.
// See the godoc of TestGithubPlugin_Integration for more details.
//
// For an overview on integration tests philosophy in the project,
// see: docs/integration-tests.md.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/k3s"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/naira-project/naira/plugins/pkg/oidctest"
)

const (
	integrationTestTimeout = 5 * time.Minute
	readinessTimeout       = 30 * time.Second
	dialAttemptTimeout     = time.Second
	pollInterval           = 200 * time.Millisecond

	pluginConnectionTimeout = 20 * time.Second
	pluginRunTimeout        = 30 * time.Second

	k3sImage       = "rancher/k3s:v1.28.2-k3s1"
	k3sNetAlias    = "k3s"
	pluginGRPCPort = "50051/tcp"

	// githubTestOrg/githubTestRepo is a real, public GitHub repository used
	// to exercise attestation verification, repo metadata and CODEOWNERS
	// lookup against the real GitHub API and a real "gh" CLI.
	githubTestOrg  = "naira-project"
	githubTestRepo = "naira"

	// TODO: fill these in with real sha256 digests of images published to
	// ghcr.io/naira-project/naira: one built via GitHub Actions with a
	// verifiable artifact attestation ("gh attestation verify" succeeds),
	// and one without an attestation (e.g. pushed manually, or from a
	// workflow run without attestations enabled).
	attestedImageSHA   = "sha256:9d1149eb56e62050316edd85b12a67bf67d4b74b"
	unattestedImageSHA = "sha256:866a6b2a4d119ba33bc9434073ca0c6850229c3f"

	attestedImage   = "ghcr.io/" + githubTestOrg + "/" + githubTestRepo + "@" + attestedImageSHA
	unattestedImage = "ghcr.io/" + githubTestOrg + "/" + githubTestRepo + "@" + unattestedImageSHA

	// otherOrgImage belongs to a different GitHub org on ghcr.io. It doesn't
	// need to exist for real: the plugin's ghcr.io optimization
	// (shouldVerifyImage) filters it out based on the registry path alone,
	// before ever invoking "gh attestation verify".
	otherOrgImage = "ghcr.io/other-org/service:v1"
)

// TestGithubPlugin_Integration tests a real build of the github plugin's
// container image against its "neighbor" components:
//
//   - the real GitHub API (api.github.com),
//   - a real "gh" CLI, downloaded and checksum-verified exactly as
//     plugins/cmd/github/Dockerfile does it, performing real
//     "gh attestation verify" calls,
//   - a real binary of the catalog,
//   - a real kubernetes cluster (k3s).
//
// Unlike a plain "go build" of the plugin, this test builds the actual
// plugins/cmd/github/Dockerfile with testcontainers and runs the resulting
// image as a container, so it also exercises the Dockerfile's pinned gh CLI
// download/checksum step and the distroless runtime environment - not just
// the plugin's Go code. The plugin container and the k3s container share a
// docker network so the plugin can reach the cluster the same way it would
// in production (via KUBECONFIG), without depending on anything from the
// host other than Docker and network access for the image build.
//
// It requires:
//
//   - a GITHUB_TOKEN environment variable with a valid GitHub token (a
//     classic token with no selected scopes is sufficient); the test is
//     skipped if it's missing, so it doesn't fail in environments that
//     aren't set up for it,
//   - Docker, with network access to pull base images and let the
//     Dockerfile download the gh CLI release, and for the running
//     container to reach api.github.com,
//   - github.com/naira-project/naira to be a real, public repository with
//     a top-level CODEOWNERS "*" rule, and two known image digests
//     published to ghcr.io/naira-project/naira (see attestedImageSHA and
//     unattestedImageSHA above).
//
// The catalog binary is still built and run natively with "go build" (as in
// other integration tests); only the plugin under test runs as a real
// container, since testing the Dockerfile is the point of this test.
//
// Test input: the k3s cluster is seeded with four Deployments:
//   - one running the attested image (single container),
//   - one running the unattested image (single container),
//   - one running an image on ghcr.io under a different, made-up org
//     (filtered out before ever calling gh, per the plugin's ghcr.io
//     optimization),
//   - one running two containers (skipped: ambiguous attribution).
//
// Test output assertion: only the first Deployment should end up linked, via
// the catalog API, to a git_repository node for naira-project/naira, which
// itself should have at least one CODEOWNERS-derived owner (exact owner
// handles aren't asserted, to avoid coupling the test to CODEOWNERS
// content). The other three Deployments must not produce any "built_from"
// relation.
//
// NOTE: In case of problems with the test, try increasing kernel inotify limit:
//
//	sudo sysctl -w fs.inotify.max_user_instances=512
func TestGithubPlugin_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		t.Skip("skipping: GITHUB_TOKEN not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), integrationTestTimeout)
	defer cancel()

	// Shared docker network so the plugin container can reach k3s the way it
	// would reach a cluster in production (a reachable API server address in
	// KUBECONFIG), rather than through the host-mapped port.
	nw, err := network.New(ctx)
	require.NoError(t, err, "creating docker network")
	t.Cleanup(func() { _ = nw.Remove(context.Background()) })

	// Start kubernetes (k3s) on that network, seeded with the four
	// Deployments described above.
	hostKubeconfig, clusterID := startK3s(ctx, t, nw,
		deploymentWithImages2("app-attested", attestedImage),
		deploymentWithImages2("app-unattested", unattestedImage),
		deploymentWithImages2("app-other-org", otherOrgImage),
		deploymentWithImages2("app-multi-container", attestedImage, unattestedImage),
	)
	// The plugin container resolves k3s by its network alias rather than
	// through the host-mapped port used by the test process above.
	containerKubeconfig := rewriteKubeconfigServer(t, hostKubeconfig, fmt.Sprintf("https://%s:6443", k3sNetAlias))

	// Start a mock OIDC (i.e. Keycloak-like) server.
	oidc := oidctest.New(t, "test-realm")

	// Build the real plugin image from plugins/cmd/github/Dockerfile and run
	// it as a container on the shared network.
	pluginContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		Started: true,
		ContainerRequest: testcontainers.ContainerRequest{
			FromDockerfile: testcontainers.FromDockerfile{
				Context:       repoRootDir(t),
				Dockerfile:    filepath.Join("plugins", "cmd", "github", "Dockerfile"),
				PrintBuildLog: true,
			},
			ExposedPorts: []string{pluginGRPCPort},
			Networks:     []string{nw.Name},
			Env: map[string]string{
				"PORT":         "50051",
				"GITHUB_ORG":   githubTestOrg,
				"GITHUB_TOKEN": githubToken,
				"KUBECONFIG":   "/kubeconfig.yaml",
				// GH_CLI_PATH is deliberately left unset: the point of this
				// test is to exercise the real Dockerfile, so it should
				// resolve "gh" from PATH exactly as the image ships it,
				// at /usr/local/bin/gh.
			},
			Files: []testcontainers.ContainerFile{
				{
					Reader:            bytes.NewReader(containerKubeconfig),
					ContainerFilePath: "/kubeconfig.yaml",
					FileMode:          0o644,
				},
			},
			WaitingFor: wait.ForListeningPort(pluginGRPCPort).WithStartupTimeout(readinessTimeout),
		},
	})
	require.NoError(t, err, "building/starting github plugin container")
	t.Cleanup(func() { _ = pluginContainer.Terminate(context.Background()) })

	pluginHost, err := pluginContainer.Host(ctx)
	require.NoError(t, err)
	pluginPort, err := pluginContainer.MappedPort(ctx, pluginGRPCPort)
	require.NoError(t, err)
	pluginAddr := fmt.Sprintf("%s:%s", pluginHost, pluginPort.Port())

	// Build and start catalog (native binary; not the subject of this test).
	catalogPort := findFreePort(t)
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(fmt.Sprintf(`
plugins:
  github:
    address: "%s"
`, pluginAddr)), 0o600), "writing config.yaml")
	buildAndStart(ctx, t, "github.com/naira-project/naira/catalog/cmd/catalog", []string{
		"PORT=" + fmt.Sprint(catalogPort),
		"PLUGIN_CONFIG_FILE=" + configPath,
		"PLUGIN_CONNECTION_TIMEOUT=" + pluginConnectionTimeout.String(),
		"PLUGIN_TIMEOUT=" + pluginRunTimeout.String(),
		"KEYCLOAK_BASE_URL=" + oidc.BaseURL,
		"KEYCLOAK_REALM=test-realm",
		"KEYCLOAK_ISSUER=" + oidc.Issuer,
	})
	catalogBaseURL := fmt.Sprintf("http://127.0.0.1:%d", catalogPort)
	require.Eventually(t, func() bool {
		return checkHTTPReady(ctx, catalogBaseURL+"/healthz")
	}, readinessTimeout, pollInterval, "catalog didn't become ready")

	// Generate an access token for authenticating to catalog's HTTP API.
	token := oidc.SignAccessToken(t, "test-user")

	// Trigger a run of the plugin through the catalog, and wait for the
	// operation to succeed.
	operationID := requestPluginRun(ctx, t, catalogBaseURL, token, "github")
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		op, status := doJSON[apiOperation](ctx, c, http.MethodGet, catalogBaseURL+"/v1/operations/"+operationID, token)
		assert.Equal(c, http.StatusOK, status, "GET /v1/operations/%s", operationID)
		assert.True(c, op.Done, operationErrorMessage(op))
		assert.NotNil(c, op.Response, operationErrorMessage(op))
	}, readinessTimeout, pollInterval, "operation %q didn't succeed", operationID)

	//
	// Verify nodes & relations in the catalog API.
	//

	var (
		pathPrefix     = clusterID + "/default/"
		repoPath       = "github.com/" + githubTestOrg + "/" + githubTestRepo
		attestedDepl   = pathPrefix + "app-attested"
		unattestedDepl = pathPrefix + "app-unattested"
		otherOrgDepl   = pathPrefix + "app-other-org"
		multiDepl      = pathPrefix + "app-multi-container"
	)

	type node struct {
		Kind string `json:"kind"`
		Path string `json:"path"`
	}
	nodes, status := doJSON[struct {
		Nodes []node `json:"nodes"`
	}](ctx, t, http.MethodGet, catalogBaseURL+"/v1/nodes", token)
	assert.Equal(t, http.StatusOK, status, "GET /v1/nodes")
	// TODO: when catalog API allows filtering by path prefix, tighten these
	// assertions. (Currently, there are extra namespaces and nodes from k8s
	// in the response, and we deliberately don't assert on exact CODEOWNERS
	// content, only that owners were found - see the test's godoc.)
	assert.Contains(t, nodes.Nodes, node{Kind: "git_repository", Path: repoPath},
		"expected a git_repository node for %s", repoPath)
	assert.Contains(t, nodes.Nodes, node{Kind: "deployment", Path: attestedDepl})

	var ownerCount int
	for _, n := range nodes.Nodes {
		if n.Kind == "owner" {
			ownerCount++
		}
	}
	assert.Positive(t, ownerCount, "expected at least one owner node from CODEOWNERS")

	type relation struct {
		Kind     string `json:"kind"`
		FromNode string `json:"fromNode"`
		ToNode   string `json:"toNode"`
	}
	relations, status := doJSON[struct {
		Relations []relation `json:"relations"`
	}](ctx, t, http.MethodGet, catalogBaseURL+"/v1/relations", token)
	assert.Equal(t, http.StatusOK, status, "GET /v1/relations")

	assert.Contains(t, relations.Relations, relation{
		Kind:     "built_from",
		FromNode: "nodes/deployment/" + attestedDepl,
		ToNode:   "nodes/git_repository/" + repoPath,
	})

	var sawOwnedBy bool
	for _, r := range relations.Relations {
		if r.Kind == "owned_by" && r.FromNode == "nodes/git_repository/"+repoPath {
			sawOwnedBy = true
			break
		}
	}
	assert.True(t, sawOwnedBy, "expected an owned_by relation from %s", repoPath)

	for _, deplPath := range []string{unattestedDepl, otherOrgDepl, multiDepl} {
		for _, r := range relations.Relations {
			assert.NotEqual(t, "nodes/deployment/"+deplPath, r.FromNode,
				"deployment %s should not be linked to anything", deplPath)
		}
	}
}

func deploymentWithImages2(name string, images ...string) *appsv1.Deployment {
	containers := make([]corev1.Container, 0, len(images))
	for i, image := range images {
		containers = append(containers, corev1.Container{
			Name:  fmt.Sprintf("container-%d", i),
			Image: image,
		})
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": name},
				},
				Spec: corev1.PodSpec{Containers: containers},
			},
		},
	}
}

// startK3s starts a k3s container on network netName (with alias
// k3sNetAlias), seeded with deployments, and returns a kubeconfig usable
// from the host (test process) plus the cluster ID (the kube-system
// namespace UID).
func startK3s(ctx context.Context, t *testing.T, net *testcontainers.DockerNetwork, deployments ...*appsv1.Deployment) (hostKubeconfig []byte, clusterID string) {
	t.Helper()

	k3sContainer, err := k3s.Run(ctx, k3sImage,
		// Trim optional components we don't need, to reduce the container's
		// resource footprint.
		testcontainers.WithCmdArgs(
			"--disable=metrics-server",
			"--disable=servicelb",
			"--disable=local-storage",
		),
		network.WithNetwork([]string{k3sNetAlias}, net),
	)
	require.NoError(t, err, "starting k3s container")
	t.Cleanup(func() { _ = k3sContainer.Terminate(context.Background()) })

	hostKubeconfig, err = k3sContainer.GetKubeConfig(ctx)
	require.NoError(t, err, "getting k3s kubeconfig")

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(hostKubeconfig)
	require.NoError(t, err)
	clientset, err := kubernetes.NewForConfig(restConfig)
	require.NoError(t, err)

	ns, err := clientset.CoreV1().Namespaces().Get(ctx, "kube-system", metav1.GetOptions{})
	require.NoError(t, err, "getting kube-system namespace")
	clusterID = string(ns.UID)

	for _, d := range deployments {
		_, err = clientset.AppsV1().Deployments(d.Namespace).Create(ctx, d, metav1.CreateOptions{})
		require.NoError(t, err, "creating deployment %s", d.Name)
	}

	return hostKubeconfig, clusterID
}

// rewriteKubeconfigServer returns a copy of kubeconfig with every cluster's
// server address replaced by server. This is used to hand the plugin
// container a kubeconfig that reaches k3s over the shared docker network,
// instead of the host-mapped port the test process itself uses.
func rewriteKubeconfigServer(t *testing.T, kubeconfig []byte, server string) []byte {
	t.Helper()

	cfg, err := clientcmd.Load(kubeconfig)
	require.NoError(t, err, "parsing kubeconfig")
	for _, cluster := range cfg.Clusters {
		cluster.Server = server
	}
	out, err := clientcmd.Write(*cfg)
	require.NoError(t, err, "serializing kubeconfig")
	return out
}

// repoRootDir returns the repository root (the directory containing go.mod),
// used as the Docker build context so plugins/cmd/github/Dockerfile can COPY
// go.mod, go.sum, plugins/pkg, plugins/internal and plugins/cmd/github.
func repoRootDir(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	require.NoError(t, err)
	dir, err := filepath.Abs(filepath.Join(wd, "..", "..", ".."))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "go.mod"))
	require.NoError(t, err, "could not locate repo root (expected go.mod under %s)", dir)
	return dir
}

// buildAndStart builds pkg, trying to use the same command line used in our
// Dockerfiles, then starts the resulting binary as a background process with
// extraEnv appended to the current environment. The process is killed if ctx
// is done or on test cleanup.
func buildAndStart(ctx context.Context, t *testing.T, pkg string, extraEnv []string) {
	t.Helper()

	binary := filepath.Join(t.TempDir(), filepath.Base(pkg))

	build := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", binary, pkg)
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	output, err := build.CombinedOutput()
	require.NoError(t, err, "building %s:\n%s", pkg, output)

	run := exec.CommandContext(ctx, binary)
	run.Env = append(os.Environ(), extraEnv...)
	run.Stdout = os.Stdout
	run.Stderr = os.Stderr
	require.NoError(t, run.Start(), "starting %s", binary)
	t.Cleanup(func() {
		_ = run.Process.Kill()
		_ = run.Wait()
	})
}

func findFreePort(t *testing.T) int {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// checkHTTPReady reports whether a GET to url succeeds with a 200 within
// dialAttemptTimeout.
func checkHTTPReady(ctx context.Context, url string) bool {
	ctx, cancel := context.WithTimeout(ctx, dialAttemptTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

type apiOperation struct {
	Name     string `json:"name"`
	Done     bool   `json:"done"`
	Response *struct {
		NodesUpserted     int `json:"nodesUpserted"`
		RelationsUpserted int `json:"relationsUpserted"`
	} `json:"response,omitempty"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func requestPluginRun(ctx context.Context, t *testing.T, catalogBaseURL, token, pluginName string) string {
	t.Helper()

	op, status := doJSON[apiOperation](ctx, t, http.MethodPost, catalogBaseURL+"/v1/plugins/"+pluginName+":run", token)
	require.Equal(t, http.StatusAccepted, status, "POST /v1/plugins/%s:run", pluginName)
	require.NotEmpty(t, op.Name)
	return op.Name
}

func operationErrorMessage(op apiOperation) string {
	if op.Error == nil {
		return ""
	}
	return op.Error.Message
}

func doJSON[T any](ctx context.Context, t require.TestingT, method, url, token string) (T, int) {
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}

	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var v T
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "reading response body: %s", string(body))
	require.NoError(t, json.Unmarshal(body, &v), "unmarshaling response body: %s", string(body))
	return v, resp.StatusCode
}
