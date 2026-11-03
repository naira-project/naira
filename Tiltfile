# Dependencies (Keycloak, LiteLLM, MLflow, llama.cpp, MCP mock) come from the
# published test-dependencies chart (DEPS_VERSION), or from a local checkout of
# naira-project/test-dependencies to change its values or templates:
#
#   tilt up -- --deps-repo=<path>
#
# Developer tasks (lint, tests, proto) are in the Taskfile: they don't need Tilt.

NAMESPACE = 'idp-system'  # test-dependencies reaches PostgreSQL at postgres.idp-system
CHART = 'deploy/charts/naira'
CLUSTER_NAME = 'naira-idp'
KIND_NODE_IMAGE = 'kindest/node:v1.36.1'  # update kubectl in mise.toml if changing version
DEPS_NAMESPACE = 'naira-deps'  # the naira chart's default addresses point here
DEPS_OCI = 'oci://ghcr.io/naira-project/charts/test-dependencies'
DEPS_VERSION = '0.1.0'

config.define_string('deps-repo', usage='Path to a naira-project/test-dependencies checkout')
cfg = config.parse()
DEPS_REPO = cfg.get('deps-repo', '')
if DEPS_REPO:
    DEPS_CHART = os.path.abspath(DEPS_REPO) + '/charts/test-dependencies'
    if not os.path.exists(DEPS_CHART + '/Chart.yaml'):
        fail('test-dependencies chart not found at %s.' % DEPS_CHART)
else:
    # Pulled once per version into the gitignored .local/ (after the context check).
    DEPS_CHART = os.path.abspath('.local/test-dependencies-' + DEPS_VERSION) + '/test-dependencies'

# --- Cluster ------------------------------------------------------------------
# Checked before any local(): Tilt refuses local() on a non-dev context, and
# keeps the context it started with.
if k8s_context() != 'kind-' + CLUSTER_NAME:
    fail(('kube context is "%s", expected kind-%s. Create or select the cluster, then rerun tilt:\n'
          + '  kind create cluster --name %s --image %s\n'
          + '  kind export kubeconfig --name %s   # if it exists')
         % (k8s_context(), CLUSTER_NAME, CLUSTER_NAME, KIND_NODE_IMAGE, CLUSTER_NAME))

if not DEPS_REPO and not os.path.exists(DEPS_CHART + '/Chart.yaml'):
    local('helm pull %s --version %s --untar --untardir %s'
          % (DEPS_OCI, DEPS_VERSION, os.path.dirname(DEPS_CHART)))

# --- naira chart ------------------------------------------------------------
# The PostgreSQL subchart is not vendored; fetch it when missing.
if os.path.exists(CHART + '/Chart.lock'):
    watch_file(CHART + '/Chart.lock')
    for d in read_yaml(CHART + '/Chart.lock').get('dependencies', []):
        if not os.path.exists('%s/charts/%s-%s.tgz' % (CHART, d['name'], d['version'])):
            local('helm dependency build ' + CHART)
            break

yaml = helm(
    CHART,
    name='naira',
    namespace=NAMESPACE,
    values=[CHART + '/values-dev.yaml'],
)
namespace_yaml = blob('apiVersion: v1\nkind: Namespace\nmetadata:\n  name: %s\n' % NAMESPACE)
k8s_yaml([namespace_yaml, yaml])

# PostgreSQL and db-init deploy first: LiteLLM keeps its database there, so
# test-dependencies waits for them (the rest of naira waits for test-dependencies).
objs = decode_yaml_stream(yaml)
has_pg = len([o for o in objs if o['kind'] == 'StatefulSet' and o['metadata']['name'] == 'postgres']) > 0
db_init_jobs = [o['metadata']['name'] for o in objs if o['kind'] == 'Job' and o['metadata']['name'].startswith('db-init-')]

def pg_object(o):
    md = o['metadata']
    if o['kind'] == 'Secret' and (md['name'] == 'postgres-admin' or md['name'].endswith('-db')):
        return True
    return o['kind'] == 'ConfigMap' and md['name'] in ('postgres', 'db-init')

if has_pg:
    k8s_resource('postgres', objects=['%s:namespace' % NAMESPACE] +
                 ['%s:%s' % (o['metadata']['name'], o['kind'].lower()) for o in objs if pg_object(o)],
                 labels=['naira'])
for job in db_init_jobs:
    k8s_resource(job, new_name='db-init', resource_deps=['postgres'] if has_pg else [], labels=['naira'])
DEPS_AFTER = ['db-init'] if db_init_jobs else (['postgres'] if has_pg else [])

# --- test-dependencies chart -------------------------------------------------
# Installed with helm itself rather than helm(): its subcharts use hooks (MLflow's
# secret, helm test pods, kube-prometheus-stack's admission jobs) and ship CRDs.
load('ext://helm_resource', 'helm_resource')

# The published chart ships its subcharts; a checkout needs them fetched.
# `update`, not `build`: it needs no `helm repo add` for the subchart repos.
if DEPS_REPO:
    local_resource(
        'test-dependencies-subcharts',
        cmd='helm dependency update ' + DEPS_CHART,
        deps=[DEPS_CHART + '/Chart.yaml', DEPS_CHART + '/Chart.lock'],
        labels=['test-dependencies'],
    )
    DEPS_AFTER = DEPS_AFTER + ['test-dependencies-subcharts']
