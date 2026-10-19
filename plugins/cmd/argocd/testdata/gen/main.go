// Command gen scrapes real ArgoCD Application objects from an ad-hoc k3s
// cluster, saving them to JSON files for use as test data of the argocd
// plugin.
//
// It starts k3s in a container (via testcontainers), installs ArgoCD in it,
// applies the manifests from the input file, waits for ArgoCD to reconcile
// WANT_OK and WANT_FAIL Applications (see below), and writes each WANT_*
// Application as JSON into the output directory. Nondeterministic metadata is
// replaced by fixed values, so that reruns produce identical files.
//
// The input file must start with two special comments, naming the Applications
// to wait for and scrape (comma-separated):
//
//	#WANT_OK: app1, app2
//	#WANT_FAIL: app3
//
// WANT_OK Applications are expected to sync. WANT_FAIL ones are expected to
// have an invalid spec, so ArgoCD only reports error conditions for them, and
// never syncs them.
//
// The command needs Docker and Internet access, and takes several minutes.
package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
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
	"k8s.io/client-go/metadata"
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

var gvrApplications = schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}

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
	argoManifests, err := downloadArgoManifests(ctx)
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

	log.Printf("waiting for ArgoCD to reconcile Applications")
	apps, err := waitForApplications(ctx, dyn, input.wantApps)
	if err != nil {
		return fmt.Errorf("waiting for Applications: %w", err)
	}

	log.Printf("reading managed resources")
	meta, err := metadata.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("creating metadata client: %w", err)
	}
	resources, err := scrapeManagedResources(ctx, disc, meta, apps)
	if err != nil {
		return fmt.Errorf("reading managed resources: %w", err)
	}

	err = writeApplications(outDir, apps)
	if err != nil {
		return fmt.Errorf("writing Applications: %w", err)
	}
	err = writeJSON(filepath.Join(outDir, resourcesFile), resources)
	if err != nil {
		return fmt.Errorf("writing managed resources: %w", err)
	}
	return nil
}

// resourcesFile is the name of the file with managedFields of all managed resources.
const resourcesFile = "managed-fields.json"

// managedFields lists the managedFields of resources, keyed by
// "<group>/<kind>/<namespace>/<name>" (group is empty for the core API).
type managedFields map[string][]metav1.ManagedFieldsEntry

// scrapeManagedResources reads the managedFields of every resource managed by apps.
// Times are replaced by fixed ones: fixedTimestamp for ArgoCD, and a later one
// for any other field manager, so that tests can tell whether the plugin picks
// the ArgoCD entry, or just the newest one.
func scrapeManagedResources(ctx context.Context, disc discovery.DiscoveryInterface, meta metadata.Interface, apps []unstructured.Unstructured) (managedFields, error) {
	_, apis, err := disc.ServerGroupsAndResources()
	if err != nil {
		return nil, fmt.Errorf("listing resource APIs: %w", err)
	}

	result := managedFields{}
	for _, app := range apps {
		resources, _, _ := unstructured.NestedSlice(app.Object, "status", "resources")
		for _, r := range resources {
			res, _ := r.(map[string]any)
			group, _ := res["group"].(string)
			kind, _ := res["kind"].(string)
			ns, _ := res["namespace"].(string)
			name, _ := res["name"].(string)

			gvr, ok := findResource(apis, group, kind)
			if !ok {
				return nil, fmt.Errorf("no API resource for %s/%s", group, kind)
			}
			obj, err := meta.Resource(gvr).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return nil, fmt.Errorf("getting %s %s/%s: %w", kind, ns, name, err)
			}
			entries := obj.ManagedFields
			for i := range entries {
				ts := fixedTimestamp
				if entries[i].Manager != argoFieldManager {
					ts = fixedOtherManagerTimestamp
				}
				parsed, err := time.Parse(time.RFC3339, ts)
				if err != nil {
					return nil, fmt.Errorf("parsing fixed timestamp %q: %w", ts, err)
				}
				entries[i].Time = &metav1.Time{Time: parsed}
				entries[i].FieldsV1 = nil // large, and not used by the plugin
			}
			result[strings.Join([]string{group, kind, ns, name}, "/")] = entries
		}
	}
	return result, nil
}

// findResource returns the API resource of the given group and kind.
func findResource(apis []*metav1.APIResourceList, group, kind string) (schema.GroupVersionResource, bool) {
	for _, list := range apis {
		gv, err := schema.ParseGroupVersion(list.GroupVersion)
		if err != nil || gv.Group != group {
			continue
		}
		for _, res := range list.APIResources {
			if res.Kind == kind && !strings.Contains(res.Name, "/") {
				return gv.WithResource(res.Name), true
			}
		}
	}
	return schema.GroupVersionResource{}, false
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling: %w", err)
	}
	err = os.WriteFile(path, append(data, '\n'), 0o644)
	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	log.Printf("wrote %s", path)
	return nil
}

// wantApp is an Application to wait for and scrape.
type wantApp struct {
	name string
	// invalid is true for an Application with an invalid spec, which ArgoCD
	// only reports error conditions for, and never syncs.
	invalid bool
}

