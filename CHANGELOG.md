# Changelog

## [0.1.0](https://github.com/naira-project/naira/compare/v0.0.2...v0.1.0) (2026-07-16)


### Features

* Naira is an open-source AI Engineering Hub that connects existing AI tools into one place to discover, govern and operate AI assets and services and their related components. In this first alpha release, it comes with a set of example PoC plugins for scanning the following Nodes and Relations into the central catalog:
  - Deployments and Services by simple scanning for Service hostnames in Deployments envs
  - FluxCD CRDs and the Deployments they manage
  - LiteLLM models referenced by Deployments
  - models and datasets from MLFlow
  - available datasets in OpenMetadata

*  Naira UI leverages OpenMFP to expose two core views into the asset catalog: a tabular "Catalog Explorer" view for browsing all kinds of assets, and an interactive "Catalog Graph" view for visually exploring the relationships between them, giving users both a structured and a visual lens on their data.
     
([71ed87b](https://github.com/naira-project/naira/commit/71ed87ba4a0d7e14a98fce091f54aefdff70702e))
