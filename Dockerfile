# ---- 构建阶段 ----
FROM golang:1.26-alpine AS builder
WORKDIR /src
ENV GOPROXY=https://goproxy.cn,direct
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/wms ./cmd/wms \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

# ---- 运行阶段 ----
FROM alpine:3.21
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
