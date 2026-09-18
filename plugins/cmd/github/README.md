# github plugin

github finds GitHub repositories whose artifact attestations cryptographically prove that they built images running in Kubernetes Deployments, then enriches these repositories with metadata and CODEOWNERS information.

A Deployment is linked to a repository only after verification with "gh attestation verify".

Verification is skipped for ghcr.io images that do not belong to GITHUB\_ORG to avoid unnecessary "gh" CLI calls. Images hosted on other OCI registries are always passed to verification because path conventions vary across providers.

Deployments with more than one container are not verified because the repository cannot be attributed to a single image unambiguously.

TODO: Link deployments with more than one container to source repositories, verifying each container image independently.

TODO(optimization): Support filtering images for verification on registries other than ghcr.io (e.g. via AllowedImagePrefixes in the plugin configuration) to avoid verifying images that do not belong to GITHUB\_ORG.

TODO: Check support for private OCI registries and private GitHub repositories.

## Environment Variables

  - GITHUB\_ORG (mandatory) - limits collection to repositories whose attestations are verified for this GitHub organization, via "gh attestation verify --owner".
  - GITHUB\_TOKEN (mandatory) - GitHub API token used for repository metadata and CODEOWNERS, and passed as GH\_TOKEN to "gh attestation verify". It is required even for public repositories. A classic token with no selected scopes is sufficient.
  - GITHUB\_BASE\_URL (optional) - GitHub API base URL; defaults to "[https://api.github.com](https://api.github.com)".
  - GITHUB\_HTTP\_TIMEOUT (optional) - GitHub API request timeout; defaults to "10s".
  - GITHUB\_ATTESTATION\_TIMEOUT (optional) - timeout for a single "gh attestation verify" invocation; defaults to "30s".
  - GH\_CLI\_PATH (optional) - path to the "gh" binary; defaults to "gh" (resolved from PATH).
  - KUBECONFIG (optional) - path to a kubeconfig file; when unset, in-cluster configuration is used.

---
Readme created from Go doc with [goreadme](https://github.com/posener/goreadme)
