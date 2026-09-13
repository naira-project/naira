# Naira inner development loop (RFC-009 §4).
#
# This file lives at the repository root because it has to watch catalog/,
# plugins/, ui/ and naira-openmfp-portal/.
#
# It is deliberately THIN: it renders the same charts everything else deploys
# and derives the plugin list from the chart's own values. Adding a plugin is
# one entry in deploy/charts/naira/values-dev.yaml and nothing here changes.
# A Tiltfile that re-declares the stack is what rots; one that reads it does not.

version_settings(constraint='>=0.33.0')
load('ext://helm_resource', 'helm_resource')

def namespace(name):
    return blob('apiVersion: v1\nkind: Namespace\nmetadata:\n  name: %s\n' % name)

CLUSTER   = 'naira-poc'
NS        = 'idp-system'
DEPS_NS   = 'naira-deps'
CHART     = 'deploy/charts/naira'
DEPS_CHART = '../component-testbed/charts/naira-dependencies'

# Never apply this against anything but the local kind cluster.
allow_k8s_contexts('kind-' + CLUSTER)

# ── the single source of truth for what exists ────────────────────────────────
values  = read_yaml(CHART + '/values-dev.yaml')
plugins = [p for p in values['catalog']['plugins'] if p.get('enabled', True)]

# ── image builds, derived ─────────────────────────────────────────────────────
def go_image(ref, dockerfile, deps):
    # Full docker build rather than a host `go build`: the Dockerfiles already
    # exist and are what CI uses, so the inner loop builds what CI builds.
    docker_build(ref, '.', dockerfile=dockerfile, only=deps)

go_image('catalog', 'catalog/Dockerfile', ['catalog/', 'go.mod', 'go.sum', 'plugins/pkg/'])

for p in plugins:
    go_image(
        p['image']['repository'],
        'plugins/cmd/' + p['source'] + '/Dockerfile',
        ['plugins/', 'go.mod', 'go.sum'],
    )

docker_build('ui', 'ui')
docker_build('portal', 'naira-openmfp-portal')

# ── the dependency tier, same chart ArgoCD syncs ──────────────────────────────
k8s_yaml(namespace(DEPS_NS))
k8s_yaml(helm(
    DEPS_CHART,
    name='naira-dependencies',
    namespace=DEPS_NS,
    values=[DEPS_CHART + '/values-dev.yaml'],
))

# ── Naira itself, same chart ArgoCD syncs ─────────────────────────────────────
k8s_yaml(namespace(NS))
k8s_yaml(helm(
    CHART,
    name='naira',
    namespace=NS,
    values=[CHART + '/values-dev.yaml'],
))

# ── port-forwards, so nobody needs a second terminal ──────────────────────────
k8s_resource('naira-catalog', port_forwards=['8090:8090'], labels=['naira'])
k8s_resource('naira-ui',      port_forwards=['3001:80'],   labels=['naira'])
k8s_resource('naira-portal',  port_forwards=['3000:3000'], labels=['naira'])
k8s_resource('keycloak',      port_forwards=['8080:8080'], labels=['deps'])
k8s_resource('litellm',       port_forwards=['4000:4000'], labels=['deps'])
k8s_resource('mlflow',        port_forwards=['5000:5000'], labels=['deps'])
k8s_resource('mcp-mock',                                   labels=['deps'])

# ── seeding: ordering declared, not documented ────────────────────────────────
# The Taskfile's seed targets say "requires port-forward" and leave the
# developer to arrange it. resource_deps makes that an actual dependency.
local_resource(
    'seed-mlflow',
    cmd='deploy/poc/seed-mlflow.sh',
    resource_deps=['mlflow'],
    auto_init=False,
    trigger_mode=TRIGGER_MODE_MANUAL,
    labels=['seed'],
)
