#!/usr/bin/env bash
# 双击运行：停止本地前后端服务
cd "$(dirname "$0")" || exit 1
exec ./scripts/dev-local.sh stop