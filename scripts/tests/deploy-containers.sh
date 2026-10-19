#!/usr/bin/env bash
# CI 专用隔离 Compose：真实镜像版本、nginx → API → MySQL，以及故障时的非 2xx。
set -euo pipefail
cd "$(dirname "$0")/../.."
: "${COMMIT:?missing COMMIT}" "${VERSION:?missing VERSION}" "${BUILD_TIME:?missing BUILD_TIME}"
export WMS_API_IMAGE=gowms-api:test WMS_WEB_IMAGE=gowms-web:test
export MYSQL_ROOT_PASSWORD=ci-root-password MYSQL_PASSWORD=ci-app-password
export JWT_SECRET=ci-jwt-secret-0123456789abcdef0123456789abcdef
export WMS_ADMIN_PASSWORD=ci-admin-password WMS_INTEGRATION_API_KEY=ci-integration-key
export WMS_WEB_BIND=127.0.0.1 WMS_WEB_PORT=18081 WMS_API_BIND=127.0.0.1 WMS_API_PORT=18080
override=()
failure_override=$(mktemp)
failure_log=$(mktemp)
compose() {
    docker compose --env-file /dev/null -p gowms-deploy-test -f deploy/docker-compose.yaml -f deploy/docker-compose.release.yaml "${override[@]}" "$@"
}
. ./scripts/lib/deploy-services.sh
cleanup() {
    compose logs --tail=100 || true
    # 仅销毁此脚本创建的 CI 专用 project 数据卷。
    compose down -v --remove-orphans || true
    rm -f -- "$failure_override" "$failure_log"
}
trap cleanup EXIT
deploy_services
base=http://127.0.0.1:18081
curl -fsS --max-time 5 "$base/readyz" | jq -e '.code == 0 and .data.status == "ok"'
curl -fsS --max-time 5 "$base/api/v1/version" | jq -e '.code == 0 | select(.)'
for path in api/v1/version build-info.json; do
    curl -fsS --max-time 5 "$base/$path" | jq -e --arg commit "$COMMIT" --arg version "$VERSION" --arg time "$BUILD_TIME" \
        '(.data // .) | .commit == $commit and .version == $version and .build_time == $time'
done
for image in gowms-api:test gowms-web:test; do
    test "$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$image")" = "$COMMIT"
done
snapshot() {
    local service
    for service in mysql redis wms web; do
        docker inspect --format '{{.Id}} {{.State.StartedAt}} {{.State.Running}}' "$(compose ps -q "$service")"
    done
}
before=$(snapshot)
for service in migrate bootstrap-admin; do
    cat > "$failure_override" <<YAML
services:
  $service:
    entrypoint: ["sh", "-c", "echo 'injected $service failure' >&2; exit 42"]
YAML
    override=(-f "$failure_override")
    status=0
    deploy_services > "$failure_log" 2>&1 || status=$?
    if [ "$status" -ne 42 ]; then
        cat "$failure_log"
        echo "一次性服务未保留失败状态：$service ($status)" >&2
        exit 1
    fi
    grep -q "injected $service failure" "$failure_log"
    [ "$(snapshot)" = "$before" ] || { echo '迁移/初始化失败重建或重启了已有服务' >&2; exit 1; }
    curl -fsS --max-time 5 "$base/readyz" | jq -e '.code == 0 and .data.status == "ok"'
done
override=()
deploy_services
curl -fsS --max-time 5 "$base/readyz" | jq -e '.code == 0 and .data.status == "ok"'
assert_unready() {
    curl -fsS --max-time 5 "$base/healthz" >/dev/null
    status=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' "$base/readyz")
    case "$status" in 502|503|504) ;; *) echo "故障未被就绪探针检出：$status" >&2; exit 1 ;; esac
}
compose stop mysql
assert_unready
compose stop wms
assert_unready
echo '真实容器验证通过：版本一致，迁移/初始化失败保留运行中的服务，MySQL/API 故障被就绪探针检出。'
