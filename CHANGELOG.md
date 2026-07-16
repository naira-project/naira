# Changelog

## [0.1.0](https://github.com/naira-project/naira/compare/v0.0.2...v0.1.0) (2026-07-16)


### Features

* Naira is a central catalog, with an API for querying scanned Nodes and Relations. A simple SDK for writing plugins in Go language. A set of example PoC plugins, for scanning the following Nodes and Relations into the central catalog: ([71ed87b](https://github.com/naira-project/naira/commit/71ed87ba4a0d7e14a98fce091f54aefdff70702e))

## [0.0.2](https://github.com/naira-project/naira/compare/v0.0.1...v0.0.2) (2026-07-16)


### Features

* add cmd/plugintest ([99c3471](https://github.com/naira-project/naira/commit/99c347188aeaf943c224afa713d5f559d7f38637))
* add github workflows to support release process ([#96](https://github.com/naira-project/naira/issues/96)) ([1bff3ec](https://github.com/naira-project/naira/commit/1bff3ecf64f789e9b91a44fe2aebad16201338bc))
* add Helm charts for catalog and ui ([#72](https://github.com/naira-project/naira/issues/72)) ([d4e385c](https://github.com/naira-project/naira/commit/d4e385c6abfc2f89bd686506c81f1b0c3aaa9b29))
* add PR validation and release workflows ([#43](https://github.com/naira-project/naira/issues/43)) ([5dec3fe](https://github.com/naira-project/naira/commit/5dec3fe46384e360c1c8b4ec27b8a438f072fa42))
* allow depl-uses-litellm to fetch models from http ([340ec33](https://github.com/naira-project/naira/commit/340ec33e20fe311da7e9fbddf1bfad74a32ebef3))
* create mise.toml ([94a602d](https://github.com/naira-project/naira/commit/94a602d8730301500ae79f95d8782dba4168ca52))
* keep data in buckets ([#44](https://github.com/naira-project/naira/issues/44)) ([f169323](https://github.com/naira-project/naira/commit/f169323772d5f42da14fa8a36111d17df5dd766b))
* openmetadata plugin  ([#93](https://github.com/naira-project/naira/issues/93)) ([f2c2186](https://github.com/naira-project/naira/commit/f2c21869335ea8578f1494f78f670e57a64442b8))
* OpenMFP portal for catalog ([42ed32f](https://github.com/naira-project/naira/commit/42ed32f0b9fb9402a5a540b3243d32e5f56a1722))
* OpenMFP portal for catalog ([42ed32f](https://github.com/naira-project/naira/commit/42ed32f0b9fb9402a5a540b3243d32e5f56a1722))
* OpenMFP portal for catalog ([09a1019](https://github.com/naira-project/naira/commit/09a1019b4a7b0e7c9adbea566929a6f0d6440568))
* pin k8s version in kind ([2528a07](https://github.com/naira-project/naira/commit/2528a075106e67065426af393cf7cda1731c2898))
* plugin scanning Deployment-&gt;Service calls ([c8265de](https://github.com/naira-project/naira/commit/c8265dedc9480c396b94f81f8c17085c58ed449e))
* plugin scanning Deployment-&gt;Service calls ([9a85dac](https://github.com/naira-project/naira/commit/9a85dac157ba4ccca4577ec2ee231796fb95e784))
* plugin scanning FluxCD CRDs & Deployments ([74f6996](https://github.com/naira-project/naira/commit/74f6996867da990558c0ab5ee4bae044af5edc48))
* plugin scanning FluxCD CRDs & Deployments ([34110e3](https://github.com/naira-project/naira/commit/34110e3fac3234370bfff145872ab2aba35213e8))
* plugin scanning LiteLLM models from Deployment Secrets ([26475f2](https://github.com/naira-project/naira/commit/26475f2ceec51a8b50e2204fb57818ec1df9b1d8))
* plugin scanning LiteLLM models from Deployment Secrets ([64186c1](https://github.com/naira-project/naira/commit/64186c133a1d75b7ffc00d763f9eebc41a2f9b76))
* prefix k8s Nodes paths with cluster UID ([7f76424](https://github.com/naira-project/naira/commit/7f76424b79a08c9e4d0d3073724afbc7b4a2f939))
* run plugins as gRPC sidecars ([#75](https://github.com/naira-project/naira/issues/75)) ([1442aaa](https://github.com/naira-project/naira/commit/1442aaa56ba6ed27229e3bb0545aa947d4ad5d83))
* UI generic table for kinds ([#94](https://github.com/naira-project/naira/issues/94)) ([d4f3a81](https://github.com/naira-project/naira/commit/d4f3a81aea9671dbf69506c25ae19cd69aeaea4e))
* use named hosts in depl_uses_litellm ([c3c28f1](https://github.com/naira-project/naira/commit/c3c28f1b5e38f57a619259a90df898eb08904561))


### Bug Fixes

* Add artifact-metadata permission to workflow ([#110](https://github.com/naira-project/naira/issues/110)) ([cc72094](https://github.com/naira-project/naira/commit/cc72094d391a412c7ee9a0a64dbb0b9e2b30d61c))
* add missing -trimpath in depl_uses_litellm Dockerfile ([5d7b3ae](https://github.com/naira-project/naira/commit/5d7b3aea75ec91e3fe4f940f6e5d9fd58557a322))
* avoid repeated fetches of same secrets ([7c657d6](https://github.com/naira-project/naira/commit/7c657d66b134722cd851595375fad8faa4c88049))
* caching of secrets in depl_uses_litellm ([aee9f3d](https://github.com/naira-project/naira/commit/aee9f3d1a70b70787ccd560ee15c416f46f21341))
* catalog dockerfile typo ([#85](https://github.com/naira-project/naira/issues/85)) ([f34a931](https://github.com/naira-project/naira/commit/f34a93195894b0115e3e2651b82cd354d6bce750))
* dockerfile to not hardcode os+arch ([5f5b78d](https://github.com/naira-project/naira/commit/5f5b78d028b143e508e4b4d0f60bb444d3a74682))
* drop optional namespace filter ([ceabcd2](https://github.com/naira-project/naira/commit/ceabcd22c1e7c576f875d4eaf7c81115f0f2b83c))
* duplicate "litellm/" in depl_uses_litellm model Path ([88e9a2b](https://github.com/naira-project/naira/commit/88e9a2b071f679c9930d221cb45b01ce185c3927))
* empty "deployed_from".To possibility ([1588532](https://github.com/naira-project/naira/commit/1588532ee4e0db84577532fea8e37d1c5f1bf94d))
* group and sort tools in mise ([34957ed](https://github.com/naira-project/naira/commit/34957edaf958e1de5fdcf33b9e0fe3010e4c1d04))
* list Depls and Svcs from all namespaces ([c60866d](https://github.com/naira-project/naira/commit/c60866d6c984817d018431723c923829558644ca))
* log error rather than silently ignoring ([2792667](https://github.com/naira-project/naira/commit/279266776c81b03e230c53059e95019209993d27))
* no need for string separator yet in plugintest ([d13aed4](https://github.com/naira-project/naira/commit/d13aed4b2278aaf7258eea14c1760f6bed0666e6))
* openmetadata port value change & remove ns creation from secrets file ([#109](https://github.com/naira-project/naira/issues/109)) ([4b3def8](https://github.com/naira-project/naira/commit/4b3def87729ee614218e9f5fa07fef54626bce1a))
* post-merge ([a9f93cc](https://github.com/naira-project/naira/commit/a9f93cccad76b9603bfd637ecf29d572c517a2f3))
* post-rebase ([758404c](https://github.com/naira-project/naira/commit/758404c381275c1a4ed85e9c43ac947a22165e4b))
* prefix Node paths with cluster ID ([9b0d066](https://github.com/naira-project/naira/commit/9b0d066fb009e45c06f994b240f40b1ccfbe5f20))
* previous fix to err handling was logging too eagerly ([14e386f](https://github.com/naira-project/naira/commit/14e386fd7e42c0f4b78652024dac12e4034bc6be))
* reduce number of k8s API calls ([c93e968](https://github.com/naira-project/naira/commit/c93e9685106a66680522a4e93a8e8668b96ad38f))
* report bad config when HOSTS is empty ([4659731](https://github.com/naira-project/naira/commit/4659731709a1372a4f4d967c9050cb3119317f39))
* scan all namespaces ([f94d1da](https://github.com/naira-project/naira/commit/f94d1daf6ae9867f64b225c0fdc2982ad6baa9d8))
* scan all namespaces ([d7f81a3](https://github.com/naira-project/naira/commit/d7f81a367ee617aca0bd491a4c0f5f764b871197))
* use context in plugin depl_uses_litellm ([28b428e](https://github.com/naira-project/naira/commit/28b428ed07ecbc2ef5b617674d94c6cfc648c923))
