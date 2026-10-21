#!/usr/bin/env bash
# 用 Docker 做 linux 交叉编译，产出飞牛 fnOS 应用包（.fpk）。
#
# 为什么不用 make fnpack：
#   原脚本依赖 musl-cross 交叉编译器（x86_64-linux-musl-gcc / aarch64-linux-musl-gcc），
#   只能 brew tap filosottile/musl-cross —— 而该tap 走 GitHub，国内被墙（CONNECT tunnel failed 502），
#   且沙箱不允许写 /opt/homebrew。改用容器内 gcc 编译，无需在本机装任何交叉编译器。
#
# 为什么不能 CGO_ENABLED=0：
#   go-sqlite3 强制要求 cgo。实测 CGO_ENABLED=0 编译出的产物启动即报：
#   "Binary was compiled with 'CGO_ENABLED=0', go-sqlite3 requires cgo to work. This is a stub"
#
# 前置依赖：
#   1. fnpack：curl -sSL https://static2.fnnas.com/fnpack/fnpack-1.2.3-darwin-arm64 \
#              -o ~/.local/bin/fnpack && chmod +x ~/.local/bin/fnpack
#   2. Docker（用于交叉编译）
#
# 用法：
#   ./scripts/build-fnpack-docker.sh# 同时打包 amd64 + arm64（首次约 20 分钟，之后约 3 分钟）
#   ./scripts/build-fnpack-docker.sh amd64        # 只打包 amd64
set -uo pipefail

TARGET_ARCH="${1:-all}"
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR" || exit 1

DOCKER_CFG_DIR="${DOCKER_CFG_DIR:-$HOME/.cache/bill-fnpack}"
GOCACHE_VOL="bill-fnpack-gocache"
GOMOD_VOL="bill-fnpack-gomod"

export PATH="$HOME/.local/go-install/go/bin:$HOME/.local/bin:$PATH"
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
export GOSUMDB="${GOSUMDB:-sum.golang.google.cn}"

command -v fnpack >/dev/null 2>&1 || {
  echo "fnpack not found. install with:" >&2
  echo "  curl -sSL https://static2.fnnas.com/fnpack/fnpack-1.2.3-darwin-arm64 -o ~/.local/bin/fnpack" >&2
  echo "  chmod +x ~/.local/bin/fnpack" >&2
  exit 1
}
command -v docker >/dev/null 2>&1 || { echo "docker not found" >&2; exit 1; }

APP_NAME=$(python3 -c "import json; print(json.load(open('app.json'))['appname'])")
FNOS_PKG_NAME=$(awk -F'=' '/^appname/ {gsub(/^[ \t]+|[ \t]+$/, "", $2); print $2}' fnpack/manifest)
VERSION=$(tr -d '\n' < VERSION)
BUILD_TIME=$(date +%Y-%m-%dT%H:%M:%S)
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS="-X smallgo/server/version.Version=${VERSION}"
LDFLAGS="$LDFLAGS -X smallgo/server/version.BuildTime=${BUILD_TIME}"
LDFLAGS="$LDFLAGS -X smallgo/server/version.GitCommit=${GIT_COMMIT}"
LDFLAGS="$LDFLAGS -X smallgo/server/version.AppName=bill"

BUILD_DIR="release/${VERSION}"
mkdir -p "$BUILD_DIR" "$DOCKER_CFG_DIR"

echo "==> [1/4] build frontend (fnOS base path)"
# vite 的 emptyDir 会先删空 dist 再写入新产物。在受限环境里删除可能被拦，
# 导致构建"成功"但dist 只剩零星文件，打出来的 fpk 能装却打开白屏。
# 所以先手工清空，并校验产物完整性，不合格就直接失败而不是打坏包。
rm -rf web/dist 2>/dev/null || true
VITE_FNOS_APP=true VITE_BASE_PATH="/app/${FNOS_PKG_NAME}/" npm --prefix web ci --loglevel=error --no-audit --no-fund
VITE_FNOS_APP=true VITE_BASE_PATH="/app/${FNOS_PKG_NAME}/" npm --prefix web run build --silent

FE_COUNT=$(find web/dist -type f 2>/dev/null | wc -l | tr -d ' ')
if [ ! -f web/dist/index.html ] || [ "${FE_COUNT}" -lt 20 ]; then
  echo "ERROR: frontend build incomplete (only ${FE_COUNT} files, expected 20+)." >&2
  echo "       web/dist was emptied but not repopulated -- likely blocked by a" >&2
  echo "       delete guard. Run 'rm -rf web/dist' manually, then retry:" >&2
  echo "         cd web && VITE_FNOS_APP=true VITE_BASE_PATH=\"/app/${FNOS_PKG_NAME}/\" npm run build" >&2
  exit 1
fi
echo "    frontend ok: ${FE_COUNT} files"

