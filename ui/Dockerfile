FROM node:26.12.0-alpine@sha256:6f08a523443ddedf52941874c203475b0ad503e859ad70cb26ab8be9a517dbd5 AS builder
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:1.31.0-alpine@sha256:2f07d83bf561b506400dc183b1b2003803e39efbd22451f848adaba14d28c7c7

COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf.template /etc/nginx/templates/default.conf.template
COPY docker-entrypoint.sh /custom-entrypoint.sh
RUN chmod +x /custom-entrypoint.sh
# Upstreams the nginx config proxies to; override per deployment.
ENV CATALOG_UPSTREAM=http://catalog.idp-system.svc.cluster.local:8090
ENV PROMETHEUS_UPSTREAM=http://prometheus-operated.monitoring.svc.cluster.local:9090
# Prometheus proxy rules; see ui.prometheus in the Helm chart values.
ENV PROMETHEUS_ALLOWED_PATHS="query|query_range|series|labels|label/[^/]+/values"
ENV PROMETHEUS_ALLOWED_METHODS="GET POST"
EXPOSE 80
ENTRYPOINT ["/custom-entrypoint.sh"]
CMD ["nginx", "-g", "daemon off;"]
