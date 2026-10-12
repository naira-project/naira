# thalamus plugin

thalamus plugin discovers models served by Thalamus ([https://github.com/cobaltcore-dev/thalamus](https://github.com/cobaltcore-dev/thalamus)), a Kubernetes-native LLM serving operator.

When running in-cluster, models are collected from thalamus.cloud/v1alpha1 Model custom resources via the Kubernetes API. Each Model CR becomes a model Node (its path uses the Hugging Face repo ID when the weights are sourced from Hugging Face, falling back to the CR name otherwise), with properties describing its namespace, phase, engine, EPP and scheduling configuration. A Model whose weights are sourced from Hugging Face also gets a git\_repository Node for that Hugging Face repo, linked to the model via a "sourced\_from" relation.

When not running in-cluster (e.g. during local development), the plugin instead queries THALAMUS\_BASE\_URL's OpenAI API-compatible "/v1/models" endpoint directly, emitting a model Node per returned model id with only its "owned\_by" property.

## Known Issues

This plugin is third-party and HIGHLY EXPERIMENTAL. Notably, the Thalamus custom resources were NOT SUPPORTED BY THALAMUS yet at the time of writing.

## Environment Variables

  - THALAMUS\_BASE\_URL (optional) - base URL of the Thalamus OpenAI API-compatible endpoint, used only when the in-cluster CRD source is unavailable; defaults to: "[http://127.0.0.1:5000](http://127.0.0.1:5000)".
  - THALAMUS\_BEARER\_TOKEN (optional) - bearer token sent to THALAMUS\_BASE\_URL; if unset, the request is made unauthenticated.
  - HTTP\_TIMEOUT (optional) - HTTP request timeout; defaults to: 5s.
  - PATH\_PREFIX (optional) - prefix for the emitted model Node paths, e.g. "thalamus" yields "thalamus/gpt-oss-120b"; defaults to: "thalamus".

---
Readme created from Go doc with [goreadme](https://github.com/posener/goreadme)
