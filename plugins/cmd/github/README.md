# github plugin

github plugin connects Kubernetes Deployments to their source GitHub repositories.

It discovers repositories whose artifact attestations cryptographically prove that they built the container images running in a cluster, enriching the results with repository metadata and top-level CODEOWNERS ownership.

## How It Works

  - Attestation Verification: A Deployment is linked to a repository only after successful verification using \`gh attestation verify\`.
  - ghcr.io Optimization: Images on ghcr.io that do not belong to GITHUB\_ORG are skipped to avoid unnecessary CLI calls. Non-ghcr.io images are always verified since path conventions vary across registries.
  - Single-Container Limit: Deployments with multiple containers are skipped because ownership cannot be attributed to a single image unambiguously.
  - CODEOWNERS Attribution: Only the default (\`\*\`) rule is used to assign ownership. Path-specific rules (e.g., \`/docs/ @docs-team\`) are ignored to prevent misrepresenting partial owners as whole-repository owners.

## Environment Variables

  - GITHUB\_ORG (mandatory) - limits collection to repositories whose attestations are verified for this GitHub organization, via "gh attestation verify --owner".
  - GITHUB\_TOKEN (mandatory) - GitHub API token used for repository metadata and CODEOWNERS, and passed as GH\_TOKEN to "gh attestation verify". It is required even for public repositories. A classic token with no selected scopes is sufficient.
  - GITHUB\_BASE\_URL (optional) - GitHub API base URL; defaults to "[https://api.github.com](https://api.github.com)".
  - GITHUB\_HTTP\_TIMEOUT (optional) - GitHub API request timeout; defaults to "10s".
  - GITHUB\_ATTESTATION\_TIMEOUT (optional) - timeout for a single "gh attestation verify" invocation; defaults to "30s".
  - GH\_CLI\_PATH (optional) - path to the "gh" binary; defaults to "gh" (resolved from PATH).
  - KUBECONFIG (optional) - path to a kubeconfig file; when unset, in-cluster configuration is used.

## TODOs

  - Link multi-container deployments by verifying each image independently.
  - Add registry filtering for non-ghcr.io images (e.g., \`AllowedImagePrefixes\`).
  - Verify support for private OCI registries and private GitHub repositories.

---
Readme created from Go doc with [goreadme](https://github.com/posener/goreadme)
