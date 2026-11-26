# Live Kubernetes access test

Temporary local lab for developing cluster connection validation. It creates a
**separate kind cluster**, not a Naira deployment, and gives you real kubeconfigs
for successful and failed read-access checks. No cloud account is needed.

## Start

Requirements: Bash, Docker, kind, kubectl, Python 3 and enough memory for another
single-node Kubernetes cluster. Defaults match the repository's kind node image.

```bash
cd /home/rootlsz/github.com/naira/k8s-real-test
bash lab.sh
```

Use the menu in this order:

1. **Utwórz klaster** — creates `naira-registry-test`, ServiceAccounts, RBAC,
   a Deployment and a Service. The Deployment has zero replicas: no application
   image needs to be downloaded.
2. **Wygeneruj konfigurację** — creates expiring tokens and private kubeconfigs
   for all three profiles. It prints the fields to copy into Naira.
3. **Sprawdź rzeczywisty dostęp** — performs actual Kubernetes API reads and
   checks expected `Forbidden` responses.
4. Optionally install a kubeconfig Secret in the **existing Naira cluster**.

The script keeps its administrator kubeconfig under `.generated/`, never passes
it to Naira, and does not change your normal kubectl current context. Existing
clusters without this lab's local ownership marker are not adopted or deleted.

## Profiles

| Profile | Namespaces and `kube-system` UID | Deployments | Services | Expected future result |
| --- | --- | --- | --- | --- |
| `reader` | Read | Read | Read | READY |
| `no-services` | Read | Read | Forbidden | FAILED: Services check |
| `no-access` | Forbidden | Forbidden | Forbidden | FAILED: access checks |

The reader scope is **cluster-wide get/list** for namespaces, Deployments and
Services. This is a lab scope, not a final product decision about namespace
inclusion. No profile can read Secrets or create Deployments. Standard API
discovery access may still exist for authenticated ServiceAccounts.

Select a profile through the menu or use `--profile`. The check command always
tests every profile, including real reads, not only `kubectl auth can-i`.

## Values for the Naira form

Choose **Pokaż konfigurację**. For the selected profile the script prints:

- cluster name;
- Kubernetes API endpoint;
- kubeconfig context;
- a credential reference and the path to the kubeconfig;
- a JSON request matching the current registration fields.

Choose the endpoint according to **where the backend/plugin runs**, not where
the browser runs:

| Backend location | Generated configuration |
| --- | --- |
| Go process running on the host | `<profile>.host.kubeconfig` |
| Backend/plugin in the existing `naira-idp` kind cluster | `<profile>.kind-network.kubeconfig` |

The host configuration uses kind's loopback address and published port. Inside a
Pod, `127.0.0.1` is that Pod, not the host. The kind-network configuration uses
the target control-plane container's Docker-network IP and port 6443.

The latter requires both clusters to use the same Docker `kind` network and
routing from the Naira Pod to the target node. This is intended for a local
Linux Docker host. Docker Desktop, rootless or remote Docker and VPN/firewall
setups may need different routing. Both generated configurations keep CA
verification enabled; the check command tests both from the host, not from a
Naira Pod. It does **not** prove Pod-to-target connectivity.

### What works in Naira today?

Open **test workspace** and copy the selected profile's fields into the form.
Naira saves the registration in memory as `PENDING`.

After saving, use **Test connection** next to **Edit**. It shows individual
checks and sets READY or FAILED without importing resources. The backend must
first be configured to allow the referenced file or Secret; see below. Pasting
a reference does not make a host file visible to a Pod. Do not paste the
kubeconfig or token into the credential-reference field.

