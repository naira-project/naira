#!/usr/bin/env bash
set -euo pipefail
umask 077

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLUSTER="naira-registry-test"
IMAGE="kindest/node:v1.36.1"
NAIRA_CONTEXT="kind-naira-idp"
NAIRA_NAMESPACE="idp-system"
PROFILE="reader"
TOKEN_DURATION="1h"
ACTION="menu"

usage() {
  cat <<'EOF'
Usage: bash lab.sh [menu|create|generate|show|check|status|install-secret|delete-secret|delete] [options]

  --cluster NAME       Dedicated kind cluster, must start with naira-registry-
  --image IMAGE        Node image (default: kindest/node:v1.36.1)
  --profile PROFILE    reader, no-services, no-access (default: reader)
  --duration DURATION  Requested token lifetime (default: 1h)
  --naira-context CTX  Destination for optional Secret (default: kind-naira-idp)
  --naira-namespace NS Destination namespace (default: idp-system)

Without arguments, starts an interactive menu. Explicit actions never prompt.
create: creates and seeds only the dedicated cluster.
generate: issues fresh, expiring tokens and writes kubeconfigs for all profiles.
check: performs real API reads, including expected failures; never imports data.
delete: deletes only the cluster identified by this lab's ownership marker and its files.
EOF
}

fail() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

if [[ $# -gt 0 && $1 != --* && $1 != -h ]]; then
  ACTION="$1"
  shift
fi
while [[ $# -gt 0 ]]; do
  [[ $1 != --help && $1 != -h ]] || { usage; exit 0; }
  [[ $# -ge 2 ]] || fail "Missing value for $1"
  case "$1" in
    --cluster) CLUSTER="$2" ;;
    --image) IMAGE="$2" ;;
    --profile) PROFILE="$2" ;;
    --duration) TOKEN_DURATION="$2" ;;
    --naira-context) NAIRA_CONTEXT="$2" ;;
    --naira-namespace) NAIRA_NAMESPACE="$2" ;;
    *) fail "Unknown option: $1" ;;
  esac
  shift 2
done

[[ $CLUSTER =~ ^naira-registry-[a-z0-9]([a-z0-9-]*[a-z0-9])?$ && ${#CLUSTER} -le 36 ]] || fail "Use a dedicated naira-registry-* name, at most 36 characters."
case "$PROFILE" in reader|no-services|no-access) ;; *) fail "Unknown profile: $PROFILE" ;; esac
case "$ACTION" in menu|create|generate|show|check|status|install-secret|delete-secret|delete) ;; *) usage; fail "Unknown action: $ACTION" ;; esac

STATE="$SCRIPT_DIR/.generated/$CLUSTER"
ADMIN="$STATE/admin.kubeconfig"
NODE="$CLUSTER-control-plane"
MARKER="$STATE/node-id"
SECRET="$CLUSTER-$PROFILE-access"

require_tools() {
  local tool
  for tool in "$@"; do command -v "$tool" >/dev/null || fail "Install $tool first."; done
}

admin() { kubectl --kubeconfig "$ADMIN" --request-timeout=15s "$@"; }

owned_cluster() {
  require_tools docker kubectl
  [[ -f $MARKER && -f $ADMIN ]] || fail "No ownership marker. Run create; never reuse an unrelated cluster."
  local current
  current="$(docker inspect --format '{{.Id}}' "$NODE")" || fail "Cluster is absent; generated files may be stale."
  [[ $current == "$(<"$MARKER")" ]] || fail "The cluster was replaced outside this script; refusing to use or delete it."
}

create_cluster() {
  require_tools kind docker kubectl
  docker info >/dev/null
  if kind get clusters | grep -Fxq "$CLUSTER"; then
    owned_cluster
    printf 'Reusing lab cluster %s.\n' "$CLUSTER"
  else
    [[ ! -e $STATE ]] || fail "Stale directory $STATE exists. Inspect and remove it before recreating."
    mkdir -p "$STATE"
    chmod 700 "$SCRIPT_DIR/.generated" "$STATE"
    # This private kubeconfig avoids changing the user's current context.
    if ! kind create cluster --name "$CLUSTER" --image "$IMAGE" --kubeconfig "$ADMIN" --wait 120s; then
      if docker inspect --format '{{.Id}}' "$NODE" > "$MARKER" 2>/dev/null; then
        printf 'Partial cluster recorded. Use delete after inspecting the failure.\n' >&2
      else
        rm -f "$MARKER"
        printf 'No node recorded. Inspect kind get clusters and %s before retrying.\n' "$STATE" >&2
      fi
      return 1
    fi
    docker inspect --format '{{.Id}}' "$NODE" > "$MARKER"
  fi
  admin apply -f "$SCRIPT_DIR/fixtures.yaml"
  printf '\nCluster seeded. Choose generate next. No changes made to %s.\n' "$NAIRA_CONTEXT"
}

