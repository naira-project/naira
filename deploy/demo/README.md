# Demo: "Who uses which AI in claims?"

A demo overlay for the local kind environment. Every enabled plugin gets data,
and all of it tells one story.

```sh
tilt up -- --demo                      # or, on a running Tilt: tilt args -- --demo
GITHUB_TOKEN=$(gh auth token) tilt up -- --demo   # also enables the github plugin
```

OpenMetadata (about 4 GiB) and the monitoring stack (Prometheus, Grafana, Loki,
Alloy; about 2 GiB) are part of the demo. OpenMetadata takes several minutes to
start; its seed Job waits for it.

Switching an existing environment to the demo can leave the test-dependencies
release stuck in `pending-install`. Then run
`helm -n naira-deps uninstall test-dependencies`, `tilt trigger test-dependencies`,
and rerun the seed Jobs (delete them, then `tilt trigger <job>`).

## The story

Acme Insurance (fictional) uses AI in its claims department. Each persona sees
their part of the same picture.

**rAIner (claims-ai team)** trains models in MLflow: `claims-triage-classifier`,
`fraud-signal-scorer` and `damage-photo-tagger`, each linked to its training
datasets. The training data comes from the `claims_warehouse` in OpenMetadata,
where lineage shows `claims_raw` (tagged `PII.Sensitive`) flowing into
`claims_labeled_2025` and on into `fraud_features`. He also runs
`claims-assistant`, an agent that uses Claude Sonnet and embeddings through
LiteLLM with a key scoped to those 2 models, plus the knowledge-base MCP server.

**Abdel (claims-digital team)** builds `claims-portal`. It only calls
`claims-assistant` and holds no model key at all.

**Peter (platform team)** runs the platform. `claims-portal` and
`claims-assistant` come from the `claims-apps` Flux Kustomization, `pii-redactor`
from a Flux HelmRelease, both sourced from the `claims-platform` Git repository.
`pii-redactor` uses the self-hosted Qwen model on llama.cpp through the gateway,
so claim data stays in the cluster. The AI board publishes the `Acme Insurance AI Radar`,
edition 2026-11, whose entries link to the catalog nodes they describe.

**The finding.** `claims-faq-bot` has no Flux source and uses
`openrouter-gpt-3.5-turbo`, which the radar puts on Hold. The knowledge-base MCP
server exposes `delete_records`, and "Destructive MCP tools without approval" is
also on Hold. That is Shadow AI made visible.

## Click path

1. Tech radar: Acme Insurance AI Radar. Open "GPT-3.5 Turbo via OpenRouter" (Hold)
   and follow it to the model.
2. Model `litellm/openrouter-gpt-3.5-turbo`: used by deployment `claims/claims-faq-bot`.
3. `claims-faq-bot`: no Flux source, unlike `claims-assistant` next to it.
4. `claims-assistant`: uses Claude Sonnet and embeddings, calls the MCP server and
   `pii-redactor`; called by `claims-portal`.
5. MLflow model `fraud-signal-scorer`: trained on `claims-labeled-2025` and
   `payments-history-2025`. OpenMetadata shows where that data comes from.
6. Inference endpoint of `idp-llama-qwen25-05b`: healthy, metric panels with
   request rate, tokens and latency, and "View Logs" opens Grafana with the
   llama.cpp and LiteLLM lines. The Claude and GPT-3.5 endpoints show as
   unhealthy, because their provider keys are placeholders.

## Traffic, metrics and logs

`claims-assistant`, `pii-redactor` and `claims-faq-bot` send one request per
model every minute through LiteLLM with their own keys (`send.sh` in
`manifests/claims.yaml`). The litellm plugin only emits inference endpoints
for models with requests, and only those get metric panels and a logs link.

Only Qwen answers. The cloud models fail at the provider (401), and LiteLLM
records no per-deployment metrics for those failures, so their panels stay
empty. Their logs link still works.

## What feeds which plugin

| Plugin | Data in the demo | Source |
|---|---|---|
| litellm | Gateway models, inference endpoints, MCP server, AppIdentity `claims-assistant` | test-dependencies LiteLLM, `manifests/custom-resources.yaml` |
| depl-uses-litellm | Deployment to model relations from scoped gateway keys | `manifests/claims.yaml`, `seed/litellm_keys.py` |
| depl-calls-svc | Calls between claims workloads, LiteLLM, MCP and llama.cpp | `manifests/claims.yaml` |
| fluxcd | GitRepository, Kustomization, HelmRelease and the Deployments they manage | `manifests/custom-resources.yaml` |
| mlflow | 3 models with training datasets | `seed/mlflow_models.py` |
| openmetadata | 5 tables with PII and tier tags, 4 lineage edges | `seed/openmetadata_tables.py` |
| openai-api-models | Self-hosted Qwen on llama.cpp, prefix `onprem` | `values-naira.yaml` |
| mcp-servers | Knowledge-base MCP server and its tools | test-dependencies mcp-mock |
| tech-radar | 16 entries with catalog refs | `values-naira.yaml` |
| github | Only with `GITHUB_TOKEN`: links `mcp-mock` to its attested source repository | Tiltfile |

Not in the demo: bedrock (needs AWS credentials), thalamus and depl-uses-thalamus
(need a Thalamus installation).

## Limits

- `claims-portal` only sleeps (`pause` image); the other claims workloads only
  send the demo requests.
- The Flux CRDs are schema-less stand-ins with no controllers. The objects exist
  so the fluxcd plugin can read them; nothing reconciles.
  `github.com/acme-insurance/claims-platform` does not exist.
- All plugins run every 2 minutes. Use "Run plugins" in the UI for a faster refresh.
