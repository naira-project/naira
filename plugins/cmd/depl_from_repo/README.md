# depl_from_repo plugin

depl\_from\_repo scans Kubernetes Deployments and links them to the Git repositories from which they were deployed.

For every discovered Deployment, the plugin emits a Deployment node with container-image properties. When repository metadata can be extracted from the Deployment, it emits a GitRepository node and a deployed\_from relation from the Deployment to that repository.

Deployments with more than one container are intentionally not linked to a source repository because the repository cannot be attributed unambiguously.

TODO: Link deployments with more than one container to source repositories

TODO: Implement support for private OCI registries. Source repository discovery may fail for images stored in registries requiring authentication.

## Environment Variables

  - KUBECONFIG (optional) - path to a kubeconfig file; when unset, in-cluster configuration is used.

---
Readme created from Go doc with [goreadme](https://github.com/posener/goreadme)
