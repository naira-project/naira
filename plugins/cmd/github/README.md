# github plugin

github enriches GitHub repositories discovered from Kubernetes Deployments with repository metadata and CODEOWNERS ownership information.

The plugin only collects repositories that are both referenced by a Kubernetes Deployment and owned by the GitHub organization configured in GITHUB\_ORG. It does not enumerate or collect repositories outside that organization. The organization restriction is applied using the GITHUB\_ORG environment variable.

TODO: For now repository - deployment relation only takes into account the first container image in a Deployment. It should support multiple images per Deployment.

TODO: Implement support for private OCI registries. Source repository discovery may fail for images stored in registries requiring authentication.

## Environment Variables

  - GITHUB\_ORG (mandatory) - limits collection to repositories owned by this GitHub organization.
  - GITHUB\_TOKEN (optional) - GitHub API bearer token used to access the repositories and CODEOWNERS files.
  - GITHUB\_BASE\_URL (optional) - GitHub API base URL; defaults to "[https://api.github.com](https://api.github.com)". Set this for GitHub Enterprise.
  - GITHUB\_HTTP\_TIMEOUT (optional) - GitHub API request timeout; defaults to 10s.
  - KUBECONFIG (optional) - path to a kubeconfig file; when unset, in-cluster configuration is used.

---
Readme created from Go doc with [goreadme](https://github.com/posener/goreadme)
