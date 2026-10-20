#!/usr/bin/env bash
# 双击运行：脱离终端会话启动前后端，日志写入 dev/logs/
cd "$(dirname "$0")" || exit 1
exec ./scripts/dev-local.sh start