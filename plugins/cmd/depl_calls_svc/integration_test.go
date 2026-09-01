// Integration test for the depl_calls_svc plugin.
// See the godoc of TestDeplCallsSvc_Integration for more details.
//
// For an overview on integration tests philosophy in the project,
// see: docs/integration-tests.md.
package main

import (
	"context"
	"encoding/json"
	"fmt"
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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
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

	k3sImage = "rancher/k3s:v1.28.2-k3s1"
	// tinyImage is intended to be a tiny image that is already downloaded by
	// k3s and in its cache, so that it spins up fast. Verify presence with:
	//   cid=$(docker run -d --privileged rancher/k3s:v1.28.2-k3s1 server)
	//   docker logs -f "$cid" 2>&1 | grep -m1 "Node controller sync successful"
	//   docker exec "$cid" crictl -r unix:///run/k3s/containerd/containerd.sock images
	//   docker rm -f "$cid"
	tinyImage = "docker.io/rancher/mirrored-pause:3.6"
)

// TestDeplCallsSvc_Integration tests a real binary of the depl_calls_svc
// plugin against its "neighbor" components:
//
//   - a real binary of the catalog,
//   - a real kubernetes cluster (k3s).
//
// The plugin & catalog binaries are built as part of the test (should be
// mostly cached on repeated runs).
//
// Test input: the k3s cluster is seeded with a Service, and a Deployment
// referencing it via Env.
//
// Test output assertion: the catalog API should show 2 nodes (for the Service
// and the Deployment), and a "calls" relation between them.
func TestDeplCallsSvc_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(t.Context(), integrationTestTimeout)
	defer cancel()

	// Start kubernetes (k3s), with a Service and a Deployment. The
	// Deployment's Env points to the Service.
	kubeconfigPath, clusterID := startK3s(t, ctx,
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sample-svc",
				Namespace: "default",
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{
					Port:       80,
					TargetPort: intstr.FromInt(8080),
				}},
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sample-depl",
				Namespace: "default",
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: new(int32(1)),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "sample-app"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "sample-app"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:  "sample-container",
							Image: tinyImage,
							Env: []corev1.EnvVar{{
								Name:  "SAMPLE_URL",
								Value: "http://sample-svc/api",
							}},
						}},
					},
				},
			},
		},
	)

	// Start a mock OIDC (i.e. Keycloak-like) server.
	oidc := oidctest.New(t, "test-realm")

	// Build and start the plugin.
	pluginPort := findFreePort(t)
	buildAndStart(t, ctx, "github.com/naira-project/naira/plugins/cmd/depl_calls_svc", []string{
		"PORT=" + fmt.Sprint(pluginPort),
		"DEPL_CALLS_SVC_KUBECONFIG=" + kubeconfigPath,
	})
	pluginAddr := fmt.Sprintf("127.0.0.1:%d", pluginPort)
	require.Eventually(t, func() bool {
		return checkTCPReady(ctx, pluginAddr)
	}, readinessTimeout, pollInterval, "plugin at %s didn't start accepting connections", pluginAddr)

	// Build and start catalog.
	catalogPort := findFreePort(t)
	buildAndStart(t, ctx, "github.com/naira-project/naira/catalog/cmd/catalog", []string{
		"PORT=" + fmt.Sprint(catalogPort),
		"PLUGIN_ADDRESSES=depl_calls_svc=" + pluginAddr,
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
	operationID := requestPluginRun(t, ctx, catalogBaseURL, token)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		op, status := doJSON[apiOperation](c, ctx, http.MethodGet, catalogBaseURL+"/v1/operations/"+operationID, token)
		assert.Equal(c, http.StatusOK, status, "GET /v1/operations/%s", operationID)
		assert.Equal(c, "SUCCEEDED", op.State, operationErrorMessage(op))
	}, readinessTimeout, pollInterval, "operation %q didn't succeed", operationID)

	//
	// Verify nodes & relations in the catalog API.
	//

	var (
		pathPrefix   = clusterID + "/default/"
		wantDeplPath = pathPrefix + "sample-depl"
		wantSvcPath  = pathPrefix + "sample-svc"
	)

	type node struct {
		Kind string `json:"kind"`
		Path string `json:"path"`
	}
	nodes, status := doJSON[struct {
		Nodes []node `json:"nodes"`
	}](t, ctx, http.MethodGet, catalogBaseURL+"/v1/nodes", token)
	assert.Equal(t, http.StatusOK, status, "GET /v1/nodes")
	// TODO: when catalog API allows filtering by path prefix, switch to assert.ElementsMatch
	// (Currently, there are extra namespaces and nodes from k8s in the response.)
	assert.Contains(t, nodes.Nodes, node{Kind: "deployment", Path: wantDeplPath})
	assert.Contains(t, nodes.Nodes, node{Kind: "service", Path: wantSvcPath})

	type relation struct {
		Kind     string `json:"kind"`
		FromNode string `json:"fromNode"`
		ToNode   string `json:"toNode"`
	}
	relations, status := doJSON[struct {
		Relations []relation `json:"relations"`
	}](t, ctx, http.MethodGet, catalogBaseURL+"/v1/relations", token)
	assert.Equal(t, http.StatusOK, status, "GET /v1/relations")
	assert.ElementsMatch(t, relations.Relations, []relation{
		{
			Kind:     "calls",
			FromNode: "nodes/deployment/" + wantDeplPath,
			ToNode:   "nodes/service/" + wantSvcPath,
		},
	})
}

