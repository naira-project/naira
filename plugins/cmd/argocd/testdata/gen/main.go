// Command gen scrapes real ArgoCD Application objects from the REST API of
// argocd-server, running in an ad-hoc k3s cluster, saving them to JSON files
// for use as test data of the argocd plugin.
//
// It starts k3s in a container (via testcontainers), installs ArgoCD in it,
// applies the manifests from the input file, waits for ArgoCD to reconcile
// the Applications named by WANT_* comments (see below), and writes each
// Application, as returned by the API, as JSON into the output directory.
// The API is reached through the Kubernetes API server's proxy of the
// argocd-server Service, and authenticated as the "admin" user. Nondeterministic metadata is
// replaced by fixed values, so that reruns produce identical files.
//
// The input file must start with special comments, naming the Applications
// to wait for and scrape (comma-separated):
//
//	#WANT_OK: app1, app2
//	#WANT_RUNNING: app3
//	#WANT_FAIL: app4
//
// WANT_OK Applications are expected to sync. WANT_RUNNING ones are expected to
// be stuck in the middle of a sync (operation phase Running), which is scraped
// as is. WANT_FAIL ones are expected to have an invalid spec, so ArgoCD only
// reports error conditions for them, and never syncs them.
//
// The command needs Docker and Internet access, and takes several minutes.
// If k3s never becomes ready and its logs show "too many open files" from
// fsnotify, raise the host's inotify limit, e.g.:
//
//	sudo sysctl -w fs.inotify.max_user_instances=1024
package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/naira-project/naira/plugins/pkg/httpjson"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/k3s"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	k3sImage      = "rancher/k3s:v1.32.1-k3s1"
	argoVersion   = "v3.5.5"
	argoNamespace = "argocd"

	runTimeout   = 20 * time.Minute
	pollInterval = 5 * time.Second
)

var gvrSecrets = schema.GroupVersionResource{Version: "v1", Resource: "secrets"}

func main() {
	in := flag.String("in", "testdata/gen/input.yaml", "manifests to apply on top of the ArgoCD installation")
	out := flag.String("out", "testdata", "directory to write Application JSON files into")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()

	err := run(ctx, *in, *out)
	if err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, inFile, outDir string) error {
	input, err := parseInput(inFile)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", inFile, err)
	}

	log.Printf("starting k3s %s", k3sImage)
	k3sContainer, err := k3s.Run(ctx, k3sImage,
		testcontainers.WithCmdArgs(
			"--disable=metrics-server",
			"--disable=servicelb",
			"--disable=local-storage",
		),
	)
	if err != nil {
		return fmt.Errorf("starting k3s container: %w", err)
	}
	defer func() { _ = k3sContainer.Terminate(context.Background()) }()

	kubeconfig, err := k3sContainer.GetKubeConfig(ctx)
	if err != nil {
		return fmt.Errorf("getting k3s kubeconfig: %w", err)
	}
	cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("parsing k3s kubeconfig: %w", err)
	}
	disc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return fmt.Errorf("creating discovery client: %w", err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("creating dynamic client: %w", err)
	}

	log.Printf("installing ArgoCD %s", argoVersion)
	argoManifests, err := downloadYAMLObjects(ctx,
		"https://raw.githubusercontent.com/argoproj/argo-cd/"+argoVersion+"/manifests/install.yaml")
	if err != nil {
		return fmt.Errorf("loading manifests: %w", err)
	}
	err = applyAll(ctx, disc, dyn, slices.Concat(
		[]*unstructured.Unstructured{{Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]any{
				"name": argoNamespace,
			},
		}}},
		argoManifests,
		input.manifests,
	))
	if err != nil {
		return fmt.Errorf("applying manifests: %w", err)
	}

	log.Printf("logging in to argocd-server")
	api, err := newAPIClient(ctx, cfg, dyn)
	if err != nil {
		return fmt.Errorf("connecting to argocd-server: %w", err)
	}

	log.Printf("waiting for ArgoCD to reconcile Applications")
	apps, err := waitForApplications(ctx, api, input.wantApps)
	if err != nil {
		return fmt.Errorf("waiting for Applications: %w", err)
	}

	err = writeJSONFiles(outDir, apps)
	if err != nil {
		return fmt.Errorf("writing Applications: %w", err)
	}
	return nil
}

// expectation is the state of an Application to wait for before scraping it.
type expectation int

