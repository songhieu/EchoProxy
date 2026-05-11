{{/* Common name helpers */}}

{{- define "echoproxy.fullname" -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "echoproxy.name" -}}
{{- .Chart.Name -}}
{{- end -}}

{{- define "echoproxy.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: echoproxy
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end -}}

{{- define "echoproxy.selectorLabels" -}}
app.kubernetes.io/name: {{ include "echoproxy.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/* Component-specific selector — used per Deployment/Service to avoid
     overlap between proxy / ingest / consumer / etc. */}}
{{- define "echoproxy.componentSelector" -}}
{{ include "echoproxy.selectorLabels" . }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{- define "echoproxy.componentLabels" -}}
{{ include "echoproxy.labels" . }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{/* Image reference. Each service is built as a separate image:
     ghcr.io/<owner>/echoproxy-<service>:<tag> */}}
{{- define "echoproxy.image" -}}
{{ .Values.image.registry }}/{{ .Values.image.owner }}/echoproxy-{{ .service }}:{{ .Values.image.tag }}
{{- end -}}

{{/* Name of the shared Secret (either user-provided or chart-generated). */}}
{{- define "echoproxy.secretName" -}}
{{- if .Values.secrets.existingSecret -}}
{{ .Values.secrets.existingSecret }}
{{- else -}}
{{ include "echoproxy.fullname" . }}-secrets
{{- end -}}
{{- end -}}
