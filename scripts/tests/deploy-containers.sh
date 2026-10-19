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
compose() {
    docker compose --env-file /dev/null -p gowms-deploy-test -f deploy/docker-compose.yaml -f deploy/docker-compose.release.yaml "$@"
}
cleanup() {
    compose logs --tail=100 || true
    # 仅销毁此脚本创建的 CI 专用 project 数据卷。
    compose down -v --remove-orphans || true
}
trap cleanup EXIT
compose up -d --no-build --wait --wait-timeout 180
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
assert_unready() {
    curl -fsS --max-time 5 "$base/healthz" >/dev/null
    status=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' "$base/readyz")
    case "$status" in 502|503|504) ;; *) echo "故障未被就绪探针检出：$status" >&2; exit 1 ;; esac
}
compose stop mysql
assert_unready
compose stop wms
assert_unready
echo '真实容器验证通过：版本一致，MySQL/API 故障被 Web 就绪探针检出。'