// startK3s starts a k3s container seeded with objs (each a *corev1.Service
// or *appsv1.Deployment, carrying its own namespace), and returns a
// kubeconfig file path for that cluster plus its cluster ID (the
// kube-system namespace UID, exactly as depl_calls_svc derives it).
func startK3s(t *testing.T, ctx context.Context, objs ...runtime.Object) (kubeconfigPath, clusterID string) {
	t.Helper()

	// Trim optional components we don't need, to reduce the container's
	// resource footprint (each of these runs its own controllers, some of
	// which hold their own fsnotify watchers).
	k3sContainer, err := k3s.Run(ctx, k3sImage,
		testcontainers.WithCmdArgs("--disable=metrics-server", "--disable=servicelb", "--disable=local-storage"),
	)
	require.NoError(t, err, "starting k3s container")
	t.Cleanup(func() { _ = k3sContainer.Terminate(context.Background()) })

	kubeconfig, err := k3sContainer.GetKubeConfig(ctx)
	require.NoError(t, err, "getting k3s kubeconfig")

	kubeconfigPath = filepath.Join(t.TempDir(), "kubeconfig.yaml")
	require.NoError(t, os.WriteFile(kubeconfigPath, kubeconfig, 0o600))

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	require.NoError(t, err)
	clientset, err := kubernetes.NewForConfig(restConfig)
	require.NoError(t, err)

	ns, err := clientset.CoreV1().Namespaces().Get(ctx, "kube-system", metav1.GetOptions{})
	require.NoError(t, err, "getting kube-system namespace")
	clusterID = string(ns.UID)

	for _, obj := range objs {
		switch o := obj.(type) {
		case *corev1.Service:
			_, err = clientset.CoreV1().Services(o.Namespace).Create(ctx, o, metav1.CreateOptions{})
		case *appsv1.Deployment:
			_, err = clientset.AppsV1().Deployments(o.Namespace).Create(ctx, o, metav1.CreateOptions{})
		default:
			t.Fatalf("startK3s: unsupported object type %T", obj)
		}
		require.NoError(t, err, "creating %#v", obj)
	}

	return kubeconfigPath, clusterID
}

// buildAndStart builds pkg, trying to use the same command line used by its
// Dockerfile, then starts the resulting binary as a background process with
// extraEnv appended to the current environment. The process is killed if ctx
// is done, and unconditionally on test cleanup.
func buildAndStart(t *testing.T, ctx context.Context, pkg string, extraEnv []string) {
	t.Helper()

	out := filepath.Join(t.TempDir(), filepath.Base(pkg))
	build := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", out, pkg)
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	output, err := build.CombinedOutput()
	require.NoError(t, err, "building %s:\n%s", pkg, output)

	cmd := exec.CommandContext(ctx, out)
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start(), "starting %s", out)
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
}

func findFreePort(t *testing.T) int {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// checkTCPReady reports whether addr accepts a connection within
// dialAttemptTimeout.
func checkTCPReady(ctx context.Context, addr string) bool {
	ctx, cancel := context.WithTimeout(ctx, dialAttemptTimeout)
	defer cancel()

	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
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
	Name  string `json:"name"`
	State string `json:"state"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func requestPluginRun(t *testing.T, ctx context.Context, catalogBaseURL, token string) string {
	t.Helper()

	op, status := doJSON[apiOperation](t, ctx, http.MethodPost, catalogBaseURL+"/v1/depl_calls_svc:run", token)
	require.Equal(t, http.StatusAccepted, status, "POST /v1/depl_calls_svc:run")
	require.NotEmpty(t, op.Name)
	return op.Name
}

func operationErrorMessage(op apiOperation) string {
	if op.Error == nil {
		return ""
	}
	return op.Error.Message
}

func doJSON[T any](t require.TestingT, ctx context.Context, method, url, token string) (T, int) {
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
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&v))
	return v, resp.StatusCode
}
