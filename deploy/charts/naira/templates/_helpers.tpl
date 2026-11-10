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

{{/* Usage: include "naira.pluginSecurityContext" (dict "root" $ "plugin" $p) */}}
{{- define "naira.pluginSecurityContext" -}}
{{- toYaml (mergeOverwrite (deepCopy .root.Values.catalog.pluginDefaults.securityContext) (.plugin.securityContext | default dict)) -}}
{{- end -}}

{{/*
Pod-level fields shared by the three Deployments; unset values render nothing.
Usage: include "naira.podSpec" (dict "root" $ "name" "catalog" "cfg" .Values.catalog)
*/}}
{{- define "naira.podSpec" -}}
{{- with .root.Values.imagePullSecrets }}
imagePullSecrets:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- with .cfg.priorityClassName }}
priorityClassName: {{ . | quote }}
{{- end }}
{{- with .cfg.podSecurityContext }}
securityContext:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- with .cfg.nodeSelector }}
nodeSelector:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- with .cfg.tolerations }}
tolerations:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- with .cfg.affinity }}
affinity:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- end -}}

{{- define "naira.dbHost" -}}
{{- .Values.database.host | default (printf "postgres.%s.svc.cluster.local" .Release.Namespace) -}}
{{- end -}}

{{- define "naira.dbAdminSecretName" -}}
{{- .Values.database.admin.secret.existingSecret | default "postgres-admin" -}}
{{- end -}}

{{/* Usage: include "naira.dbSecretName" (dict "name" $name "db" $db) */}}
{{- define "naira.dbSecretName" -}}
{{- .db.secret.existingSecret | default (printf "%s-db" .name) -}}
{{- end -}}

{{/*
"true" for a handbook entry that is enabled. A listed handbook is enabled unless it says false.
handbooks.yaml renders before validate.yaml, so this must not fail on a malformed entry.
*/}}
{{- define "naira.handbookEnabled" -}}
{{- if and (kindIs "map" .) (or (not (hasKey . "enabled")) .enabled) -}}true{{- end -}}
{{- end -}}

{{/* Non-empty when at least one handbook is enabled. */}}
{{- define "naira.hasHandbooks" -}}
{{- range .Values.handbooks -}}
{{- if include "naira.handbookEnabled" . -}}true{{- end -}}
{{- end -}}
{{- end -}}

{{- define "naira.handbookGatewayName" -}}
handbook-gateway
{{- end -}}

{{/* Usage: include "naira.handbookHost" (dict "root" $ "id" $id) */}}
{{- define "naira.handbookHost" -}}
{{- printf "%s.%s" .id .root.Values.handbookGateway.domain -}}
{{- end -}}

{{/*
Browser-facing address of a handbook. The port is left out when it is the scheme's default.
Usage: include "naira.handbookUrl" (dict "root" $ "id" $id)
*/}}
{{- define "naira.handbookUrl" -}}
{{- $gw := .root.Values.handbookGateway -}}
{{- $port := int $gw.port -}}
{{- if eq $port (ternary 443 80 (eq (toString $gw.scheme) "https")) -}}
{{- printf "%s://%s/" $gw.scheme (include "naira.handbookHost" .) -}}
{{- else -}}
{{- printf "%s://%s:%d/" $gw.scheme (include "naira.handbookHost" .) $port -}}
{{- end -}}
{{- end -}}

{{/*
keycloak.baseUrl as JSON {tls, host, port}; empty unless it is an http(s) URL with a host. The
match comes first because urlParse panics on a malformed URL, and handbook-gateway.yaml renders
before validate.yaml can reject it.
*/}}
{{- define "naira.keycloakEndpoint" -}}
{{- $base := toString (default "" .Values.keycloak.baseUrl) -}}
{{- if regexMatch "^https?://([a-zA-Z0-9]([-a-zA-Z0-9.]*[a-zA-Z0-9])?|\\[[0-9a-fA-F:.]+\\])(:[0-9]{1,5})?(/[^\\s?#]*)?$" $base -}}
{{- $url := urlParse $base -}}
{{- $tls := eq (get $url "scheme") "https" -}}
{{- $port := regexFind ":[0-9]+$" (get $url "host") | trimPrefix ":" | default (ternary "443" "80" $tls) -}}
{{- dict "tls" $tls "host" (get $url "hostname") "port" (int $port) | toJson -}}
{{- end -}}
{{- end -}}

{{/*
handbooks.json: the enabled handbooks for the portal, as JSON.
*/}}
{{- define "naira.handbooksJson" -}}
{{- $enabled := list }}
{{- range .Values.handbooks }}
{{- if include "naira.handbookEnabled" . }}
{{- $enabled = append $enabled . }}
{{- end }}
{{- end }}
{{- /* Primary first, then order, then id. The key sorts as text, so order is shifted and padded. */}}
{{- $byKey := dict }}
{{- range $enabled }}
{{- $primary := eq (toString .primary) "true" }}
{{- $_ := set $byKey (printf "%s|%07d|%s" (ternary "0" "1" $primary) (add 1000000 (int .order)) (toString .id)) . }}
{{- end }}
{{- $entries := list }}
{{- range $key := keys $byKey | sortAlpha }}
{{- $handbook := get $byKey $key }}
{{- $id := toString $handbook.id }}
{{- $entry := dict "id" $id "label" ($handbook.label | default $id) "primary" (eq (toString $handbook.primary) "true") "order" (int $handbook.order) "url" (include "naira.handbookUrl" (dict "root" $ "id" $id)) }}
{{- with $handbook.owner }}{{ $_ := set $entry "owner" . }}{{ end }}
{{- with $handbook.source }}{{ $_ := set $entry "source" . }}{{ end }}
{{- $entries = append $entries $entry }}
{{- end }}
{{- toPrettyJson $entries -}}
{{- end -}}
