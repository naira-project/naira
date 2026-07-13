# depl_uses_litellm plugin

depl\_uses\_litellm scans k8s Deployments for LiteLLM API keys stored in referenced Secrets, then queries each configured LiteLLM host to discover which models that key can access, and emits deployment→model "uses\_model" relations.

Currently, the plugin emits Nodes for all Deployments with at least one secret with value matching the configured API key regexp (DEPL\_USES\_LITELLM\_APIKEY\_REGEXP), even if the key is not verified to be found in any of the configured LiteLLM hosts. This is intended to help detect potentially missing LiteLLM host configurations. TODO(security): add option to emit only verified deployments, to avoid leaking parts of accidentally matched secrets.

## Environment Variables {#hdr-Environment_Variables}

  - DEPL\_USES\_LITELLM\_NAMED\_HOSTS - MANDATORY - comma-separated list of named LiteLLM base URLs, e.g.: "host1=[https://litellm.example.com,host2=http://litellm2.example.com:1234/base/](https://litellm.example.com,host2=http://litellm2.example.com:1234/base/)"

  - DEPL\_USES\_LITELLM\_KUBECONFIG (optional) - path to kubeconfig file; if unset, in-cluster config is used.

  - DEPL\_USES\_LITELLM\_APIKEY\_REGEXP (optional) - custom regexp to match API keys; defaults to: "^sk-.{22}$" (LiteLLM API keys format as of May 2026)

  - DEPL\_USES\_LITELLM\_HTTP\_TIMEOUT (optional) - HTTP request timeout for LiteLLM API calls; defaults to: 5s.

---
Readme created from Go doc with [goreadme](https://github.com/posener/goreadme)
