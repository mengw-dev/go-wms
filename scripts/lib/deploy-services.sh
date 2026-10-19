#!/bin/sh
# 调用方提供 compose 函数；CI 和服务器使用相同的启动顺序。
deploy_services() {
    deploy_phase='基础服务就绪'
    # 已有 MySQL/Redis 不因发布应用而重建；基础服务升级另行安排维护。
    compose up -d --no-build --no-recreate --wait --wait-timeout 120 mysql redis || return $?
    deploy_phase='数据库迁移'
    # 只操作一次性容器，迁移失败时不停止、重建或启动应用容器。
    compose up --no-deps --no-build --exit-code-from migrate migrate || return $?
    deploy_phase='管理员初始化'
    compose up --no-deps --no-build --exit-code-from bootstrap-admin bootstrap-admin || return $?
    deploy_phase='应用切换'
    # 前两步已完成，不再让 depends_on 隐式重新执行迁移。
    compose up -d --no-deps --no-build --wait --wait-timeout 180 --remove-orphans wms web "$@" || return $?
}