const (
	// expectSynced is an Application that completed a sync.
	expectSynced expectation = iota
	// expectRunning is an Application stuck in the middle of a sync.
	expectRunning
	// expectInvalid is an Application with an invalid spec, which ArgoCD only
	// reports error conditions for, and never syncs.
	expectInvalid
)

// wantApp is an Application to wait for and scrape.
type wantApp struct {
	name   string
	expect expectation
}

// input is the parsed content of the input file.
type input struct {
	manifests []*unstructured.Unstructured
	wantApps  []wantApp
}

// parseInput reads the input file: the "#WANT_OK:", "#WANT_RUNNING:" and
// "#WANT_FAIL:" header comments, and the manifests.
func parseInput(path string) (input, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return input{}, fmt.Errorf("reading file: %w", err)
	}

	headers := []struct {
		prefix string
		expect expectation
	}{
		{prefix: "#WANT_OK:", expect: expectSynced},
		{prefix: "#WANT_RUNNING:", expect: expectRunning},
		{prefix: "#WANT_FAIL:", expect: expectInvalid},
	}

	const (
		wantOKPrefix      = "#WANT_OK:"
		wantRunningPrefix = "#WANT_RUNNING:"
		wantFailPrefix    = "#WANT_FAIL:"
	)
	var result input
lines:
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		for _, h := range headers {
			if strings.HasPrefix(line, h.prefix) {
				result.wantApps = append(result.wantApps, parseWantApps(strings.TrimPrefix(line, h.prefix), h.expect)...)
				continue lines
			}
		}

		if line != "" && !strings.HasPrefix(line, "#") {
			break // the header ends at the first non-comment line
		}
	}

	result.manifests, err = decodeYAMLObjects(data)
	if err != nil {
		return input{}, fmt.Errorf("decoding manifests: %w", err)
	}
	return result, nil
}

// parseWantApps splits a comma-separated list of Application names.
func parseWantApps(list string, expect expectation) []wantApp {
	var apps []wantApp
	for _, name := range strings.Split(list, ",") {
		name = strings.TrimSpace(name)
		if name != "" {
			apps = append(apps, wantApp{name: name, expect: expect})
		}
	}
	return apps
}

// downloadYAMLObjects downloads and decodes YAML objects from yamlURL.
func downloadYAMLObjects(ctx context.Context, yamlURL string) ([]*unstructured.Unstructured, error) {
	body, err := httpGet(ctx, yamlURL)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", yamlURL, err)
	}
	objs, err := decodeYAMLObjects(body)
	if err != nil {
		return nil, fmt.Errorf("decoding %s: %w", yamlURL, err)
	}
	return objs, nil
}

// httpGet returns the body of a successful GET response from url.
func httpGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request for %s: %w", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("requesting %s: unexpected status %s", url, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", url, err)
	}
	return body, nil
}

func decodeYAMLObjects(data []byte) ([]*unstructured.Unstructured, error) {
	var objs []*unstructured.Unstructured
	dec := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	for {
		var obj map[string]any
		err := dec.Decode(&obj)
		if err == io.EOF {
			return objs, nil
		}
		if err != nil {
			return nil, fmt.Errorf("decoding YAML document: %w", err)
		}
		if len(obj) == 0 {
			continue
		}
		objs = append(objs, &unstructured.Unstructured{Object: obj})
	}
}

// applyAll server-side applies objs. Objects whose kind is not served yet
// (e.g. waiting for a CRD to be established) are retried.
func applyAll(ctx context.Context, disc discovery.DiscoveryInterface, dyn dynamic.Interface, objs []*unstructured.Unstructured) error {
	cachedDisc := memory.NewMemCacheClient(disc)
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(cachedDisc)

	pending := objs
	for {
		var retry []*unstructured.Unstructured
		var lastErr error
		for _, obj := range pending {
			err := apply(ctx, mapper, dyn, obj)
			if meta.IsNoMatchError(err) {
				retry = append(retry, obj)
				lastErr = err
				continue
			}
			if err != nil {
				return fmt.Errorf("applying %s %q: %w", obj.GetKind(), obj.GetName(), err)
			}
		}

		pending = retry
		if len(pending) == 0 {
			return nil
		}

		log.Printf("%d objects waiting for their CRDs: %v", len(pending), lastErr)
		mapper.Reset()
		select {
		case <-ctx.Done():
			return fmt.Errorf("%d objects not applied: %w", len(pending), context.Cause(ctx))
		case <-time.After(2 * time.Second):
		}
	}
}

