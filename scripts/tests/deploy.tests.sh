#!/usr/bin/env bash
# 真实 Git 提交/远端与 jq；只替换 Docker、HTTP 与等待，不启动服务或触及业务库。
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
temp_parent=$(cd "${TMPDIR:-/tmp}" && pwd -P)
work=$(mktemp -d "$temp_parent/gowms-deploy-test.XXXXXX")
cleanup() {
    local resolved
    resolved=$(cd "$work" && pwd -P) || return 1
    [ "$(dirname "$resolved")" = "$temp_parent" ] || return 1
    case "$(basename "$resolved")" in
        gowms-deploy-test.*) rm -rf -- "$resolved" ;;
        *) return 1 ;;
    esac
}
trap cleanup EXIT
mkdir -p "$work/bin" "$work/source/deploy"
git -C "$work/source" init -q -b main
git -C "$work/source" config user.name 'Deployment test'
git -C "$work/source" config user.email 'deploy-test@example.invalid'
printf '.env\n' > "$work/source/.gitignore"
printf 'configuration A\n' > "$work/source/deploy/docker-compose.yaml"
git -C "$work/source" add .
git -C "$work/source" commit -qm '版本 A'
export DEPLOY_REVISION
DEPLOY_REVISION=$(git -C "$work/source" rev-parse HEAD)
printf 'configuration B\n' > "$work/source/deploy/docker-compose.yaml"
git -C "$work/source" commit -qam '版本 B'
git clone -q --bare "$work/source" "$work/origin.git"
git clone -q "$work/origin.git" "$work/server"
printf 'test-only-env\n' > "$work/server/.env"
export DEPLOY_PATH="$work/server" HEALTH_URL='http://127.0.0.1:8081/healthz'
export BUILD_TIME='2026-10-19T00:00:00Z'
export API_DIGEST="sha256:$(printf '%064d' 1)" WEB_DIGEST="sha256:$(printf '%064d' 2)"
export TEST_LOG="$work/docker.log" TEST_HTTP_LOG="$work/http.log" TEST_SCENARIO=ok

cat > "$work/bin/docker" <<'MOCK'
#!/bin/sh
echo "$*" >> "$TEST_LOG"
if [ "$1 $2" = 'image inspect' ]; then
    if [ "$TEST_SCENARIO" = bad_label ]; then echo wrong; else echo "$DEPLOY_REVISION"; fi
else
    [ "$TEST_SCENARIO" != pull_failed ] || exit 1
    [ "$(git rev-parse HEAD)" = "$DEPLOY_REVISION" ] || exit 2
    grep -q 'configuration A' deploy/docker-compose.yaml || exit 3
    [ "$WMS_API_IMAGE" = "ghcr.io/mengw-dev/gowms-api@$API_DIGEST" ] || exit 4
    [ "$WMS_WEB_IMAGE" = "ghcr.io/mengw-dev/gowms-web@$WEB_DIGEST" ] || exit 5
fi
MOCK
cat > "$work/bin/curl" <<'MOCK'
#!/bin/sh
for url; do :; done
echo "$url" >> "$TEST_HTTP_LOG"
commit=$DEPLOY_REVISION
time=$BUILD_TIME
version="sha-$DEPLOY_REVISION"
case "$url" in
    */healthz) echo ok ;;
    */readyz)
        case "$TEST_SCENARIO" in
            backend_down|db_down) exit 22 ;;
            spa) echo '<html>SPA fallback</html>'; exit 0 ;;
            truncated) echo '{"code":0,"data":{"status":"ok"}}'; exit 18 ;;
            duplicate_json) printf '{"code":0,"data":{"status":"bad"}}\n{"code":0,"data":{"status":"ok"}}'; exit 0 ;;
        esac
        echo '{"code":0,"data":{"status":"ok"}}' ;;
    */api/v1/version)
        case "$TEST_SCENARIO" in
            old_backend) commit=old ;;
            dev_build) version=dev ;;
            wrong_time) time=unknown ;;
            business_error) echo '{"code":50000,"data":null}'; exit 0 ;;
            missing_fields) echo '{"code":0,"data":{}}'; exit 0 ;;
        esac
        printf '{"code":0,"data":{"commit":"%s","version":"%s","build_time":"%s"}}' "$commit" "$version" "$time" ;;
    */build-info.json)
        case "$TEST_SCENARIO" in old_frontend) commit=old ;; esac
        printf '{"commit":"%s","version":"%s","build_time":"%s"}' "$commit" "$version" "$time" ;;
    *) exit 2 ;;
