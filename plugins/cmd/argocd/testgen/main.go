// Command testgen scrapes real ArgoCD Application objects, to be used as test data
// of the argocd plugin.
//
// It starts k3s in a container (via testcontainers), installs ArgoCD in it,
// creates a few Applications deploying from a pinned revision of
// https://github.com/argoproj/argocd-example-apps, waits for ArgoCD to
// reconcile them, and writes each Application as JSON into the output directory,
// with nondeterministic metadata replaced by fixed values, so that reruns produce
// identical files.
// It needs Docker and Internet access, and takes several minutes.
package main

import (
	"bytes"
	"context"
	"crypto/sha1"
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
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	k3sImage      = "rancher/k3s:v1.32.1-k3s1"
	argoVersion   = "v3.5.5"
	argoNamespace = "argocd"

	runTimeout   = 20 * time.Minute
	pollInterval = 5 * time.Second

	// Name of the Application that is not expected to ever sync.
	remoteApp = "remote-cluster"
)

var gvrApplications = schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}

// Manifests applied on top of the ArgoCD installation.
var extraManifests = []string{
	`
apiVersion: v1
kind: Namespace
metadata:
  name: argocd
`,
	`
apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: default
  namespace: argocd
spec:
  sourceRepos: ["*"]
  destinations:
  - {server: "*", namespace: "*"}
  clusterResourceWhitelist:
  - {group: "*", kind: "*"}
`,

	// The "apps-parent" Application deploys a Helm chart creating the
	// "example.guestbook" child Application (app-of-apps).
	`
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: apps-parent
  namespace: argocd
spec:
  project: default
  destination:
    server: https://kubernetes.default.svc
    namespace: argocd
  source:
    repoURL: https://github.com/argoproj/argocd-example-apps
    targetRevision: 8088f4c0d970abb09e250248cc97e35623447cb5
    path: apps
    helm:
      valuesObject:
        config:
          spec:
            source:
              targetRevision: 8088f4c0d970abb09e250248cc97e35623447cb5
        applications:
        - name: guestbook
          destination:
            namespace: guestbook
  syncPolicy:
    automated: {prune: true, selfHeal: true}
`,
	`
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: helm-guestbook
  namespace: argocd
spec:
  project: default
  destination:
    server: https://kubernetes.default.svc
    namespace: helm-guestbook
  source:
    repoURL: https://github.com/argoproj/argocd-example-apps
    targetRevision: 8088f4c0d970abb09e250248cc97e35623447cb5
    path: helm-guestbook
  syncPolicy:
    automated: {prune: true, selfHeal: true}
    syncOptions: [CreateNamespace=true]
`,
	`
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: kustomize-guestbook
  namespace: argocd
spec:
  project: default
  destination:
    server: https://kubernetes.default.svc
    namespace: kustomize-guestbook
  source:
    repoURL: https://github.com/argoproj/argocd-example-apps
    targetRevision: 8088f4c0d970abb09e250248cc97e35623447cb5
    path: kustomize-guestbook
  syncPolicy:
    automated: {prune: true, selfHeal: true}
    syncOptions: [CreateNamespace=true]
`,
	`
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: multi-source
  namespace: argocd
spec:
  project: default
  destination:
    server: https://kubernetes.default.svc
    namespace: multi-source
  sources:
  - repoURL: https://github.com/argoproj/argocd-example-apps
    targetRevision: 8088f4c0d970abb09e250248cc97e35623447cb5
    path: helm-guestbook
    helm:
      valueFiles:
      - $values/helm-guestbook/values-production.yaml
  - repoURL: https://github.com/argoproj/argocd-example-apps
    targetRevision: 8088f4c0d970abb09e250248cc97e35623447cb5
    ref: values
  syncPolicy:
    automated: {prune: true, selfHeal: true}
    syncOptions: [CreateNamespace=true]
`,
	`
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: remote-cluster
  namespace: argocd
spec:
  project: default
  destination:
    server: https://other.example.com
    namespace: default
  source:
    repoURL: https://github.com/argoproj/argocd-example-apps
    targetRevision: 8088f4c0d970abb09e250248cc97e35623447cb5
    path: guestbook
`,
}

// Names of Applications to wait for and scrape.
var wantApps = []string{
	"apps-parent",
	"example.guestbook",
	"helm-guestbook",
	"kustomize-guestbook",
	"multi-source",
	remoteApp,
}

func main() {
	out := flag.String("out", "testdata", "directory to write Application JSON files into")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()

	err := run(ctx, *out)
	if err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, outDir string) error {
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
	objs, err := loadManifests(ctx)
	if err != nil {
		return fmt.Errorf("loading manifests: %w", err)
	}
	err = applyAll(ctx, disc, dyn, objs)
	if err != nil {
		return fmt.Errorf("applying manifests: %w", err)
	}

	log.Printf("waiting for ArgoCD to reconcile Applications")
	apps, err := waitForApplications(ctx, dyn)
	if err != nil {
		return fmt.Errorf("waiting for Applications: %w", err)
	}

	err = writeApplications(outDir, apps)
	if err != nil {
		return fmt.Errorf("writing Applications: %w", err)
	}
	return nil
}

