FROM node:26.13.0-alpine@sha256:8996bb5821d62a8fd9bb27c1cb07462f3fdce6e47d640c6a881ead0b064ce216 AS builder
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:1.31.7-alpine@sha256:892bb2152464f0c5d8972593a5f5afaf8054552580b699d95f8ff4af08c2d33d

COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf.template /etc/nginx/templates/default.conf.template
COPY docker-entrypoint.sh /custom-entrypoint.sh
RUN chmod +x /custom-entrypoint.sh
# Upstreams the nginx config proxies to; override per deployment.
ENV CATALOG_UPSTREAM=http://catalog.idp-system.svc.cluster.local:8090
ENV PROMETHEUS_UPSTREAM=http://prometheus-operated.naira-deps.svc.cluster.local:9090
# Prometheus proxy rules; see ui.prometheus in the Helm chart values.
ENV PROMETHEUS_ALLOWED_PATHS="query|query_range|series|labels|label/[^/]+/values"
ENV PROMETHEUS_ALLOWED_METHODS="GET POST"
EXPOSE 80
ENTRYPOINT ["/custom-entrypoint.sh"]
CMD ["nginx", "-g", "daemon off;"]