generate_configs() {
  require_tools python3
  owned_cluster
  admin apply -f "$SCRIPT_DIR/fixtures.yaml" >/dev/null
  local profile token_file
  for profile in reader no-services no-access; do
    token_file="$(mktemp "$STATE/.token.XXXXXX")"
    if ! admin -n naira-discovery-test create token "$profile" --duration="$TOKEN_DURATION" > "$token_file"; then
      rm -f "$token_file"
      return 1
    fi
    if ! python3 - "$ADMIN" "$STATE" "$CLUSTER" "$profile" "$token_file" <<'PY'
import base64
import datetime
import json
import os
from pathlib import Path
import subprocess
import sys

admin, directory, cluster_name, profile, token_file = sys.argv[1:]
raw = json.loads(subprocess.check_output([
    'kubectl', '--kubeconfig', admin, 'config', 'view', '--raw', '--flatten', '-o', 'json'
]))
connection = raw['clusters'][0]['cluster']
node = json.loads(subprocess.check_output(['docker', 'inspect', cluster_name + '-control-plane']))[0]
address = node['NetworkSettings']['Networks']['kind']['IPAddress']
if not address:
    raise SystemExit('Expected an IPv4 address on the kind Docker network.')
token = Path(token_file).read_text().strip()
claims = json.loads(base64.urlsafe_b64decode(token.split('.')[1] + '==='))
expires = datetime.datetime.fromtimestamp(claims['exp'], datetime.timezone.utc).isoformat()
context = f'{cluster_name}-{profile}'
for mode, server in [('host', connection['server']), ('kind-network', f'https://{address}:6443')]:
    config = {
        'apiVersion': 'v1', 'kind': 'Config', 'current-context': context,
        'clusters': [{'name': cluster_name, 'cluster': {
            'server': server, 'certificate-authority-data': connection['certificate-authority-data']
        }}],
        'users': [{'name': profile, 'user': {'token': token}}],
        'contexts': [{'name': context, 'context': {'cluster': cluster_name, 'user': profile}}],
    }
    path = Path(directory) / f'{profile}.{mode}.kubeconfig'
    path.write_text(json.dumps(config, indent=2) + '\n')
    os.chmod(path, 0o600)
    request = {
        'name': f'{cluster_name}-{profile}',
        'credentialRef': f'kubeconfig-file:{path.resolve()}',
        'kubernetes': {'apiEndpoint': server, 'context': context},
    }
    (Path(directory) / f'{profile}.{mode}.registration.json').write_text(json.dumps(request, indent=2) + '\n')
(Path(directory) / f'{profile}.expires-at').write_text(expires + '\n')
PY
    then
      rm -f "$token_file"
      return 1
    fi
    rm -f "$token_file"
  done
  printf 'Generated private kubeconfigs in %s. Token lifetime requested: %s.\n' "$STATE" "$TOKEN_DURATION"
  printf 'Generating again issues new tokens; it does not revoke previously issued tokens.\n'
  show_config
}