// loadManifests returns the extra manifests, followed by the ArgoCD "core"
// installation manifests. The latter lack the namespace, thus it is set later.
func loadManifests(ctx context.Context) ([]*unstructured.Unstructured, error) {
	var objs []*unstructured.Unstructured
	for _, m := range extraManifests {
		docs, err := decodeAll([]byte(m))
		if err != nil {
			return nil, fmt.Errorf("decoding extra manifest: %w", err)
		}
		objs = append(objs, docs...)
	}

	url := "https://raw.githubusercontent.com/argoproj/argo-cd/" + argoVersion + "/manifests/core-install.yaml"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request for %s: %w", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s: unexpected status %s", url, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", url, err)
	}
	docs, err := decodeAll(body)
	if err != nil {
		return nil, fmt.Errorf("decoding %s: %w", url, err)
	}
	// Extra manifests go last when applied, because they need CRDs. They were
	// added first above only to keep the Namespace before everything else.
	return append(objs[:1:1], append(docs, objs[1:]...)...), nil
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
	for ctx.Err() == nil && len(pending) > 0 {
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
		if len(pending) > 0 {
			log.Printf("%d objects waiting for their CRDs: %v", len(pending), lastErr)
			mapper.Reset()
			select {
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
			}
		}
	}
	if len(pending) > 0 {
		return fmt.Errorf("%d objects not applied: %w", len(pending), context.Cause(ctx))
	}
	return nil
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
		FieldManager: "argocd-testgen",
		Force:        &force,
	})
	if err != nil {
		return fmt.Errorf("patching %s: %w", gvk, err)
	}
	return nil
}

// waitForApplications returns all wantApps once ArgoCD has reconciled them.
func waitForApplications(ctx context.Context, dyn dynamic.Interface) ([]unstructured.Unstructured, error) {
	for {
		list, err := dyn.Resource(gvrApplications).Namespace(argoNamespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("listing Applications: %w", err)
		}
		var apps []unstructured.Unstructured
		var notReady []string
		for _, name := range wantApps {
			i := slices.IndexFunc(list.Items, func(u unstructured.Unstructured) bool { return u.GetName() == name })
			if i < 0 || !reconciled(list.Items[i]) {
				notReady = append(notReady, name)
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
// The remoteApp has an invalid destination, so it only gets error conditions.
func reconciled(app unstructured.Unstructured) bool {
	conditions, _, _ := unstructured.NestedSlice(app.Object, "status", "conditions")
	if app.GetName() == remoteApp {
		return len(conditions) > 0
	}
	var (
		syncStatus, _, _  = unstructured.NestedString(app.Object, "status", "sync", "status")
		health, _, _      = unstructured.NestedString(app.Object, "status", "health", "status")
		resources, _, _   = unstructured.NestedSlice(app.Object, "status", "resources")
		sourceType, _, _  = unstructured.NestedString(app.Object, "status", "sourceType")
		sourceTypes, _, _ = unstructured.NestedStringSlice(app.Object, "status", "sourceTypes")
	)
	return syncStatus == "Synced" && health != "" && len(resources) > 0 &&
		(sourceType != "" || len(sourceTypes) > 0)
}

// writeApplications writes apps as "*.json" files, replacing
// any such files already present in dir.
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

// fixedTimestamp replaces all timestamps in the written files.
const fixedTimestamp = "2000-01-01T00:00:00Z"

var timestampRe = regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$`)

// normalize replaces nondeterministic values in app with fixed ones, so that
// scraping the same cluster setup again yields identical output.
//
// The metadata is normalized explicitly (UID is derived from the name to stay
// unique), all remaining timestamps anywhere in the object are replaced.
// status.operationState is dropped, because it records the last sync operation,
// which depends on timing (e.g. whether an automated sync was retried).
func normalize(app *unstructured.Unstructured) {
	unstructured.RemoveNestedField(app.Object, "metadata", "managedFields")
	unstructured.RemoveNestedField(app.Object, "status", "operationState")
	sum := sha1.Sum([]byte(app.GetNamespace() + "/" + app.GetName()))
	app.SetUID(types.UID(fmt.Sprintf("%x-%x-%x-%x-%x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])))
	app.SetResourceVersion("1")
	app.SetGeneration(1)
	app.SetCreationTimestamp(metav1.Time{})
	_ = unstructured.SetNestedField(app.Object, fixedTimestamp, "metadata", "creationTimestamp")
	replaceTimestamps(app.Object)
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
