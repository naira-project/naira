# Local kind environment: deploy/charts/naira plus its test dependencies.
#
#   tilt up                        # create the cluster, build, deploy, port-forward
#   tilt down                      # remove what tilt deployed
#   tilt trigger cluster-delete    # delete the kind cluster (while tilt is up),
#                                  #   or: kind delete cluster --name naira-idp
#
# Dependencies (Keycloak, LiteLLM, MLflow, llama.cpp, MCP mock) come from the
# test-dependencies chart in a local checkout of naira-project/test-dependencies,
# by default next to this repo:
#
#   tilt up -- --deps-repo=<path>
#
# Developer tasks (lint, tests, proto) are manual resources: trigger them from
# the UI or with `tilt trigger <name>`.

NAMESPACE = 'idp-system'  # ui/nginx.conf.template still proxies to catalog.idp-system
CHART = 'deploy/charts/naira'
CLUSTER_NAME = 'naira-idp'
KIND_NODE_IMAGE = 'kindest/node:v1.36.1'  # update kubectl in mise.toml if changing version
DEPS_NAMESPACE = 'naira-deps'  # the naira chart's default addresses point here

config.define_string('deps-repo', usage='Path to a naira-project/test-dependencies checkout')
cfg = config.parse()
DEPS_REPO = os.path.abspath(cfg.get('deps-repo', '../test-dependencies'))
DEPS_CHART = DEPS_REPO + '/charts/test-dependencies'
if not os.path.exists(DEPS_CHART + '/Chart.yaml'):
    fail(('test-dependencies chart not found at %s. Clone naira-project/test-dependencies '
          + 'next to this repo or pass -- --deps-repo=<path>.') % DEPS_CHART)

# --- Cluster ------------------------------------------------------------------
# Tilt resolves the kube context before the Tiltfile runs, so the first
# `tilt up` after creating the cluster stops here and needs a restart.
if config.tilt_subcommand == 'up' and \
        CLUSTER_NAME not in str(local('kind get clusters', quiet=True)).splitlines():
    local('kind create cluster --name %s --image %s' % (CLUSTER_NAME, KIND_NODE_IMAGE))

# Tilt keeps the context it started with, so switching only takes effect after
# a restart; stop instead of deploying into the old context.
if k8s_context() != 'kind-' + CLUSTER_NAME:
    if config.tilt_subcommand != 'up':
        fail('kube context is %s, expected kind-%s.' % (k8s_context(), CLUSTER_NAME))
    # Also restores the context if the cluster exists but kubeconfig lost it.
    local('kind export kubeconfig --name ' + CLUSTER_NAME)
    fail('tilt started with kube context %s; switched to kind-%s. Restart tilt.'
         % (k8s_context(), CLUSTER_NAME))

# --- test-dependencies chart -------------------------------------------------
# Installed with helm itself rather than helm(): its subcharts rely on hooks
# (LiteLLM's migration job) and ship CRDs, which `helm template` would flatten.
load('ext://helm_resource', 'helm_resource')

# The published mcp-mock image is a private ghcr package; build it instead.
docker_build('mcp-mock', DEPS_REPO + '/images/mcp-mock')

local_resource(
    'test-dependencies-subcharts',
    cmd='helm dependency build ' + DEPS_CHART,
    deps=[DEPS_CHART + '/Chart.yaml', DEPS_CHART + '/Chart.lock'],
    labels=['test-dependencies'],
)
helm_resource(
    'test-dependencies', DEPS_CHART,
    namespace=DEPS_NAMESPACE,
    flags=['--create-namespace', '--values=' + DEPS_CHART + '/values-dev.yaml'],
    deps=[DEPS_CHART + '/values.yaml', DEPS_CHART + '/values-dev.yaml', DEPS_CHART + '/templates'],
    image_deps=['mcp-mock'],
    image_keys=[('mcpMock.image.registry', 'mcpMock.image.repository', 'mcpMock.image.tag')],
    resource_deps=['test-dependencies-subcharts'],
    labels=['test-dependencies'],
)

# One pod per release can't serve every Service, so forward them by name.
# Keycloak's port matches the portal's OIDC URLs (keycloak.issuer).
DEPS_FORWARDS = {
    'keycloak': '8080:8080',
    'mcp-mock': '8081:8080',
    'llama-qwen25-05b': '8083:8080',
    'llama-dummy-model': '8084:8080',
    'litellm': '4000:4000',
    'mlflow': '5000:5000',
}
for svc, ports in DEPS_FORWARDS.items():
    local_resource(
        svc + '-port-forward',
        serve_cmd='kubectl -n %s port-forward svc/%s %s' % (DEPS_NAMESPACE, svc, ports),
        resource_deps=['test-dependencies'],
        labels=['test-dependencies'],
    )

yaml = helm(
    CHART,
    name='naira',
    namespace=NAMESPACE,
    values=[CHART + '/values-dev.yaml'],
)
namespace_yaml = blob('apiVersion: v1\nkind: Namespace\nmetadata:\n  name: %s\n' % NAMESPACE)
k8s_yaml([namespace_yaml, yaml])

# --- Images ------------------------------------------------------------------
# values-dev.yaml sets an empty registry, so the chart references plain
# names: naira-catalog, naira-ui, naira-portal, naira-plugin-<name>. Build
# only the images the rendered chart uses, so enabling a plugin in values is
# enough. Tilt loads the images into kind itself.
images = []
for o in decode_yaml_stream(yaml):
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
config_objects = ['%s:namespace' % NAMESPACE]
for o in decode_yaml_stream(yaml):
    if o['kind'] in ('Deployment', 'Service'):
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

# --- Developer tasks ---------------------------------------------------------
PROTO_DIR = 'plugins/pkg/pluginapi/proto'
DEV_TASKS = {
    'check': 'kind version && kubectl version --client && helm version && go version'
             + ' && docker version && buf --version',
    'proto-generate': 'cd %s && go generate' % PROTO_DIR,
    'proto-lint': 'cd %s && buf lint' % PROTO_DIR,
    'go-test': 'go test -race ./...',
    'go-unit-test': 'go test -race ./... -short',
    'go-lint': 'golangci-lint run',
    'chart-check': CHART + '/tests/render-test.sh'
                   + ' && ct lint --config ct.yaml --target-branch main'
                   + ' && ' + CHART + '/tests/kubeconform.sh',
    'shell-lint': "git ls-files '*.sh' | xargs shellcheck",
    # Removes everything; stop tilt afterwards.
    'cluster-delete': 'kind delete cluster --name %s' % CLUSTER_NAME,
}
for name, cmd in DEV_TASKS.items():
    local_resource(name, cmd=cmd, trigger_mode=TRIGGER_MODE_MANUAL, auto_init=False,
                   labels=['dev-tasks'])
