# argocd plugin

Plugin argocd collects ArgoCD Application objects and all the Kubernetes resources they manage, as reported in each Application's status.

Managed Deployments and Services are emitted as deployment and service Nodes. Child Applications (app-of-apps) are emitted as Application Nodes. Any other managed resource is emitted as a k8s\_resource Node, with a path of "\<cluster>/\<group>/\<Kind>/\<namespace>/\<name>" ("core" is used as the group of the core API, and the namespace segment is omitted for cluster-scoped resources).

An Application "describes" each resource it manages. If its source is a GitHub repository, the Application is "sourced\_from" a git\_repository Node, and managed Deployments are "deployed\_from" it.

Only Applications deploying to the cluster they live in are supported; Applications targeting other clusters are skipped.

TODO: add support for ApplicationSets and AppProjects. TODO: git repository Nodes could support other Git hosts then GitHub.

---
Readme created from Go doc with [goreadme](https://github.com/posener/goreadme)
