# langfuse plugin

langfuse plugin reads aggregate LLM metrics — cost, latency and user feedback scores — from Langfuse projects and attaches them to applications the catalog already knows about. It links out to Langfuse for the detail.

## Configuration

Projects, credentials and their bindings to catalog nodes live in a YAML file, re-read on every collect. Each binding names the observed node by namespace and name; its path prefix is either set on the binding or, by default, the cluster id resolved from the cluster. The file holds API secrets, so mount it from a Secret. See sample\_config.yaml for the schema.

## Metrics

Each collect queries GET /api/public/v2/metrics per scope: once for cost and latency, once for the configured score. A scope is a project, optionally narrowed to one trace name so applications sharing a project stay distinct.

A failing project re-emits its last good values marked stale rather than failing the collect, which would discard every other project's metrics too.

## Environment Variables

  - LANGFUSE\_CONFIG\_PATH (optional) - path to the YAML file described above; defaults to /etc/naira/langfuse/config.yaml.

  - LANGFUSE\_HTTP\_TIMEOUT (optional) - per-request timeout for Langfuse API calls; defaults to 10s.

  - LANGFUSE\_KUBECONFIG (optional) - path to a kubeconfig used to resolve the cluster id that bindings without a path\_prefix build node paths from; defaults to the in-cluster config.

---
Readme created from Go doc with [goreadme](https://github.com/posener/goreadme)
