// langfuse plugin reads aggregate LLM metrics — cost, latency and user
// feedback scores — from Langfuse projects and attaches them to applications
// the catalog already knows about. It links out to Langfuse for the detail.
//
// # Configuration
//
// Projects, credentials and their bindings to catalog nodes live in a YAML
// file, re-read on every collect. Each binding names the observed workload by
// namespace and name; the cluster id completing its node path is resolved from
// the cluster. The file holds API secrets, so mount it from a Secret. See
// sample_config.yaml for the schema.
//
// # Metrics
//
// Each collect queries GET /api/public/v2/metrics per scope: once for cost and
// latency, once for the configured score. A scope is a project, optionally
// narrowed to one trace name so applications sharing a project stay distinct.
//
// A failing project re-emits its last good values marked stale rather than
// failing the collect, which would discard every other project's metrics too.
//
// # Environment Variables
//
//   - LANGFUSE_CONFIG_PATH (optional) - path to the YAML file described above;
//     defaults to /etc/naira/langfuse/config.yaml.
//
//   - LANGFUSE_HTTP_TIMEOUT (optional) - per-request timeout for Langfuse API
//     calls; defaults to 10s.
//
//   - LANGFUSE_KUBECONFIG (optional) - path to a kubeconfig used to resolve the
//     cluster id that catalog node paths are built from; defaults to the
//     in-cluster config.
//
//go:generate bash -c "goreadme -use-stdlib-markdown -title 'langfuse plugin' | sed 's/ {#hdr-[^}]*}//g' > README.md"
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"k8s.io/client-go/dynamic"

	"github.com/naira-project/naira/plugins/internal/kubeutil"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/naira-project/naira/plugins/pkg/pluginmain"
)

type config struct {
	ConfigPath  string        `env:"LANGFUSE_CONFIG_PATH" default:"/etc/naira/langfuse/config.yaml" usage:"path to the Langfuse YAML config, re-read on every collect"`
	HTTPTimeout time.Duration `env:"LANGFUSE_HTTP_TIMEOUT" default:"10s" usage:"per-request timeout for Langfuse API calls"`
	Kubeconfig  string        `env:"LANGFUSE_KUBECONFIG" usage:"kubeconfig used to resolve the cluster id; empty means in-cluster"`
}

type Plugin struct {
	config   config
	client   *client
	logger   *log.Logger
	lastGood *lastGood

	// now is overridden in tests so window bounds are predictable.
	now func() time.Time

	// lookupClusterID is overridden in tests to avoid needing a cluster.
	lookupClusterID func(context.Context) (string, error)

	clusterMu sync.Mutex
	clusterID string
}

func New(config config, logger *log.Logger) (*Plugin, error) {
	if config.ConfigPath == "" {
		return nil, fmt.Errorf("no config file configured: LANGFUSE_CONFIG_PATH is empty")
	}

	return &Plugin{
		config:          config,
		client:          newClient(config.HTTPTimeout),
		logger:          logger,
		lastGood:        newLastGood(),
		now:             time.Now,
		lookupClusterID: clusterIDFromCluster(config.Kubeconfig),
	}, nil
}

// resolveClusterID returns the cluster id, looking it up at most once. A live
// cluster cannot change its kube-system UID, but a failed lookup is not cached
// or the plugin would never recover from a transient API error.
func (p *Plugin) resolveClusterID(ctx context.Context) (string, error) {
	p.clusterMu.Lock()
	defer p.clusterMu.Unlock()

	if p.clusterID != "" {
		return p.clusterID, nil
	}

	id, err := p.lookupClusterID(ctx)
	if err != nil {
		return "", err //nolint // the lookup wraps; Collect names the operation
	}
	p.clusterID = id
	return id, nil
}

func clusterIDFromCluster(kubeconfig string) func(context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		cfg, err := kubeutil.RestConfig(kubeconfig)
		if err != nil {
			return "", fmt.Errorf("loading k8s config: %w", err)
		}
		dyn, err := dynamic.NewForConfig(cfg)
		if err != nil {
			return "", fmt.Errorf("creating k8s dynamic client: %w", err)
		}
		_, clusterID, err := kubeutil.NamespacesAndClusterIDFromDynamic(ctx, dyn)
		if err != nil {
			return "", err //nolint // kubeutil names the failing call; Collect names the operation
		}
		return clusterID, nil
	}
}