func apply(ctx context.Context, mapper meta.RESTMapper, dyn dynamic.Interface, obj *unstructured.Unstructured) error {
	gvk := obj.GroupVersionKind()
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return fmt.Errorf("mapping %s: %w", gvk, err)
	}
	var ri dynamic.ResourceInterface = dyn.Resource(mapping.Resource)
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		if obj.GetNamespace() == "" {
			obj.SetNamespace(argoNamespace)
		}
		ri = dyn.Resource(mapping.Resource).Namespace(obj.GetNamespace())
	}
	data, err := json.Marshal(obj.Object)
	if err != nil {
		return fmt.Errorf("marshaling %s: %w", gvk, err)
	}
	force := true
	_, err = ri.Patch(ctx, obj.GetName(), types.ApplyPatchType, data, metav1.PatchOptions{
		FieldManager: "argocd-testdata-gen",
		Force:        &force,
	})
	if err != nil {
		return fmt.Errorf("patching %s: %w", gvk, err)
	}
	return nil
}

// apiClient calls the argocd-server REST API, through the proxy of the
// Kubernetes API server.
type apiClient struct {
	http    *http.Client
	baseURL string
	token   string
}

// newAPIClient waits for argocd-server to come up, and logs in as "admin"
// with the initial password generated by ArgoCD.
func newAPIClient(ctx context.Context, cfg *rest.Config, dyn dynamic.Interface) (*apiClient, error) {
	httpClient, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating HTTP client for k8s API: %w", err)
	}
	c := &apiClient{
		http: httpClient,
		// "https:" asks the proxy to talk TLS to argocd-server. The proxy
		// doesn't verify its self-signed certificate.
		baseURL: strings.TrimRight(cfg.Host, "/") +
			"/api/v1/namespaces/" + argoNamespace + "/services/https:argocd-server:https/proxy",
	}
	for {
		err := c.login(ctx, dyn)
		if err == nil {
			return c, nil
		}
		log.Printf("argocd-server not ready yet: %v", err)

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("logging in: %w", context.Cause(ctx))
		case <-time.After(pollInterval):
		}
	}
}

func (c *apiClient) login(ctx context.Context, dyn dynamic.Interface) error {
	secret, err := dyn.Resource(gvrSecrets).Namespace(argoNamespace).Get(ctx, "argocd-initial-admin-secret", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting initial admin secret: %w", err)
	}
	encoded, _, _ := unstructured.NestedString(secret.Object, "data", "password")
	password, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("decoding initial admin password: %w", err)
	}

	var session struct {
		Token string `json:"token"`
	}
	err = httpjson.Post(ctx, c.http, c.baseURL+"/api/v1/session",
		map[string]string{"username": "admin", "password": string(password)}, &session)
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	c.token = session.Token
	return nil
}

// getApplications returns the Applications as returned by the API, wrapped in unstructured.Unstructured objects to slightly mimic the k8s API results and provide some helper utility methods.
func (c *apiClient) getApplications(ctx context.Context) ([]unstructured.Unstructured, error) {
	var list struct {
		Items []map[string]any `json:"items"`
	}
	// The k8s API proxy would consume an Authorization header itself, so the
	// token is sent in the cookie that argocd-server also accepts.
	err := httpjson.Get(ctx, c.http, c.baseURL+"/api/v1/applications", &list,
		httpjson.WithHeaders{"Cookie": "argocd.token=" + c.token})
	if err != nil {
		return nil, fmt.Errorf("calling argocd-server API: %w", err)
	}
	apps := make([]unstructured.Unstructured, 0, len(list.Items))
	for _, item := range list.Items {
		apps = append(apps, unstructured.Unstructured{Object: item})
	}
	return apps, nil
}

// waitForApplications returns all wantApps once ArgoCD has reconciled them.
func waitForApplications(ctx context.Context, api *apiClient, wantApps []wantApp) ([]unstructured.Unstructured, error) {
	for {
		apps, err := api.getApplications(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing Applications: %w", err)
		}
		var ready []unstructured.Unstructured
		var notReady []string
		for _, want := range wantApps {
			i := slices.IndexFunc(apps, func(u unstructured.Unstructured) bool { return u.GetName() == want.name })
			if i < 0 || !isReady(apps[i], want.expect) {
				notReady = append(notReady, want.name)
				continue
			}
			ready = append(ready, apps[i])
		}
		if len(notReady) == 0 {
			return ready, nil
		}
		log.Printf("Applications not ready yet: %v", notReady)

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("Applications %v not ready: %w", notReady, context.Cause(ctx))
		case <-time.After(pollInterval):
		}
	}
}