The generated `*.registration.json` files contain references, not credentials.
They can be used as request bodies for
`POST /v1/workspaces/test-workspace/registrations` after authenticating to Naira.
Saving does not automatically trigger validation. See
[connection test behavior and security](../docs/workspaces.md#manual-connection-tests).

**Sprawdź rzeczywisty dostęp** also verifies the expected permissions independently
of Naira.

### Backend running on the host

Copy only the desired reader configurations into a private directory. Do **not**
allow the entire `.generated/` directory: it also contains administrator access.

```bash
mkdir -p /tmp/naira-target-credentials
chmod 700 /tmp/naira-target-credentials
cp .generated/naira-registry-test/{reader,no-services,no-access}.host.kubeconfig /tmp/naira-target-credentials/
chmod 600 /tmp/naira-target-credentials/*.kubeconfig
export CLUSTER_CREDENTIAL_DIRECTORY=/tmp/naira-target-credentials
```

Start/restart the backend with this environment variable in addition to its
existing plugin and Keycloak configuration. In the UI, use the generated **host**
endpoint and context, but change the reference to, for example,
`kubeconfig-file:/tmp/naira-target-credentials/reader.host.kubeconfig`.
Refreshing lab tokens requires copying the new files again. Remove the private
directory when finished; cluster deletion does not remove these copies.

## Optional Secret in the Naira cluster

Use **Zainstaluj Secret** after generating kubeconfigs. Defaults:

```text
Destination context:   kind-naira-idp
Destination namespace: idp-system
Secret:                naira-registry-test-<profile>-access
Key:                   kubeconfig
```

Only the selected profile's kind-network kubeconfig is installed. The script
does not deploy Naira, create its namespace, change its RBAC or grant access to
Secrets. It refuses to overwrite/delete a Secret without its ownership label.
It uses server-side apply so the credential is not duplicated in a client-side
last-applied annotation.

For Naira running through Tilt/Helm, add an operator-approved entry to your local
chart values, then let Tilt redeploy the backend:

```yaml
catalog:
  clusterValidation:
    credentialSecrets:
      - name: naira-registry-test-reader-access
        key: kubeconfig
      - name: naira-registry-test-no-services-access
        key: kubeconfig
      - name: naira-registry-test-no-access-access
        key: kubeconfig
```

Install each desired profile using the menu or `install-secret --profile ...`.
Use the **kind-network** endpoint/context and the Secret reference printed by the
script. The chart configures the exact allowlist and named-Secret read permission.
Restarting the backend loses registrations, so add them again after deployment.

## Non-interactive commands

Run from this directory. Explicit actions do not prompt, including deletion.

```bash
bash lab.sh create
bash lab.sh generate
bash lab.sh check
bash lab.sh show --profile no-services
bash lab.sh status

# Optional: write only the selected credential Secret into Naira's cluster.
bash lab.sh install-secret --profile reader
bash lab.sh delete-secret --profile reader

# Deletes the dedicated test cluster and its local generated files, not Naira.
bash lab.sh delete
```

For a second physical cluster/fingerprint, repeat with a different lab name:

```bash
bash lab.sh create --cluster naira-registry-second
bash lab.sh generate --cluster naira-registry-second
bash lab.sh check --cluster naira-registry-second
bash lab.sh delete --cluster naira-registry-second
```

Other options: `--image`, `--duration`, `--naira-context`, `--naira-namespace`.
Pass the same `--cluster` to every action for that cluster. `--help` lists them.

## Credential lifetime and cleanup

- `.generated/` is ignored by Git; directories are private and kubeconfigs have
  mode 600. Files include administrator access and bearer tokens: do not commit,
  share, print them in CI logs or run the script with `bash -x`.
- Tokens request a one-hour lifetime by default. The script prints the actual
  expiry reported by Kubernetes. Use **Wygeneruj konfigurację** again after expiry.
- Regeneration issues new tokens but **does not revoke old ones**. Reinstall an
  optional Secret after regeneration. Replacing credentials is not proof of a
  complete rotation/revocation workflow.
- Deleting/recreating the cluster changes its CA and fingerprint; regenerate
  all configs. This is useful for testing installation continuity later.
- Remove installed Secrets separately, before or after deleting the lab cluster.
  Cluster deletion does not touch another cluster. To delete all three Secrets,
  run `delete-secret` once for each profile you installed.

## Troubleshooting

- Image pull fails with `certificate expired or not yet valid`: check host/VM
  clock synchronization first. Do not disable TLS verification.
- `Unauthorized` after previously working: check token expiry and regenerate.
- `Forbidden` for `reader`: rerun `create` to reapply RBAC, then `generate`.
- Connection refused / timeout: check that the target node exists, use the
  endpoint for the backend's network, and inspect firewall/network policies.
- Files remain after a failed create: inspect `kind get clusters`, the reported
  `.generated/<cluster>/` directory and node container. Delete via the script if
  its ownership marker exists. Do not delete unrelated clusters to recover.