func main() {
	app := pluginmain.New[config]()
	p, err := New(app.PluginConfig, app.Logger)
	if err != nil {
		log.Fatalf("failed to initialize plugin: %v", err)
	}
	app.Serve(p)
}

func (p *Plugin) Collect(ctx context.Context) (pluginapi.CollectResponse, error) {
	data, err := os.ReadFile(p.config.ConfigPath)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("reading langfuse config %q: %w", p.config.ConfigPath, err)
	}

	cfg, err := parseConfig(data)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("loading langfuse config %q: %w", p.config.ConfigPath, err)
	}

	to := p.now().UTC()

	// Only bindings need the cluster id, so an unbound config collects without
	// cluster access at all.
	var clusterID string
	for _, project := range cfg.Projects {
		if len(project.Observes) == 0 {
			continue
		}
		clusterID, err = p.resolveClusterID(ctx)
		if err != nil {
			return pluginapi.CollectResponse{}, fmt.Errorf("resolving cluster id: %w", err)
		}
		break
	}

	var (
		nodes     []pluginapi.NodeClaim
		relations []pluginapi.RelationClaim
	)

	for _, project := range cfg.Projects {
		from := to.Add(-project.window)

		for _, traceName := range project.scopes() {
			nodes = append(nodes, pluginapi.NodeClaim{
				ID:         scopeNodeID(project, traceName),
				Properties: p.collectScope(ctx, project, traceName, from, to),
			})
			relations = append(relations, relationsFor(clusterID, project, traceName)...)
		}

		// Relations may only reach nodes the same snapshot declares, so each
		// bound app is claimed bare. Claims are per-plugin, so carrying no
		// properties leaves the owning plugin's untouched.
		for _, app := range project.Observes {
			nodes = append(nodes, pluginapi.NodeClaim{
				ID: appNodeID(clusterID, app),
			})
		}
	}

	return pluginapi.CollectResponse{Nodes: dedupeNodes(nodes), Relations: relations}, nil
}

// collectScope reads one scope's metrics, degrading to the last good values
// rather than failing the whole run. See the package comment.
func (p *Plugin) collectScope(
	ctx context.Context,
	project projectConfig,
	traceName string,
	from, to time.Time,
) pluginapi.PropertyMap {
	observations, err := p.client.fetchObservations(ctx, project, traceName, from, to)
	if err != nil {
		err = fmt.Errorf("reading metrics for project %q scope %q: %w", project.ID, traceName, err)
		p.logf("WARN: %v", err)
		return p.lastGood.degrade(project, traceName, err, to)
	}

	var scores scoreMetrics
	if project.ScoreName != "" {
		scores, err = p.client.fetchScores(ctx, project, traceName, from, to)
		if err != nil {
			// Cost and latency already landed; marking the scope stale
			// would discard them. Leave the score unset instead.
			p.logf("WARN: reading score %q for project %q scope %q: %v", project.ScoreName, project.ID, traceName, err)
			scores = scoreMetrics{}
		}
	}

	properties := measuredProperties(project, traceName, observations, scores, from, to)
	p.lastGood.record(scopeNodeID(project, traceName).Path, properties)
	return properties
}

// dedupeNodes keeps the last claim for each node id. Bare app claims are
// emitted once per binding, and several projects may observe the same app.
func dedupeNodes(nodes []pluginapi.NodeClaim) []pluginapi.NodeClaim {
	unique := make(map[pluginapi.NodeID]pluginapi.NodeClaim, len(nodes))
	order := make([]pluginapi.NodeID, 0, len(nodes))

	for _, node := range nodes {
		if _, seen := unique[node.ID]; !seen {
			order = append(order, node.ID)
		}
		// A measured claim must win over a bare app claim for the same id.
		if existing, seen := unique[node.ID]; seen && len(existing.Properties) > len(node.Properties) {
			continue
		}
		unique[node.ID] = node
	}

	result := make([]pluginapi.NodeClaim, 0, len(unique))
	for _, id := range order {
		result = append(result, unique[id])
	}
	return result
}

func (p *Plugin) logf(format string, v ...any) {
	if p.logger != nil {
		p.logger.Printf(format, v...)
	}
}