# Changelog

## [0.1.0](https://github.com/naira-project/naira/compare/v0.1.0...v0.1.0) (2026-09-01)


### Features

* add -neo4j graph formatting flag for plugins ([f31a8c6](https://github.com/naira-project/naira/commit/f31a8c63a1ae857dd2121ed59356a59dbc65f497))
* add -neo4j graph formatting flag for plugins ([4b5afcb](https://github.com/naira-project/naira/commit/4b5afcb041119ee3738c1bf8d37c7184ec31f6cf))
* add prometheus, llama.cpp, vllm to dev environment ([#121](https://github.com/naira-project/naira/issues/121)) ([fb83b1b](https://github.com/naira-project/naira/commit/fb83b1b7cdad708ec2764f3d43e2b88765e28a88))
* Keycloak integration into Naira ([#95](https://github.com/naira-project/naira/issues/95)) ([9411cd4](https://github.com/naira-project/naira/commit/9411cd4306623ada8d633d5f9a1fed6d2dbe953b))
* mcp plugin & catalog ([#160](https://github.com/naira-project/naira/issues/160)) ([745a3c1](https://github.com/naira-project/naira/commit/745a3c14be97b91364063156a26b7aff9c7c84af))
* migrate the ui-poc from CRA to Vite ([#148](https://github.com/naira-project/naira/issues/148)) ([d3300f6](https://github.com/naira-project/naira/commit/d3300f6dab0cf29c715c0c8c69de88a94ca6792c))
* plugins & ingestion management dialog ([#136](https://github.com/naira-project/naira/issues/136)) ([645c4a2](https://github.com/naira-project/naira/commit/645c4a283c8e60a39ad1e279741358fa43637a0b))
* refactors plugin execution from synchronous to asynchronous ([#135](https://github.com/naira-project/naira/issues/135)) ([c93e665](https://github.com/naira-project/naira/commit/c93e665ef5988509640fb38bd7746ad0d42c877b))
* support configurable node path prefix per plugin instance ([#125](https://github.com/naira-project/naira/issues/125)) ([738fb33](https://github.com/naira-project/naira/commit/738fb33e6cea513ddd5159870b742636bcf05ad5))
* **ui:** kind based page separation ([#152](https://github.com/naira-project/naira/issues/152)) ([fb68112](https://github.com/naira-project/naira/commit/fb681122e99ed71ec6bf27df309bceb13ad60030))
* **ui:** lint UI files with biome ([#167](https://github.com/naira-project/naira/issues/167)) ([01b7a47](https://github.com/naira-project/naira/commit/01b7a4717bda1c6ce1578ffe179bf94b8678c758))
* **ui:** TanStack Table for table solution ([#162](https://github.com/naira-project/naira/issues/162)) ([bdc7e09](https://github.com/naira-project/naira/commit/bdc7e09aaa9da2eb51fb095ba1eb10d454ce035d))


### Bug Fixes

* decode path segments in GetNode ([#144](https://github.com/naira-project/naira/issues/144)) ([db1db64](https://github.com/naira-project/naira/commit/db1db6428f7ef1b22f88e4e9b400086ae22b032d))
* **ui:** improve graph nodes connection ([#172](https://github.com/naira-project/naira/issues/172)) ([09f588c](https://github.com/naira-project/naira/commit/09f588c2fa6ce3d25ae455fba9d215a92b1225a1))
* **ui:** upgrade to Tailwind v4 ([#174](https://github.com/naira-project/naira/issues/174)) ([4a0cba8](https://github.com/naira-project/naira/commit/4a0cba8c17612479b2de9f0c055bb2dc882a13a6))

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
