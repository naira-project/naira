{{/* Base name, overridable. */}}
{{- define "naira.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "naira.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "naira.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/name: {{ include "naira.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: naira
{{- end -}}

{{/*
Resolve an image reference. Tag defaults to .Chart.AppVersion, so no chart in
this repo carries a hand-written tag (RFC-009 P2).
Usage: include "naira.image" (dict "root" $ "image" .image)
*/}}
{{- define "naira.image" -}}
{{- $img := .image -}}
{{- $registry := default .root.Values.global.imageRegistry $img.registry -}}
{{- $tag := default .root.Chart.AppVersion $img.tag -}}
{{- if $registry -}}
{{- printf "%s/%s:%s" $registry $img.repository $tag -}}
{{- else -}}
{{- printf "%s:%s" $img.repository $tag -}}
{{- end -}}
{{- end -}}

{{/* Plugins that are enabled, in declaration order. */}}
{{- define "naira.enabledPlugins" -}}
{{- $out := list -}}
{{- range .Values.catalog.plugins -}}
{{- if .enabled -}}{{- $out = append $out . -}}{{- end -}}
{{- end -}}
{{- toYaml $out -}}
{{- end -}}

{{- define "naira.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (printf "%s-catalog" (include "naira.fullname" .)) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "naira.secretName" -}}
{{- if .Values.catalog.secret.existingSecret -}}
{{- .Values.catalog.secret.existingSecret -}}
{{- else -}}
{{- printf "%s-catalog" (include "naira.fullname" .) -}}
{{- end -}}
{{- end -}}
