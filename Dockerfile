# ---- 构建阶段 ----
FROM golang:1.26.10-alpine AS builder
WORKDIR /src
ENV GOPROXY=https://goproxy.cn,direct
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X gowms/internal/pkg/version.Version=${VERSION} -X gowms/internal/pkg/version.Commit=${COMMIT} -X gowms/internal/pkg/version.BuildTime=${BUILD_TIME}" -o /out/wms ./cmd/wms \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

# ---- 运行阶段 ----
FROM alpine:3.21
ARG COMMIT=unknown
LABEL org.opencontainers.image.revision="${COMMIT}"
RUN apk add --no-cache ca-certificates tzdata wget \
    && addgroup -S app \
    && adduser -S -G app app
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY --from=builder /out/wms ./wms
COPY --from=builder /out/migrate ./migrate
COPY configs ./configs
COPY migrations ./migrations
RUN mkdir -p /app/data/uploads && chown -R app:app /app
USER app
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --retries=5 CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["./wms"]
