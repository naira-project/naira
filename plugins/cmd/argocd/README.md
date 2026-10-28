# argocd plugin

Plugin argocd collects ArgoCD Application objects and all the Kubernetes resources they manage, as reported in each Application's status.

The data is read from the REST API of argocd-server ("/api/v1/applications"), not from the Kubernetes API.

Managed Deployments and Services are emitted as deployment and service Nodes. Child Applications (app-of-apps) are emitted as Application Nodes. Any other managed resource is emitted as a k8s\_resource Node, with a path of "\<cluster>/\<group>/\<Kind>/\<namespace>/\<name>" ("core" is used as the group of the core API, and the namespace segment is omitted for cluster-scoped resources).

Application Nodes have a path of "\<PATH\_PREFIX>/\<namespace>/\<name>". All other Nodes use CLUSTER\_ID as the "\<cluster>" path segment.

## Environment Variables

  - ARGOCD\_BASE\_URL (optional, default "[http://argocd-server.argocd.svc.cluster.local](http://argocd-server.argocd.svc.cluster.local)") - base URL of argocd-server.
  - ARGOCD\_TOKEN (optional) - API token, sent as a Bearer token. Needs only read access ("applications, get"), e.g. a token of a local account with such a role, see "argocd account generate-token".
  - ARGOCD\_INSECURE\_SKIP\_VERIFY (optional, default false) - skip verification of the TLS certificate of argocd-server, e.g. when it uses the self-signed one it generates by default.
  - ARGOCD\_HTTP\_TIMEOUT (optional, default 10s) - timeout of the HTTP requests.
  - PATH\_PREFIX (optional, default "argocd") - prefix of the paths of the Application Nodes. Set it to a different value for each ArgoCD instance.
  - CLUSTER\_ID - ID of the Kubernetes cluster that the Applications deploy to, which is the UID of its "kube-system" namespace, as used by other plugins. ArgoCD API doesn't expose it, so it must be provided by the user.

## Known Limitations

Only Applications deploying to the cluster ArgoCD runs in are supported; Applications targeting other clusters are skipped.

Currently, we only recognize Application sources ("repoURL") that match GitHub repository URLs. Unfortunately, ArgoCD uses ad-hoc heuristics to detect the source type based on the URL, and they're not trivial to reproduce.

TODO:

  - add support for ApplicationSets and AppProjects.
  - git repository Nodes could support other Git hosts than GitHub.
  - allow configuring custom regexps to recognize Git sources ("repoURL") other than GitHub, and Helm chart sources, to emit Nodes for them.
  - recognize oci:// "repoURL" sources and emit Nodes for them.

---
Readme created from Go doc with [goreadme](https://github.com/posener/goreadme)
