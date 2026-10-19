FROM node:22-alpine AS builder
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
RUN npm run build \
    && printf '{"version":"%s","commit":"%s","build_time":"%s"}\n' "$VERSION" "$COMMIT" "$BUILD_TIME" > /app/dist/build-info.json

FROM nginxinc/nginx-unprivileged:1.27-alpine
ARG COMMIT=unknown
LABEL org.opencontainers.image.revision="${COMMIT}"
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder --chown=101:101 /app/dist /usr/share/nginx/html
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --retries=5 CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["nginx", "-g", "daemon off;"]
