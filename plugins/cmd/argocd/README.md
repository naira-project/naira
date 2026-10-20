# argocd plugin

Plugin argocd collects ArgoCD Application objects and all the Kubernetes resources they manage, as reported in each Application's status.

Managed Deployments and Services are emitted as deployment and service Nodes. Child Applications (app-of-apps) are emitted as Application Nodes. Any other managed resource is emitted as a k8s\_resource Node, with a path of "\<cluster>/\<group>/\<Kind>/\<namespace>/\<name>" ("core" is used as the group of the core API, and the namespace segment is omitted for cluster-scoped resources).

## Environment Variables

  - KUBECONFIG (optional) - path to the kubeconfig file; if unset, the in-cluster configuration is used.

## Known Limitations

Only Applications deploying to the cluster they live in are supported; Applications targeting other clusters are skipped.

Currently, we only recognize Application sources ("repoURL") that match GitHub repository URLs. Unfortunately, ArgoCD uses ad-hoc heuristics to detect the source type based on the URL, and they're not trivial to reproduce.

TODO:

  - add support for ApplicationSets and AppProjects.
  - git repository Nodes could support other Git hosts than GitHub.
  - allow configuring custom regexps to recognize Git sources ("repoURL") other than GitHub, and Helm chart sources, to emit Nodes for them.
  - recognize oci:// "repoURL" sources and emit Nodes for them.

---
Readme created from Go doc with [goreadme](https://github.com/posener/goreadme)
