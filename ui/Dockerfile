FROM node:22-alpine AS builder
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:1.30-alpine

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
