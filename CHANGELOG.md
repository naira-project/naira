# Changelog

## [0.1.0](https://github.com/naira-project/naira/compare/v0.0.2...v0.1.0) (2026-07-16)


### Features

* Naira is a central catalog, with an API for querying scanned Nodes and Relations. A simple SDK for writing plugins in Go language. A set of example PoC plugins, for scanning the following Nodes and Relations into the central catalog: 
 - in Kubernetes, calls between Deployments and Services (by simple scanning for Service hostnames in Deployments envs);
 - in Kubernetes, FluxCD CRDs and the Deployments they manage;
 - in Kubernetes, LiteLLM models referenced by Deployments (by simple heuristic scanning for Secrets matching the default LiteLLM key format - sk-…);
 - in LiteLLM, available models;
 - in MLFlow, models and datasets they were trained on.
 - in OpenMetadata, available datasets
   
* Naira UI and Naira OpenMFP portal, with portal listening to Naira UI as a micro-frontend component.  UI consists of:
 - table listing all nodes
 - catalog graph
([71ed87b](https://github.com/naira-project/naira/commit/71ed87ba4a0d7e14a98fce091f54aefdff70702e))
