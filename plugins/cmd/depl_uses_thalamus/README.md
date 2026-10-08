# depl_uses_thalamus plugin

depl\_uses\_thalamus plugin discovers which Kubernetes Deployments consume the Thalamus gateway.

It lists models from Thalamus's OpenAI API-compatible "/v1/models" endpoint at THALAMUS\_BASE\_URL, then scans Deployments across all namespaces for a container env var whose name ends in "\_BASE\_URL" or "\_API\_BASE" and whose plaintext value starts with that same base URL. Each matching Deployment is linked to every model returned by Thalamus via a "uses\_model" relation.

## Known Issues

This plugin is third-party and HIGHLY EXPERIMENTAL.

## Environment Variables

  - THALAMUS\_BASE\_URL - MANDATORY - base URL of the Thalamus OpenAI API-compatible endpoint, without the "/v1/models" suffix, e.g.: "[https://thalamus.example.com](https://thalamus.example.com)".
  - THALAMUS\_BEARER\_TOKEN (optional) - bearer token sent to THALAMUS\_BASE\_URL; if unset, the request is made unauthenticated.
  - KUBECONFIG (optional) - path to kubeconfig file; if unset, in-cluster config is used.
  - HTTP\_TIMEOUT (optional) - HTTP request timeout; defaults to: 5s.
  - PATH\_PREFIX (optional) - prefix for the emitted model Node paths; defaults to: "thalamus".

---
Readme created from Go doc with [goreadme](https://github.com/posener/goreadme)
