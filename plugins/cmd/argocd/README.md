# argocd plugin

Plugin argocd collects ArgoCD Application objects and all the Kubernetes resources they manage, as reported in each Application's status.

Managed Deployments and Services are emitted as deployment and service Nodes. Child Applications (app-of-apps) are emitted as Application Nodes. Any other managed resource is emitted as a k8s\_resource Node, with a path of "\<cluster>/\<group>/\<Kind>/\<namespace>/\<name>" ("core" is used as the group of the core API, and the namespace segment is omitted for cluster-scoped resources).

An Application "describes" each resource it manages. If its source is a GitHub repository, the Application is "sourced\_from" a git\_repository Node, and managed Deployments are "deployed\_from" it. These two Relations have a "paths" property: a JSON list of the paths within the repository that the Application deploys from, e.g. \["apps","charts/web"]. If multiple sources of an Application use the same repository, it lists all their distinct paths. The property is absent when no source has a path (e.g. a source that only provides values files via "ref").

Application Nodes also have a "last\_synced\_at" property: the time the last sync operation of the Application finished, in RFC 3339 format.

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
