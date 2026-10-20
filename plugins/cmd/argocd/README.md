# argocd plugin

Plugin argocd collects ArgoCD Application objects and all the Kubernetes resources they manage, as reported in each Application's status.

Managed Deployments and Services are emitted as deployment and service Nodes. Child Applications (app-of-apps) are emitted as Application Nodes. Any other managed resource is emitted as a k8s\_resource Node, with a path of "\<cluster>/\<group>/\<Kind>/\<namespace>/\<name>" ("core" is used as the group of the core API, and the namespace segment is omitted for cluster-scoped resources).

An Application "describes" each resource it manages. If its source is a GitHub repository, the Application is "sourced\_from" a git\_repository Node, and managed Deployments are "deployed\_from" it. These two Relations have a "paths" property: a JSON list of the paths within the repository that the Application deploys from, e.g. \["apps","charts/web"]. If multiple sources of an Application use the same repository, it lists all their distinct paths. The property is absent when no source has a path (e.g. a source that only provides values files via "ref").

Application Nodes also have properties describing its syncs, copied as they are in the ArgoCD API, without interpretation, so that a consumer can combine them as it sees fit:

  - "operationState.phase", "operationState.startedAt" and "operationState.finishedAt": from status.operationState, which describes the current or the most recent sync operation. Each is absent if ArgoCD does not set it; all are absent if the Application has never been synced. Note that "phase" stays "Running" while a failed sync waits to be retried, and "finishedAt" is then the end of the failed attempt.
  - "operation\_present": "true" if the Application has the top-level "operation" field, which is a requested or running sync operation, else "false". The ArgoCD UI ignores status.operationState while it is set, and shows a "Running" phase instead (see getAppOperationState: [https://github.com/argoproj/argo-cd/blob/v3.5.5/ui/src/app/applications/components/utils.tsx#L1201-L1218](https://github.com/argoproj/argo-cd/blob/v3.5.5/ui/src/app/applications/components/utils.tsx#L1201-L1218)).
  - "last\_deployed\_at": the "deployedAt" of the newest entry of status.history, i.e. the time the last completed sync deployed. Unlike "operationState.\*", it is not affected by a sync in progress. Absent if the history is empty (it can be disabled with spec.revisionHistoryLimit).

For reference, the ArgoCD UI shows the phase together with "finishedAt", falling back to "startedAt" if that is not set ([https://github.com/argoproj/argo-cd/blob/v3.5.5/ui/src/app/applications/components/application-status-panel/application-status-panel.tsx#L349](https://github.com/argoproj/argo-cd/blob/v3.5.5/ui/src/app/applications/components/application-status-panel/application-status-panel.tsx#L349)), and uses the "deployedAt" of the last history entry only to count the days since the last sync ([https://github.com/argoproj/argo-cd/blob/v3.5.5/ui/src/app/applications/components/application-status-panel/application-status-panel.tsx#L210-L214](https://github.com/argoproj/argo-cd/blob/v3.5.5/ui/src/app/applications/components/application-status-panel/application-status-panel.tsx#L210-L214)).

All the times are in RFC 3339 format.

## Environment Variables

  - KUBECONFIG (optional) - path to the kubeconfig file; if unset, the in-cluster configuration is used.
  - ARGOCD\_RESOURCE\_TIMES (optional) - if "true", the plugin reads the metadata of each resource managed by an Application, and sets a "last\_synced\_at" property on its Node: the time ArgoCD last applied it, taken from the newest "managedFields" entry of the "argocd-controller" field manager. The property is left out for resources that cannot be read. Defaults to "false", in which case managed resources are not read at all.

## Required RBAC

By default, the plugin only needs "get" and "list" on "applications" in the "argoproj.io" API group, and "list" on namespaces.

With ARGOCD\_RESOURCE\_TIMES enabled, it additionally needs "get" on every kind of resource the Applications manage. As these are arbitrary, that effectively means "get" on all resources, e.g. a ClusterRole with apiGroups \["\*"], resources \["\*"], verbs \["get"]. Narrow it down to the kinds you deploy with ArgoCD if you prefer. Only the metadata is requested, never the contents, so the plugin does not see e.g. the data of Secrets, but note that "get" on Secrets still lets whoever holds the credentials read them. A resource the plugin is not allowed to read is skipped with a warning in the log.

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
