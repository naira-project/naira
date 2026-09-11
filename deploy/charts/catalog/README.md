# catalog

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.1.0](https://img.shields.io/badge/AppVersion-0.1.0-informational?style=flat-square)

A Helm chart for the Naira Catalog component

## Requirements

Kubernetes: `>=1.29.0-0`

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` |  |
| autoscaling.enabled | bool | `false` |  |
| autoscaling.maxReplicas | int | `100` |  |
| autoscaling.minReplicas | int | `1` |  |
| autoscaling.targetCPUUtilizationPercentage | int | `80` |  |
| containerName | string | `"catalog"` |  |
| env[0].name | string | `"PORT"` |  |
| env[0].value | string | `"8090"` |  |
| existingSecret | string | `"catalog-secrets"` | Existing Secret containing credentials consumed by Catalog plugins. |
| fullnameOverride | string | `""` |  |
| image.pullPolicy | string | `"IfNotPresent"` |  |
| image.repository | string | `"ghcr.io/naira-project/naira-catalog"` |  |
| image.tag | string | `"0.1.0"` |  |
| imagePullSecrets | list | `[]` |  |
| livenessProbe.httpGet.path | string | `"/healthz"` |  |
| livenessProbe.httpGet.port | string | `"http"` |  |
| livenessProbe.initialDelaySeconds | int | `15` |  |
| livenessProbe.periodSeconds | int | `20` |  |
| nameOverride | string | `""` |  |
| nodeSelector | object | `{}` |  |
| plugins.deplCallsSvc.enabled | bool | `true` |  |
| plugins.deplCallsSvc.env[0].name | string | `"DEPL_CALLS_SVC_KUBECONFIG"` |  |
| plugins.deplCallsSvc.env[0].value | string | `""` |  |
| plugins.deplCallsSvc.image.pullPolicy | string | `"IfNotPresent"` |  |
| plugins.deplCallsSvc.image.repository | string | `"ghcr.io/naira-project/naira-plugin-depl-calls-svc"` |  |
| plugins.deplCallsSvc.image.tag | string | `"0.1.0"` |  |
| plugins.deplCallsSvc.name | string | `"depl-calls-svc"` |  |
| plugins.deplCallsSvc.port | int | `50053` |  |
| plugins.deplCallsSvc.resources.limits.cpu | string | `"200m"` |  |
| plugins.deplCallsSvc.resources.limits.memory | string | `"128Mi"` |  |
| plugins.deplCallsSvc.resources.requests.cpu | string | `"50m"` |  |
| plugins.deplCallsSvc.resources.requests.memory | string | `"64Mi"` |  |
| plugins.deplUsesLitellm.enabled | bool | `true` |  |
| plugins.deplUsesLitellm.env[0].name | string | `"DEPL_USES_LITELLM_KUBECONFIG"` |  |
| plugins.deplUsesLitellm.env[0].value | string | `""` |  |
| plugins.deplUsesLitellm.env[1].name | string | `"DEPL_USES_LITELLM_NAMED_HOSTS"` |  |
| plugins.deplUsesLitellm.env[1].value | string | `"litellm=http://litellm.litellm:4000"` |  |
| plugins.deplUsesLitellm.image.pullPolicy | string | `"IfNotPresent"` |  |
| plugins.deplUsesLitellm.image.repository | string | `"ghcr.io/naira-project/naira-plugin-depl-uses-litellm"` |  |
| plugins.deplUsesLitellm.image.tag | string | `"sha-8420753"` |  |
| plugins.deplUsesLitellm.name | string | `"depl-uses-litellm"` |  |
| plugins.deplUsesLitellm.port | int | `50054` |  |
| plugins.deplUsesLitellm.resources.limits.cpu | string | `"200m"` |  |
| plugins.deplUsesLitellm.resources.limits.memory | string | `"128Mi"` |  |
| plugins.deplUsesLitellm.resources.requests.cpu | string | `"50m"` |  |
| plugins.deplUsesLitellm.resources.requests.memory | string | `"64Mi"` |  |
| plugins.fluxcd.enabled | bool | `true` |  |
| plugins.fluxcd.env | list | `[]` |  |
| plugins.fluxcd.image.pullPolicy | string | `"IfNotPresent"` |  |
| plugins.fluxcd.image.repository | string | `"ghcr.io/naira-project/naira-plugin-fluxcd"` |  |
| plugins.fluxcd.image.tag | string | `"0.1.0"` |  |
| plugins.fluxcd.name | string | `"fluxcd"` |  |
| plugins.fluxcd.port | int | `50055` |  |
| plugins.fluxcd.resources.limits.cpu | string | `"200m"` |  |
| plugins.fluxcd.resources.limits.memory | string | `"128Mi"` |  |
| plugins.fluxcd.resources.requests.cpu | string | `"50m"` |  |
| plugins.fluxcd.resources.requests.memory | string | `"64Mi"` |  |
| plugins.litellm.enabled | bool | `true` |  |
| plugins.litellm.env[0].name | string | `"LITELLM_BASE_URL"` |  |
| plugins.litellm.env[0].value | string | `"http://litellm.litellm.svc.cluster.local:4000"` |  |
| plugins.litellm.image.pullPolicy | string | `"IfNotPresent"` |  |
| plugins.litellm.image.repository | string | `"ghcr.io/naira-project/naira-plugin-litellm"` |  |
| plugins.litellm.image.tag | string | `"0.1.0"` |  |
| plugins.litellm.name | string | `"litellm"` |  |
| plugins.litellm.port | int | `50051` |  |
| plugins.litellm.resources.limits.cpu | string | `"200m"` |  |
| plugins.litellm.resources.limits.memory | string | `"128Mi"` |  |
| plugins.litellm.resources.requests.cpu | string | `"50m"` |  |
| plugins.litellm.resources.requests.memory | string | `"64Mi"` |  |
| plugins.mlflow.enabled | bool | `true` |  |
| plugins.mlflow.env[0].name | string | `"MLFLOW_BASE_URL"` |  |
| plugins.mlflow.env[0].value | string | `"http://mlflow.mlflow.svc.cluster.local:5000"` |  |
| plugins.mlflow.image.pullPolicy | string | `"IfNotPresent"` |  |
| plugins.mlflow.image.repository | string | `"ghcr.io/naira-project/naira-plugin-mlflow"` |  |
| plugins.mlflow.image.tag | string | `"0.1.0"` |  |
| plugins.mlflow.name | string | `"mlflow"` |  |
| plugins.mlflow.port | int | `50052` |  |
| plugins.mlflow.resources.limits.cpu | string | `"200m"` |  |
| plugins.mlflow.resources.limits.memory | string | `"128Mi"` |  |
| plugins.mlflow.resources.requests.cpu | string | `"50m"` |  |
| plugins.mlflow.resources.requests.memory | string | `"64Mi"` |  |
| plugins.openmetadata.enabled | bool | `true` |  |
| plugins.openmetadata.env[0].name | string | `"OPENMETADATA_BASE_URL"` |  |
| plugins.openmetadata.env[0].value | string | `"http://openmetadata.openmetadata.svc.cluster.local:8585"` |  |
| plugins.openmetadata.env[1].name | string | `"OPENMETADATA_ADMIN_EMAIL"` |  |
| plugins.openmetadata.env[1].value | string | `"admin@open-metadata.org"` |  |
| plugins.openmetadata.image.pullPolicy | string | `"IfNotPresent"` |  |
| plugins.openmetadata.image.repository | string | `"ghcr.io/naira-project/naira-plugin-openmetadata"` |  |
| plugins.openmetadata.image.tag | string | `"sha-8420753"` |  |
| plugins.openmetadata.name | string | `"openmetadata"` |  |
| plugins.openmetadata.port | int | `50056` |  |
| plugins.openmetadata.resources.limits.cpu | string | `"200m"` |  |
| plugins.openmetadata.resources.limits.memory | string | `"128Mi"` |  |
| plugins.openmetadata.resources.requests.cpu | string | `"50m"` |  |
| plugins.openmetadata.resources.requests.memory | string | `"64Mi"` |  |
| podAnnotations | object | `{}` |  |
| podLabels | object | `{}` |  |
| podSecurityContext | object | `{}` |  |
| rbac.appIdentities.create | bool | `true` |  |
| rbac.appIdentities.name | string | `""` |  |
| rbac.create | bool | `true` |  |
| rbac.deplCallsSvc.create | bool | `true` |  |
| rbac.deplCallsSvc.name | string | `""` |  |
| rbac.deplUsesLitellm.create | bool | `true` |  |
| rbac.deplUsesLitellm.name | string | `""` |  |
| rbac.fluxcd.create | bool | `true` |  |
| rbac.fluxcd.name | string | `""` |  |
| readinessProbe.httpGet.path | string | `"/healthz"` |  |
| readinessProbe.httpGet.port | string | `"http"` |  |
| readinessProbe.initialDelaySeconds | int | `5` |  |
| readinessProbe.periodSeconds | int | `10` |  |
| replicaCount | int | `1` |  |
| resources.limits.cpu | string | `"500m"` |  |
| resources.limits.memory | string | `"256Mi"` |  |
| resources.requests.cpu | string | `"100m"` |  |
| resources.requests.memory | string | `"128Mi"` |  |
| secretKeys.litellmApiKey | string | `"LITELLM_API_KEY"` | Key containing the LiteLLM API key. |
| secretKeys.openmetadataAdminPassword | string | `"OPENMETADATA_ADMIN_PASSWORD"` | Key containing the OpenMetadata administrator password. |
| securityContext | object | `{}` |  |
| service.port | int | `8090` |  |
| service.type | string | `"ClusterIP"` |  |
| serviceAccount.annotations | object | `{}` |  |
| serviceAccount.automount | bool | `true` |  |
| serviceAccount.create | bool | `true` |  |
| serviceAccount.name | string | `""` |  |
| tolerations | list | `[]` |  |
| volumeMounts | list | `[]` |  |
| volumes | list | `[]` |  |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.14.2](https://github.com/norwoodj/helm-docs/releases/v1.14.2)