show_config() {
  require_tools python3
  [[ -f $STATE/$PROFILE.host.kubeconfig ]] || fail "Run generate first."
  python3 - "$STATE" "$PROFILE" "$NAIRA_CONTEXT" "$NAIRA_NAMESPACE" "$SECRET" <<'PY'
import json
from pathlib import Path
import sys
directory, profile, naira_context, namespace, secret = sys.argv[1:]
root = Path(directory)
print(f'\nProfile: {profile}; token expires at: {(root / (profile + ".expires-at")).read_text().strip()}')
for mode in ['host', 'kind-network']:
    path = root / f'{profile}.{mode}.kubeconfig'
    config = json.loads(path.read_text())
    print(f'\n--- {mode} ---')
    print('Cluster name:       ' + root.name + '-' + profile)
    print('API endpoint:       ' + config['clusters'][0]['cluster']['server'])
    print('Kubeconfig context: ' + config['current-context'])
    print('Credential reference (requires backend operator allowlist):')
    print('kubeconfig-file:' + str(path.resolve()))
    print('Registration JSON:  ' + str(root / f'{profile}.{mode}.registration.json'))
print(f'\nOptional destination Secret: context={naira_context}, namespace={namespace}, name={secret}, key=kubeconfig')
print(f'Secret reference: kubernetes-secret:{namespace}/{secret}#kubeconfig')
print('Secret installation is separate. Approve the exact reference in catalog.clusterValidation.credentialSecrets before testing.')
print('For host testing, copy reader configs to a private directory and set CLUSTER_CREDENTIAL_DIRECTORY; do not allow admin.kubeconfig.')
print('Host paths are not visible inside Naira Pods without a mount. Use kind-network for Naira running in kind.')
print('No token or kubeconfig content is printed. Never paste credentials into the reference field.')
PY
}

check_access() {
  owned_cluster
  local profile mode config namespaces deployments services expected resource allowed actual
  for profile in reader no-services no-access; do
    for mode in host kind-network; do
      config="$STATE/$profile.$mode.kubeconfig"
      [[ -f $config ]] || fail "Run generate first."
      printf '\n%s / %s\n' "$profile" "$mode"
      if [[ $profile == no-access ]]; then
        namespaces=no; deployments=no; services=no
      else
        namespaces=yes; deployments=yes; services=yes
        [[ $profile != no-services ]] || services=no
      fi
      for resource in namespaces deployments.apps services; do
        case "$resource" in namespaces) expected="$namespaces" ;; deployments.apps) expected="$deployments" ;; services) expected="$services" ;; esac
        if actual="$(kubectl --kubeconfig "$config" --request-timeout=15s auth can-i list "$resource" --all-namespaces 2> "$STATE/check-error.log")"; then
          allowed=yes
        else
          [[ $actual == no ]] || { cat "$STATE/check-error.log" >&2; fail "Access check failed (network, TLS, expired token?)."; }
          allowed=no
        fi
        [[ $allowed == "$expected" ]] || fail "$resource: expected $expected, got $actual"
        if [[ $expected == yes ]]; then
          kubectl --kubeconfig "$config" --request-timeout=15s get "$resource" --all-namespaces -o name
        else
          if kubectl --kubeconfig "$config" --request-timeout=15s get "$resource" --all-namespaces -o name > /dev/null 2> "$STATE/check-error.log"; then
            fail "Unexpected successful read of $resource"
          fi
          grep -qi forbidden "$STATE/check-error.log" || { cat "$STATE/check-error.log" >&2; fail "Expected Forbidden for $resource."; }
          printf '%s: Forbidden as expected\n' "$resource"
        fi
      done
      if [[ $profile != no-access ]]; then
        printf 'kube-system UID: '
        kubectl --kubeconfig "$config" --request-timeout=15s get namespace kube-system -o jsonpath='{.metadata.uid}{"\n"}'
      else
        if kubectl --kubeconfig "$config" --request-timeout=15s get namespace kube-system > /dev/null 2> "$STATE/check-error.log"; then
          fail "Unexpected successful read of kube-system"
        fi
        grep -qi forbidden "$STATE/check-error.log" || { cat "$STATE/check-error.log" >&2; fail "Expected Forbidden for kube-system."; }
        printf 'kube-system UID: Forbidden as expected\n'
      fi
      actual="$(kubectl --kubeconfig "$config" --request-timeout=15s auth can-i list secrets --all-namespaces || true)"
      [[ $actual == no ]] || fail "Profile must not read Secrets."
      actual="$(kubectl --kubeconfig "$config" --request-timeout=15s auth can-i create deployments.apps -n naira-discovery-test || true)"
      [[ $actual == no ]] || fail "Profile must not create Deployments."
    done
  done
  rm -f "$STATE/check-error.log"
  printf '\nPASS: expected reads and failures verified; no resources imported into Naira.\n'
}

destination() {
  kubectl --context "$NAIRA_CONTEXT" --namespace "$NAIRA_NAMESPACE" --request-timeout=15s "$@"
}

