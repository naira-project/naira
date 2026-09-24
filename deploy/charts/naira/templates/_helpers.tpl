{{- define "naira.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: naira
{{- end -}}

{{/* Usage: include "naira.selector" (dict "root" $ "name" "catalog") */}}
{{- define "naira.selector" -}}
app.kubernetes.io/name: {{ .name | quote }}
app.kubernetes.io/instance: {{ .root.Release.Name | quote }}
{{- end -}}

{{/*
Usage: include "naira.image" (dict "root" $ "image" .Values.catalog.image "context" "catalog")
"context" is the values path used in the error when image.repository is
missing (validate.yaml cannot guard this in time: templates render in file
order, and catalog.yaml dereferences .image before validate.yaml runs).
*/}}
{{- define "naira.image" -}}
{{- $img := .image | default dict -}}
{{- $repo := required (printf "%s.image.repository is required" .context) $img.repository -}}
{{- $tag := $img.tag | default .root.Values.image.tag | default .root.Chart.AppVersion -}}
{{- $ref := printf "%s:%v" $repo $tag -}}
{{- if .root.Values.image.registry -}}
{{- printf "%s/%s" .root.Values.image.registry $ref -}}
{{- else -}}
{{- $ref -}}
{{- end -}}
{{- end -}}

{{- define "naira.catalogSecretName" -}}
{{- .Values.catalog.secret.existingSecret | default "catalog-secrets" -}}
{{- end -}}

{{- define "naira.portalSecretName" -}}
{{- .Values.portal.oidc.secret.existingSecret | default "portal-oidc" -}}
{{- end -}}

{{/*
Env list from a map. String values are tpl-rendered against the root; a map
value is a secretKeyRef whose name defaults to the catalog Secret.
Usage: include "naira.env" (dict "root" $ "env" .env)
*/}}
{{- define "naira.env" -}}
{{- range $name, $v := .env }}
- name: {{ $name }}
{{- if kindIs "map" $v }}
  valueFrom:
    secretKeyRef:
      name: {{ $v.secretKeyRef.name | default (include "naira.catalogSecretName" $.root) }}
      key: {{ $v.secretKeyRef.key }}
{{- else }}
  value: {{ tpl (toString $v) $.root | quote }}
{{- end }}
{{- end }}
{{- end -}}

{{/* Usage: include "naira.pluginResources" (dict "root" $ "plugin" $p) */}}
{{- define "naira.pluginResources" -}}
{{- toYaml (mergeOverwrite (deepCopy .root.Values.catalog.pluginDefaults.resources) (.plugin.resources | default dict)) -}}
{{- end -}}
