# Changelog

## [0.2.0-rc](https://github.com/naira-project/naira/compare/v0.1.0...v0.2.0-rc) (2026-07-14)


### Features

* add cmd/plugintest ([99c3471](https://github.com/naira-project/naira/commit/99c347188aeaf943c224afa713d5f559d7f38637))
* add github workflows to support release process ([1aadd84](https://github.com/naira-project/naira/commit/1aadd849d79071b2ebed3b4c9eed2e218ccf7c05))
* add PR validation and release workflows ([#43](https://github.com/naira-project/naira/issues/43)) ([5dec3fe](https://github.com/naira-project/naira/commit/5dec3fe46384e360c1c8b4ec27b8a438f072fa42))
* create mise.toml ([94a602d](https://github.com/naira-project/naira/commit/94a602d8730301500ae79f95d8782dba4168ca52))
* keep data in buckets ([#44](https://github.com/naira-project/naira/issues/44)) ([f169323](https://github.com/naira-project/naira/commit/f169323772d5f42da14fa8a36111d17df5dd766b))
* OpenMFP portal for catalog ([42ed32f](https://github.com/naira-project/naira/commit/42ed32f0b9fb9402a5a540b3243d32e5f56a1722))
* OpenMFP portal for catalog ([42ed32f](https://github.com/naira-project/naira/commit/42ed32f0b9fb9402a5a540b3243d32e5f56a1722))
* OpenMFP portal for catalog ([09a1019](https://github.com/naira-project/naira/commit/09a1019b4a7b0e7c9adbea566929a6f0d6440568))
* pin k8s version in kind ([2528a07](https://github.com/naira-project/naira/commit/2528a075106e67065426af393cf7cda1731c2898))
* plugin scanning Deployment-&gt;Service calls ([c8265de](https://github.com/naira-project/naira/commit/c8265dedc9480c396b94f81f8c17085c58ed449e))
* plugin scanning Deployment-&gt;Service calls ([9a85dac](https://github.com/naira-project/naira/commit/9a85dac157ba4ccca4577ec2ee231796fb95e784))
* plugin scanning FluxCD CRDs & Deployments ([74f6996](https://github.com/naira-project/naira/commit/74f6996867da990558c0ab5ee4bae044af5edc48))
* plugin scanning FluxCD CRDs & Deployments ([34110e3](https://github.com/naira-project/naira/commit/34110e3fac3234370bfff145872ab2aba35213e8))
* prefix k8s Nodes paths with cluster UID ([7f76424](https://github.com/naira-project/naira/commit/7f76424b79a08c9e4d0d3073724afbc7b4a2f939))
* run plugins as gRPC sidecars ([#75](https://github.com/naira-project/naira/issues/75)) ([1442aaa](https://github.com/naira-project/naira/commit/1442aaa56ba6ed27229e3bb0545aa947d4ad5d83))


### Bug Fixes

* add artifact-metadata write permission to fix startup_failure ([131dcd6](https://github.com/naira-project/naira/commit/131dcd62f000617fc9e68eee0b5ce76e1715f113))
* catalog dockerfile typo ([#85](https://github.com/naira-project/naira/issues/85)) ([f34a931](https://github.com/naira-project/naira/commit/f34a93195894b0115e3e2651b82cd354d6bce750))
* drop optional namespace filter ([ceabcd2](https://github.com/naira-project/naira/commit/ceabcd22c1e7c576f875d4eaf7c81115f0f2b83c))
* empty "deployed_from".To possibility ([1588532](https://github.com/naira-project/naira/commit/1588532ee4e0db84577532fea8e37d1c5f1bf94d))
* group and sort tools in mise ([34957ed](https://github.com/naira-project/naira/commit/34957edaf958e1de5fdcf33b9e0fe3010e4c1d04))
* list Depls and Svcs from all namespaces ([c60866d](https://github.com/naira-project/naira/commit/c60866d6c984817d018431723c923829558644ca))
* log error rather than silently ignoring ([2792667](https://github.com/naira-project/naira/commit/279266776c81b03e230c53059e95019209993d27))
* no need for string separator yet in plugintest ([d13aed4](https://github.com/naira-project/naira/commit/d13aed4b2278aaf7258eea14c1760f6bed0666e6))
* prefix Node paths with cluster ID ([9b0d066](https://github.com/naira-project/naira/commit/9b0d066fb009e45c06f994b240f40b1ccfbe5f20))
* previous fix to err handling was logging too eagerly ([14e386f](https://github.com/naira-project/naira/commit/14e386fd7e42c0f4b78652024dac12e4034bc6be))
* reduce number of k8s API calls ([c93e968](https://github.com/naira-project/naira/commit/c93e9685106a66680522a4e93a8e8668b96ad38f))
* remove comments from release-please-config.json (JSON does not support comments and breaks Release Please & jq) ([1ae79e8](https://github.com/naira-project/naira/commit/1ae79e89367c60a9400bfc70e9792e8b75e1e99a))
* remove unsupported push input from reusable workflow calls ([0cd93cc](https://github.com/naira-project/naira/commit/0cd93cc6ac097fe8e69e8b77e9eff150dabf2b74))
* scan all namespaces ([d7f81a3](https://github.com/naira-project/naira/commit/d7f81a367ee617aca0bd491a4c0f5f764b871197))
* use full 40-character SHA for reusable workflows to fix GHA parsing error ([3823e73](https://github.com/naira-project/naira/commit/3823e733b270a2fca7f820b419f7d244248c08c1))