secret_guard() {
  local label
  label="$(destination get secret "$SECRET" --ignore-not-found -o "jsonpath={.metadata.labels['naira\\.io/real-test-cluster']}")"
  [[ -z $label || $label == "$CLUSTER" ]] || fail "Secret is not owned by this lab."
  if destination get secret "$SECRET" >/dev/null 2>&1; then
    [[ $label == "$CLUSTER" ]] || fail "Refusing to overwrite/delete an existing unlabelled Secret."
  fi
}

install_secret() {
  require_tools python3 kubectl
  owned_cluster
  [[ $NAIRA_CONTEXT != "kind-$CLUSTER" ]] || fail "Choose the Naira cluster, not the target cluster."
  [[ -f $STATE/$PROFILE.kind-network.kubeconfig ]] || fail "Run generate first."
  destination get namespace "$NAIRA_NAMESPACE" >/dev/null
  secret_guard
  printf 'Installing only Secret %s in %s/%s. No reader RBAC is granted to Naira.\n' "$SECRET" "$NAIRA_CONTEXT" "$NAIRA_NAMESPACE"
  destination create secret generic "$SECRET" --from-file="kubeconfig=$STATE/$PROFILE.kind-network.kubeconfig" --dry-run=client -o json |
    python3 -c 'import json,sys; d=json.load(sys.stdin); d["metadata"]["labels"]={"naira.io/real-test-cluster":sys.argv[1]}; print(json.dumps(d))' "$CLUSTER" |
    destination apply --server-side --field-manager=naira-real-test -f -
  show_config
}

delete_secret() {
  require_tools kubectl
  secret_guard
  destination delete secret "$SECRET" --ignore-not-found
}

delete_cluster() {
  require_tools kind
  owned_cluster
  kind delete cluster --name "$CLUSTER" --kubeconfig "$ADMIN"
  rm -rf -- "$STATE"
  printf 'Deleted lab cluster and generated files. Any Secrets installed in Naira must be removed separately.\n'
}

status() {
  owned_cluster
  admin get nodes
  admin -n naira-discovery-test get deployments,services,serviceaccounts
  printf 'Private files: %s\n' "$STATE"
}

menu() {
  local choice
  while true; do
    printf '\nKlaster: %s | profil: %s\n' "$CLUSTER" "$PROFILE"
    cat <<'EOF'
1) Utwórz klaster testowy i zasoby
2) Wygeneruj/odśwież kubeconfigi (wszystkie profile)
3) Pokaż konfigurację do formularza Nairy
4) Sprawdź rzeczywisty dostęp (wszystkie profile)
5) Zainstaluj Secret w klastrze Nairy (opcjonalne)
6) Usuń Secret z klastra Nairy
7) Status klastra
8) Usuń klaster testowy i lokalne pliki
9) Zmień profil: reader / no-services / no-access
0) Wyjdź
EOF
    read -r -p 'Wybór: ' choice || break
    # A failed action must stop before its later steps, but not terminate the menu.
    case "$choice" in
      1) ( set -e; create_cluster ) ;;
      2) ( set -e; generate_configs ) ;;
      3) ( set -e; show_config ) ;;
      4) ( set -e; check_access ) ;;
      5) ( set -e; install_secret ) ;;
      6) ( set -e; delete_secret ) ;;
      7) ( set -e; status ) ;;
      8)
        read -r -p "Wpisz $CLUSTER aby usunąć: " choice || break
        if [[ $choice == "$CLUSTER" ]]; then ( set -e; delete_cluster ); fi
        ;;
      9)
        read -r -p 'Profil: ' choice || break
        case "$choice" in reader|no-services|no-access) PROFILE="$choice"; SECRET="$CLUSTER-$PROFILE-access" ;; *) printf 'Nieznany profil.\n' ;; esac
        ;;
      0) break ;;
      *) printf 'Nieznany wybór.\n' ;;
    esac
  done
}

if [[ $ACTION == menu ]]; then
  # Each action enables errexit in its own subshell so failures return to the menu.
  set +e
  menu
else
  case "$ACTION" in
    create) create_cluster ;;
    generate) generate_configs ;;
    show) show_config ;;
    check) check_access ;;
    status) status ;;
    install-secret) install_secret ;;
    delete-secret) delete_secret ;;
    delete) delete_cluster ;;
  esac
fi