// isReady returns true if ArgoCD reached the state of app described by expect.
// An invalid Application is considered ready once it has error conditions.
// A running one, once its sync operation is in the Running phase.
func isReady(app unstructured.Unstructured, expect expectation) bool {
	switch expect {
	case expectInvalid:
		conditions, _, _ := unstructured.NestedSlice(app.Object, "status", "conditions")
		return len(conditions) > 0
	case expectRunning:
		phase, _, _ := unstructured.NestedString(app.Object, "status", "operationState", "phase")
		return phase == "Running"
	}
	var (
		syncStatus, _, _   = unstructured.NestedString(app.Object, "status", "sync", "status")
		healthStatus, _, _ = unstructured.NestedString(app.Object, "status", "health", "status")
		resources, _, _    = unstructured.NestedSlice(app.Object, "status", "resources")
		sourceType, _, _   = unstructured.NestedString(app.Object, "status", "sourceType")
		sourceTypes, _, _  = unstructured.NestedStringSlice(app.Object, "status", "sourceTypes")
	)
	return syncStatus == "Synced" && healthStatus != "" && len(resources) > 0 &&
		(sourceType != "" || len(sourceTypes) > 0)
}

// writeJSONFiles writes apps as "*.json" files, replacing any such files
// already present in dir. Known nondeterministic values in the Application
// objects are normalized (replaced by fixed values) or removed, so that reruns
// of the app produce identical output.
func writeJSONFiles(dir string, apps []unstructured.Unstructured) error {
	err := os.MkdirAll(dir, 0o755)
	if err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	stale, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return fmt.Errorf("listing stale files in %s: %w", dir, err)
	}
	for _, f := range stale {
		log.Printf("removing stale file: %s", f)
		err = os.Remove(f)
		if err != nil {
			return fmt.Errorf("removing %s: %w", f, err)
		}
	}

	for _, app := range apps {
		normalize(&app)
		data, err := json.MarshalIndent(app.Object, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling Application %q: %w", app.GetName(), err)
		}
		path := filepath.Join(dir, app.GetName()+".json")
		err = os.WriteFile(path, append(data, '\n'), 0o644)
		if err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
		log.Printf("wrote %s", path)
	}
	return nil
}

// fixedTimestamp for replacing all timestamps in the written files.
const fixedTimestamp = "2000-01-01T00:00:00Z"

var timestampRe = regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$`)

// normalize replaces nondeterministic values in app with fixed ones, or
// removes them, so that scraping the same cluster setup again yields identical
// output.
func normalize(app *unstructured.Unstructured) {
	unstructured.RemoveNestedField(app.Object, "metadata", "managedFields")

	// In status.operationState, only the fields we need for the plugin are
	// retained (phase, startedAt and finishedAt, if set). Most of the fields
	// record details of the sync operation, which may depend on timing (such
	// as whether a sync was retried).
	opState, _, _ := unstructured.NestedMap(app.Object, "status", "operationState")
	unstructured.RemoveNestedField(app.Object, "status", "operationState")
	for _, field := range []string{"phase", "startedAt", "finishedAt"} {
		if v, ok := opState[field].(string); ok && v != "" {
			_ = unstructured.SetNestedField(app.Object, v, "status", "operationState", field)
		}
	}

	// UID is replaced by a fixed value - derived from the name to stay unique
	app.SetUID(uidFromHashed(app.GetNamespace() + "/" + app.GetName()))

	app.SetResourceVersion("1")
	app.SetGeneration(1)
	app.SetCreationTimestamp(metav1.Time{})
	_ = unstructured.SetNestedField(app.Object, fixedTimestamp, "metadata", "creationTimestamp")

	replaceTimestamps(app.Object)
}

// uidFromHashed returns a deterministic UID-formatted hash of s.
func uidFromHashed(s string) types.UID {
	sum := sha1.Sum([]byte(s))
	return types.UID(fmt.Sprintf("%x-%x-%x-%x-%x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16]))
}

func replaceTimestamps(v any) {
	switch v := v.(type) {
	case map[string]any:
		for k, e := range v {
			if str, ok := e.(string); ok && timestampRe.MatchString(str) {
				v[k] = fixedTimestamp
				continue
			}
			replaceTimestamps(e)
		}
	case []any:
		for i, e := range v {
			if str, ok := e.(string); ok && timestampRe.MatchString(str) {
				v[i] = fixedTimestamp
				continue
			}
			replaceTimestamps(e)
		}
	}
}