DEPS_VALUES = [DEPS_CHART + '/values.yaml', DEPS_CHART + '/values-dev.yaml']
helm_resource(
    'test-dependencies', DEPS_CHART,
    namespace=DEPS_NAMESPACE,
    flags=['--create-namespace', '--values=' + DEPS_VALUES[1]],
    deps=DEPS_VALUES + [DEPS_CHART + '/templates'],
    resource_deps=DEPS_AFTER,
    labels=['test-dependencies'],
)

# Top-level keys merge like helm does for the keys read here (tags, llamacpp.models).
deps_values = {}
for f in DEPS_VALUES:
    for k, v in (read_yaml(f) or {}).items():
        deps_values[k] = dict(deps_values.get(k, {}), **v) if type(v) == 'dict' else v
tags = deps_values.get('tags', {})

# One pod per release can't serve every Service, so forward them by name, only
# for enabled tags. Keycloak's port matches the portal's OIDC URLs (keycloak.issuer).
# mlflow uses 5001: macOS AirPlay Receiver listens on 5000.
DEPS_FORWARDS = {}
for tag, svc, ports in [
    ('keycloak', 'keycloak', '8080:8080'),
    ('mcpMock', 'mcp-mock', '8081:8080'),
    ('litellm', 'litellm', '4000:4000'),
    ('mlflow', 'mlflow', '5001:5000'),
    ('vllm', 'vllm', '8000:8000'),
    ('openmetadata', 'openmetadata', '8585:8585'),
    ('monitoring', 'test-dependencies-grafana', '3002:80'),
    ('monitoring', 'test-dependencies-kube-pro-prometheus', '9090:9090'),
]:
    if tags.get(tag):
        DEPS_FORWARDS[svc] = ports
if tags.get('llamacpp'):
    for i, m in enumerate(deps_values.get('llamacpp', {}).get('models', [])):
        DEPS_FORWARDS[m['name']] = '%d:8080' % (8083 + i)

# The release is ready before its pods are, and port-forward exits while a pod
# is pending or after it restarts, so retry instead of leaving the resource failed.
for svc, ports in DEPS_FORWARDS.items():
    local_resource(
        svc + '-port-forward',
        serve_cmd='while true; do kubectl -n %s port-forward svc/%s %s; sleep 2; done'
                  % (DEPS_NAMESPACE, svc, ports),
        resource_deps=['test-dependencies'],
        labels=['test-dependencies'],
    )


# --- Images ------------------------------------------------------------------
# values-dev.yaml sets an empty registry, so the chart references plain
# names: naira-catalog, naira-ui, naira-portal, naira-plugin-<name>. Build
# only the images the rendered chart uses, so enabling a plugin in values is
# enough. Tilt loads the images into kind itself.
images = []
for o in objs:
    if o['kind'] != 'Deployment':
        continue
    # Plugins are native sidecars, i.e. initContainers.
    pod = o['spec']['template']['spec']
    for c in pod.get('initContainers', []) + pod['containers']:
        repo = c['image'].split(':')[0]
        if repo not in images:
            images.append(repo)

for repo in images:
    if repo == 'naira-catalog':
        docker_build(repo, '.', dockerfile='catalog/Dockerfile',
                     only=['go.mod', 'go.sum', 'catalog/', 'plugins/pkg/pluginapi/'])
    elif repo == 'naira-ui':
        docker_build(repo, 'ui')
    elif repo == 'naira-portal':
        docker_build(repo, 'naira-openmfp-portal')
    elif repo.startswith('naira-plugin-'):
        dir = repo[len('naira-plugin-'):].replace('-', '_')
        docker_build(repo, '.', dockerfile='plugins/cmd/%s/Dockerfile' % dir,
                     only=['go.mod', 'go.sum', 'plugins/pkg/', 'plugins/internal/', 'plugins/cmd/%s/' % dir])
    else:
        fail('no build rule for image %s' % repo)

# --- Resources ----------------------------------------------------------------
# Namespace, Secrets, ConfigMaps, ServiceAccount, RBAC and NetworkPolicies go
# into one resource that the workloads wait for, instead of "uncategorized".
config_objects = [] if has_pg else ['%s:namespace' % NAMESPACE]
for o in objs:
    if o['kind'] in ('Deployment', 'StatefulSet', 'Job', 'Service') or (has_pg and pg_object(o)):
        continue
    md = o['metadata']
    sel = '%s:%s' % (md['name'], o['kind'].lower())
    if md.get('namespace'):
        sel += ':' + md['namespace']
    config_objects.append(sel)
k8s_resource(new_name='naira-config', objects=config_objects,
             resource_deps=['test-dependencies'], labels=['naira'])

k8s_resource('catalog', resource_deps=['naira-config'], port_forwards=['8090:8090'], labels=['naira'])
k8s_resource('portal', resource_deps=['naira-config'], port_forwards=['3000:3000'], labels=['naira'])
k8s_resource('ui', resource_deps=['naira-config'], port_forwards=['3001:80'], labels=['naira'])
