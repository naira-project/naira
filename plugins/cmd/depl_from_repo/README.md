# depl_from_repo plugin

depl\_from\_repo scans Kubernetes Deployments and links them to the Git repositories from which they were deployed.

For every discovered Deployment, the plugin emits a Deployment node with container-image properties. When repository metadata can be extracted from the Deployment, it emits a GitRepository node and a deployed\_from relation from the Deployment to that repository.

## Environment Variables

  - KUBECONFIG (optional) - path to a kubeconfig file; when unset, in-cluster configuration is used.

---
Readme created from Go doc with [goreadme](https://github.com/posener/goreadme)