// input is the parsed content of the input file.
type input struct {
	manifests []*unstructured.Unstructured
	wantApps  []wantApp
}

// parseInput reads the input file: the "#WANT_OK:" and "#WANT_FAIL:" header
// comments, and the manifests. Wanted Applications need not be defined in the
// manifests, because they can be created by other Applications (app-of-apps).
func parseInput(path string) (input, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return input{}, fmt.Errorf("reading file: %w", err)
	}

	const (
		wantOKPrefix   = "#WANT_OK:"
		wantFailPrefix = "#WANT_FAIL:"
	)
	var in input
	var haveOK, haveFail bool
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, wantOKPrefix):
			haveOK = true
			in.wantApps = append(in.wantApps, parseWantApps(strings.TrimPrefix(line, wantOKPrefix), false)...)
		case strings.HasPrefix(line, wantFailPrefix):
			haveFail = true
			in.wantApps = append(in.wantApps, parseWantApps(strings.TrimPrefix(line, wantFailPrefix), true)...)
		}
		if line != "" && !strings.HasPrefix(line, "#") {
			break // the header ends at the first non-comment line
		}
	}
	if !haveOK || !haveFail {
		return input{}, errors.New(`file must start with "#WANT_OK:" and "#WANT_FAIL:" comments`)
	}

	in.manifests, err = decodeAll(data)
	if err != nil {
		return input{}, fmt.Errorf("decoding manifests: %w", err)
	}
	return in, nil
}

// parseWantApps splits a comma-separated list of Application names.
func parseWantApps(list string, invalid bool) []wantApp {
	var apps []wantApp
	for _, name := range strings.Split(list, ",") {
		name = strings.TrimSpace(name)
		if name != "" {
			apps = append(apps, wantApp{name: name, invalid: invalid})
		}
	}
	return apps
}

// downloadArgoManifests downloads ArgoCD "core" installation manifests.
func downloadArgoManifests(ctx context.Context) ([]*unstructured.Unstructured, error) {
	url := "https://raw.githubusercontent.com/argoproj/argo-cd/" + argoVersion + "/manifests/core-install.yaml"
	body, err := httpGet(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("downloading ArgoCD manifests: %w", err)
	}
	core, err := decodeAll(body)
	if err != nil {
		return nil, fmt.Errorf("decoding %s: %w", url, err)
	}
	return core, nil
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

func decodeAll(data []byte) ([]*unstructured.Unstructured, error) {
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

// waitForApplications returns all wantApps once ArgoCD has reconciled them.
func waitForApplications(ctx context.Context, dyn dynamic.Interface, wantApps []wantApp) ([]unstructured.Unstructured, error) {
	for {
		list, err := dyn.Resource(gvrApplications).Namespace(argoNamespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("listing Applications: %w", err)
		}
		var apps []unstructured.Unstructured
		var notReady []string
		for _, want := range wantApps {
			i := slices.IndexFunc(list.Items, func(u unstructured.Unstructured) bool { return u.GetName() == want.name })
			if i < 0 || !reconciled(list.Items[i], want.invalid) {
				notReady = append(notReady, want.name)
				continue
			}
			apps = append(apps, list.Items[i])
		}
		if len(notReady) == 0 {
			return apps, nil
		}
		log.Printf("Applications not ready yet: %v", notReady)

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("Applications %v not ready: %w", notReady, context.Cause(ctx))
		case <-time.After(pollInterval):
		}
	}
}

// reconciled returns true if ArgoCD finished the first reconciliation of app.
// An invalid Application is considered reconciled once it has error conditions.
func reconciled(app unstructured.Unstructured, invalid bool) bool {
	if invalid {
		conditions, _, _ := unstructured.NestedSlice(app.Object, "status", "conditions")
		return len(conditions) > 0
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

// writeApplications writes apps as "*.json" files, replacing any such files
// already present in dir.
func writeApplications(dir string, apps []unstructured.Unstructured) error {
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

// fixedOtherManagerTimestamp is used for managedFields of managers other than ArgoCD.
const fixedOtherManagerTimestamp = "2000-01-02T00:00:00Z"

// argoFieldManager is the field manager ArgoCD applies resources as.
const argoFieldManager = "argocd-controller"

var timestampRe = regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$`)

// normalize replaces nondeterministic values in app with fixed ones, so that
// scraping the same cluster setup again yields identical output.
//
// The metadata is normalized explicitly (UID is derived from the name to stay
// unique), all remaining timestamps anywhere in the object are replaced.
// status.operationState is reduced to its finishedAt, because the rest records
// the last sync operation, which depends on timing (e.g. whether an automated
// sync was retried).
func normalize(app *unstructured.Unstructured) {
	unstructured.RemoveNestedField(app.Object, "metadata", "managedFields")
	finishedAt, found, _ := unstructured.NestedString(app.Object, "status", "operationState", "finishedAt")
	unstructured.RemoveNestedField(app.Object, "status", "operationState")
	if found && finishedAt != "" {
		_ = unstructured.SetNestedField(app.Object, finishedAt, "status", "operationState", "finishedAt")
	}
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
