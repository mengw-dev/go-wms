#!/bin/sh
# 由固定提交的 workflow 通过 SSH 发送执行；不读取服务器上可能过期的脚本。
set -eu

fail() { echo "$*" >&2; exit 1; }
for command in git docker curl jq; do
    command -v "$command" >/dev/null 2>&1 || fail "缺少发布依赖：$command"
done

: "${DEPLOY_PATH:?missing DEPLOY_PATH}"
: "${DEPLOY_REVISION:?missing DEPLOY_REVISION}"
: "${BUILD_TIME:?missing BUILD_TIME}"
: "${API_DIGEST:?missing API_DIGEST}"
: "${WEB_DIGEST:?missing WEB_DIGEST}"
: "${HEALTH_URL:?missing HEALTH_URL}"
case "$DEPLOY_REVISION" in *[!0-9a-f]*|'') fail "提交号必须是完整小写 SHA" ;; esac
[ "${#DEPLOY_REVISION}" -eq 40 ] || fail "提交号必须是完整 40 位 SHA"
for digest in "$API_DIGEST" "$WEB_DIGEST"; do
    value=${digest#sha256:}
    case "$value" in *[!0-9a-f]*|'') fail "镜像 digest 无效" ;; esac
    [ "$digest" != "$value" ] && [ "${#value}" -eq 64 ] || fail "镜像 digest 无效"
done
case "$HEALTH_URL" in http://*/healthz|https://*/healthz) ;; *) fail "HEALTH_URL 必须是 Web 入口的 /healthz 地址" ;; esac
web_url=${HEALTH_URL%/healthz}
version="sha-$DEPLOY_REVISION"
cd "$DEPLOY_PATH"
[ -f .env ] || fail "服务器缺少 $DEPLOY_PATH/.env，请先生成独立密钥"
# 忽略 .env 等已忽略文件；任何已跟踪修改、暂存修改或普通未跟踪文件都先人工处理。
[ -z "$(git status --porcelain --untracked-files=normal)" ] || fail "服务器仓库有未提交文件，停止发布以保留现场修改"
git fetch --no-tags origin "$DEPLOY_REVISION"
git switch --detach "$DEPLOY_REVISION"
[ "$(git rev-parse HEAD)" = "$DEPLOY_REVISION" ] || fail "部署描述提交不匹配"

export WMS_API_IMAGE="ghcr.io/mengw-dev/gowms-api@$API_DIGEST"
export WMS_WEB_IMAGE="ghcr.io/mengw-dev/gowms-web@$WEB_DIGEST"
compose() {
    docker compose --env-file .env -f deploy/docker-compose.yaml -f deploy/docker-compose.release.yaml --profile tls "$@"
}
compose pull migrate bootstrap-admin wms web caddy
for image in "$WMS_API_IMAGE" "$WMS_WEB_IMAGE"; do
    actual=$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$image")
    [ "$actual" = "$DEPLOY_REVISION" ] || fail "镜像 revision 标签与本次提交不匹配：$image"
done
compose up -d --no-build --remove-orphans

get() { curl --fail --silent --show-error --connect-timeout 3 --max-time 5 "$1"; }
check_version() {
    jq -e --arg commit "$DEPLOY_REVISION" --arg version "$version" --arg time "$BUILD_TIME" \
        --argjson api "$1" -s 'length == 1 and (.[0] | if $api then select(.code == 0) | .data else . end |
        .commit == $commit and .version == $version and .build_time == $time)' >/dev/null
}
probe() {
    # 先核对 curl 的退出状态再解析，避免连接中断后的残缺响应被管道掩盖。
    ready=$(get "$web_url/readyz") || return 1
    printf '%s' "$ready" | jq -e -s 'length == 1 and .[0].code == 0 and .[0].data.status == "ok"' >/dev/null || return 1
    backend=$(get "$web_url/api/v1/version") || return 1
    printf '%s' "$backend" | check_version true || return 1
    frontend=$(get "$web_url/build-info.json") || return 1
    printf '%s' "$frontend" | check_version false
}
attempt=1
while [ "$attempt" -le 30 ]; do
    # 同一 Web 入口必须同时满足：后端/DB 就绪、后端版本、前端镜像构建版本。
    # jq 校验响应结构，防止 SPA 回落的 HTML 或静态 200 假通过；不跟随跳转。
    if probe; then
        echo "部署成功：前后端版本 $DEPLOY_REVISION 与数据库就绪检查均通过"
        exit 0
    fi
    [ "$attempt" -eq 30 ] || sleep 5
    attempt=$((attempt + 1))
done
echo "发布就绪或版本检查失败，最近容器状态与日志如下：" >&2
compose ps || true
compose logs --tail=100 || true
exit 1