if [ -d docs/screenshots ]; then
  mkdir -p "${BUILD_DIR}/screenshots"
  cp docs/screenshots/*.png "${BUILD_DIR}/screenshots/" 2>/dev/null
fi

rm -rf server/static/dist
cp -r web/dist server/static/dist

# 预构建带 gcc 的基础镜像。首次耗时较长（apk 下载慢，约 8-15 分钟/架构），
# 之后本地已有镜像则直接复用，后续打包只要几十秒。
ensure_gcc_image() {
  local arch="$1" image="bill-gcc-${arch}:1.0"
  if docker image inspect "$image" >/dev/null 2>&1; then
    echo "    reuse image $image"
    return 0
  fi
  echo "    building $image (first time, apk download is slow)"
  cp "$ROOT_DIR/deploy/Dockerfile.gcc" "$DOCKER_CFG_DIR/Dockerfile.gcc"
  DOCKER_CONFIG="${DOCKER_CFG_DIR}/docker-cfg" \
    docker build --platform "linux/${arch}" \
      -f "$DOCKER_CFG_DIR/Dockerfile.gcc" \
      -t "$image" "$DOCKER_CFG_DIR" || return 1
  return 0
}

build_one() {
  local arch="$1" fnos_platform="$2"
  local image="bill-gcc-${arch}:1.0"
  local bin="bill-linux-${arch}"

  echo "==> [2/4] compile linux/${arch}"
  ensure_gcc_image "$arch" || {
    echo "failed to prepare gcc image for ${arch}" >&2
    return 1
  }

  docker run --rm \
    --platform "linux/${arch}" \
    -v "${ROOT_DIR}/server:/src" \
    -v "${GOCACHE_VOL}:/root/.cache/go-build" \
    -v "${GOMOD_VOL}:/go/pkg/mod" \
    -w /src \
    -e "GOPROXY=https://goproxy.cn,direct" \
    -e "GOSUMDB=sum.golang.google.cn" \
    -e "GOFLAGS=-mod=mod" \
    -e "LDFLAGS=${LDFLAGS}" \
    "$image" \
    sh -c "CGO_ENABLED=1 GOOS=linux GOARCH=${arch} go build -ldflags \"\$LDFLAGS -extldflags -static\" -o \"${bin}\" ." || {
      echo "compile failed for ${arch}" >&2
      return 1
    }

  if [ ! -f "server/${bin}" ]; then
    echo "binary not produced for ${arch}" >&2
    return 1
  fi
  echo "    binary: $(ls -lh "server/${bin}" | awk '{print $5}')"

  echo "==> [3/4] assemble fnOS app dir (${fnos_platform})"
  local pack="${BUILD_DIR}/${APP_NAME}_${arch}"
  rm -rf "${pack}"
  mkdir -p "${pack}"

  cp -r fnpack/cmd "${pack}/"
  cp -r fnpack/config "${pack}/"
  cp -r fnpack/wizard "${pack}/"
  mkdir -p "${pack}/app"
  cp -r fnpack/app/ui "${pack}/app/"
  cp fnpack/ICON.PNG "${pack}/"
  cp fnpack/ICON_256.PNG "${pack}/"

  cp "server/${bin}" "${pack}/app/bill"
  chmod +x "${pack}/app/bill"
  rm -f "server/${bin}"
  cp -r server/static/dist/* "${pack}/app/ui/"

  sed "s/^platform.*/platform              = ${fnos_platform}/" fnpack/manifest > "${pack}/manifest"
  sed -i.bak "s/^version.*/version               = ${VERSION#v}/" "${pack}/manifest"
  rm -f "${pack}/manifest.bak"
  find "${pack}" -name '.DS_Store' -delete

  echo "==> [4/4] fnpack build"
  (cd "${pack}" && fnpack build) || return 1

  if [ -f "${pack}/${FNOS_PKG_NAME}.fpk" ]; then
    mv "${pack}/${FNOS_PKG_NAME}.fpk" "${BUILD_DIR}/${FNOS_PKG_NAME}_${VERSION}_${arch}.fpk"
    rm -rf "${pack}"
    echo "    built: ${FNOS_PKG_NAME}_${VERSION}_${arch}.fpk"
  else
    echo "fpk not produced for ${arch}" >&2
    return 1
  fi
}

case "$TARGET_ARCH" in
  amd64) build_one amd64 x86 ;;
  arm64) build_one arm64 arm ;;
  all)
    build_one amd64 x86
    build_one arm64 arm
    ;;
  *)
    echo "usage: $0 [amd64|arm64|all]" >&2
    exit 2
    ;;
esac

echo ""
echo "done. packages in ${BUILD_DIR}/"
ls -lh "${BUILD_DIR}"/*.fpk 2>/dev/null
echo ""
echo "install on fnOS:"
echo "  appcenter-cli install-fpk ${FNOS_PKG_NAME}_${VERSION}_<arch>.fpk"