esac
MOCK
printf '#!/bin/sh\nexit 0\n' > "$work/bin/sleep"
chmod +x "$work/bin/"*
export PATH="$work/bin:$PATH"

run_case() {
    local scenario=$1 expected=$2 actual=0
    : > "$TEST_LOG"
    : > "$TEST_HTTP_LOG"
    export TEST_SCENARIO=$scenario
    sh "$root/scripts/deploy.sh" > "$work/output.log" 2>&1 || actual=$?
    if { [ "$expected" = pass ] && [ "$actual" -ne 0 ]; } ||
       { [ "$expected" = fail ] && [ "$actual" -eq 0 ]; }; then
        cat "$work/output.log"
        echo "FAIL: $scenario ($actual)" >&2
        exit 1
    fi
    echo "PASS: $scenario"
}

run_case ok pass
[ "$(git -C "$DEPLOY_PATH" rev-parse HEAD)" = "$DEPLOY_REVISION" ]
grep -q '/readyz' "$TEST_HTTP_LOG"
grep -q '/api/v1/version' "$TEST_HTTP_LOG"
grep -q '/build-info.json' "$TEST_HTTP_LOG"
grep -q 'docker-compose.release.yaml' "$TEST_LOG"
grep -q 'up -d --no-build --remove-orphans' "$TEST_LOG"
[ "$(cat "$DEPLOY_PATH/.env")" = test-only-env ]

for scenario in backend_down db_down spa truncated duplicate_json old_backend old_frontend dev_build wrong_time business_error missing_fields; do
    run_case "$scenario" fail
    grep -q 'logs --tail=100' "$TEST_LOG"
done
for scenario in bad_label pull_failed; do
    run_case "$scenario" fail
    if grep -q 'up -d' "$TEST_LOG"; then echo 'Unexpected start after pull/label failure' >&2; exit 1; fi
done

printf 'local changes\n' >> "$DEPLOY_PATH/deploy/docker-compose.yaml"
run_case dirty_tracked fail
grep -q 'local changes' "$DEPLOY_PATH/deploy/docker-compose.yaml"
[ ! -s "$TEST_LOG" ]
git -C "$DEPLOY_PATH" add deploy/docker-compose.yaml
run_case dirty_index fail
[ ! -s "$TEST_LOG" ]
# 只清除测试仓库自己生成的修改，不使用 destructive Git commands。
git -C "$DEPLOY_PATH" show HEAD:deploy/docker-compose.yaml > "$DEPLOY_PATH/deploy/docker-compose.yaml"
git -C "$DEPLOY_PATH" add deploy/docker-compose.yaml
printf 'keep me\n' > "$DEPLOY_PATH/local.txt"
run_case untracked fail
[ "$(cat "$DEPLOY_PATH/local.txt")" = 'keep me' ]
[ ! -s "$TEST_LOG" ]
rm "$DEPLOY_PATH/local.txt"
saved_revision=$DEPLOY_REVISION
export DEPLOY_REVISION=main
run_case invalid_revision fail
[ ! -s "$TEST_LOG" ]
export DEPLOY_REVISION=$saved_revision
export API_DIGEST=sha256:short
run_case invalid_digest fail
[ ! -s "$TEST_LOG" ]
echo '部署回归全部通过（实际 Git A/B 版本切换，Docker/HTTP 为替身）。'
