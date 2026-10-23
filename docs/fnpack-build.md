# 飞牛 fnOS 打包指南（fnpack）

本项目以**原生应用**方式发布飞牛 fnOS 安装包（`.fpk`），不是 Docker 镜像。打包实际分成两件事：

1. **准备负载**：构建前端静态文件，交叉编译 Linux 二进制；
2. **封装安装包**：按 fnOS 目录规范组装工程，再交给 `fnpack build` 生成 `.fpk`。

仓库里的 `scripts/build-fnpack-docker.sh` 已经把这两件事串成一条命令。

## 0. 最短路径

```bash
# 前置：fnpack 1.2.3 + Docker Desktop 已启动
cd /path/to/bill

./scripts/build-fnpack-docker.sh amd64

# 产物
# release/v0.1.4/techfunway-bill_v0.1.4_amd64.fpk
```

安装到飞牛：

```bash
appcenter-cli install-fpk techfunway-bill_v0.1.4_amd64.fpk
# 或在「应用中心 → 手动安装」选择该文件
```

## 1. 产物结构

`.fpk` 本质是一个 gzip 压缩的 tar 包：

```text
techfunway-bill.fpk
├── manifest              # 应用元数据：名称、版本、平台、端口、changelog
├── ICON.PNG              # 应用图标
├── ICON_256.PNG
├── app.tgz               # 应用负载（解开后是 bill 二进制 + ui/ 前端）
├── cmd/                  # 生命周期脚本
│   ├── main              # start / stop / status / restart
│   ├── port_config       # 端口解析与持久化
│   ├── install_init      # 安装前
│   ├── install_callback  # 安装后（写入端口）
│   ├── upgrade_init / upgrade_callback
│   ├── uninstall_init / uninstall_callback
│   └── config_init / config_callback
├── config/
│   ├── privilege         # run-as: package
│   └── resource          # data-share 共享目录声明
└── wizard/               # 安装、升级、运行配置表单
    ├── install           # 默认端口 8907
    ├── upgrade
    ├── config
    └── uninstall
```

`fnpack` 只负责**校验并封装**这个目录，不负责编译代码。所以打包脚本先要把 `bill` 二进制和 `ui/` 前端准备好。

## 2. 前置条件

| 工具 | 要求 | 检查命令 |
| --- | --- | --- |
| `fnpack` | 1.2.3 | `fnpack --help` |
| Docker | Desktop 已启动（用于交叉编译） | `docker info` |
| Node.js / npm | 能构建 Vite 项目（Node 20+） | `node -v && npm -v` |
| Go | 仅 `make fnpack` 本机编译路线需要 | `go version` |
| musl-cross | 仅 `make fnpack` 需要 | `x86_64-linux-musl-gcc --version` |

安装 `fnpack`（macOS Apple Silicon）：

```bash
mkdir -p ~/.local/bin
curl -sSL https://static2.fnnas.com/fnpack/fnpack-1.2.3-darwin-arm64 \
  -o ~/.local/bin/fnpack
chmod +x ~/.local/bin/fnpack
export PATH="$HOME/.local/bin:$PATH"
fnpack --help
```

其他平台请下载对应的 fnpack 发行文件。确认版本为 `1.2.3`（`fnpack --help` 第一行会打印版本）。

## 3. 推荐流程：Docker 脚本

### 3.1 发版前准备

1. 修改 [VERSION](../VERSION)，例如 `v0.1.5`。这个值会同时写入二进制 ldflags 和 fpk 的 manifest。
2. 更新 [fnpack/manifest](../fnpack/manifest) 里的 `changelog`（脚本不会自动生成更新日志）。
3. 如涉及应用名、端口变更，同步检查 `app.json`、`manifest`、`wizard/install`、`app/ui/config`。
4. 确认工作区状态。**打包会把当前工作区代码一起打进去**，包括未提交改动：

```bash
git status --short
git log --oneline -1
```

### 3.2 执行打包

```bash
# 只打 x86_64（大多数飞牛 NAS）
./scripts/build-fnpack-docker.sh amd64

# 只打 ARM64
./scripts/build-fnpack-docker.sh arm64

# 两个架构都打（默认）
./scripts/build-fnpack-docker.sh all
```

首次运行需要构建带 gcc 的基础镜像，两个架构合计约 20 分钟；之后镜像和 Go 缓存会复用，通常几分钟内完成。

### 3.3 脚本内部做了什么

脚本输出四个阶段：`[1/4]` 前端、`[2/4]` 编译、`[3/4]` 组装、`[4/4]` 封装。

#### 第 1 步：构建前端（fnOS base path）

```bash
rm -rf web/dist
VITE_FNOS_APP=true VITE_BASE_PATH="/app/techfunway-bill/" \
  npm --prefix web ci --loglevel=error --no-audit --no-fund
VITE_FNOS_APP=true VITE_BASE_PATH="/app/techfunway-bill/" \
  npm --prefix web run build --silent
```

要点：

- `VITE_BASE_PATH` 必须是 `/app/<appname>/`，本项目为 `/app/techfunway-bill/`。该值决定 `index.html` 里 JS/CSS 的引用路径，写错会导致安装后网关入口白屏。
- 脚本会检查 `web/dist/index.html` 存在且文件数不少于 20 个。Vite 会先清空 `dist`，在受限环境里删除可能被拦截，导致“构建成功但目录是空的”，所以这里必须硬校验。
- 构建完把 `web/dist` 复制到 `server/static/dist`，供 Go 服务读取。

#### 第 2 步：交叉编译 Linux 二进制

基础镜像来自 [deploy/Dockerfile.gcc](../deploy/Dockerfile.gcc)，基于 `golang:1.26-alpine` 并安装 `gcc`、`musl-dev`，首次使用时构建为 `bill-gcc-<arch>:1.0`。

```bash
docker run --rm \
  --platform linux/amd64 \
  -v "$PWD/server:/src" \
  -w /src \
  -e "GOPROXY=https://goproxy.cn,direct" \
  -e "LDFLAGS=-X smallgo/server/version.Version=v0.1.4 ..." \
  bill-gcc-amd64:1.0 \
  sh -c 'CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build \
         -ldflags "$LDFLAGS -extldflags -static" -o bill-linux-amd64 .'
```

`LDFLAGS` 由脚本注入版本号、构建时间、Git commit 和应用名，最终可通过 `/api/version` 查看。

要点：

- **必须 `CGO_ENABLED=1`**。项目使用 go-sqlite3，关闭 cgo 后二进制启动即报 `go-sqlite3 requires cgo to work. This is a stub`。
- 使用 musl 静态链接（`-extldflags -static`），产物不依赖目标机 glibc，可直接在 fnOS 上运行。
- `ldflags` 注入版本、构建时间、Git commit、应用名，可在 `/api/version` 查看。

#### 第 3 步：组装 fnOS 应用目录

在 `release/<VERSION>/techfunway-bill_<arch>/` 下临时组装：

| 来源 | 目标 | 说明 |
| --- | --- | --- |
| `fnpack/cmd/` | `cmd/` | 生命周期脚本 |
| `fnpack/config/` | `config/` | 权限与共享目录 |
| `fnpack/wizard/` | `wizard/` | 安装/升级/配置表单 |
| `fnpack/app/ui/` | `app/ui/` | 前端入口配置 |
| `fnpack/ICON*.PNG` | 根目录 | 图标 |
| `server/bill-linux-<arch>` | `app/bill` | 第 2 步的二进制，加执行权限 |
| `server/static/dist/*` | `app/ui/` | 前端静态文件 |
| `fnpack/manifest` | `manifest` | 按架构和版本生成，不改源文件 |

同时会：

- 把 `platform` 改为 `x86`（amd64）或 `arm`（arm64）；
- 把 `version` 改为 `VERSION` 去掉前缀 `v` 的值；
- 删除 `.DS_Store` 等 macOS 元数据，避免混进安装包。

#### 第 4 步：`fnpack build`

```bash
cd release/v0.1.4/techfunway-bill_amd64
fnpack build
# Verifying files...
# Packing in progress...
# Packing successfully. -> techfunway-bill.fpk
```

`fnpack` 会校验 manifest、脚本、图标和目录结构，把 `app/` 打成 `app.tgz`，再与 `cmd/`、`config/`、`wizard/` 一起封装为 `<appname>.fpk`。生成的包随后被移动到 `release/<VERSION>/techfunway-bill_<VERSION>_<arch>.fpk`，临时目录会被清理。

### 3.4 归档结果

```text
release/v0.1.4/
├── techfunway-bill_v0.1.4_amd64.fpk
└── screenshots/            # 从 docs/screenshots 复制，便于发布页使用
```

## 4. 产物校验

发版前至少做下面四项。

### 4.1 校验和

```bash
cd release/v0.1.4
shasum -a 256 techfunway-bill_v0.1.4_amd64.fpk
# 或 Linux: sha256sum techfunway-bill_v0.1.4_amd64.fpk
```

### 4.2 检查 manifest 与包结构

```bash
tmp=$(mktemp -d)
tar xzf techfunway-bill_v0.1.4_amd64.fpk -C "$tmp"
grep -E '^(appname|version|platform|service_port|os_min_version)' "$tmp/manifest"
tar tzf techfunway-bill_v0.1.4_amd64.fpk | head -20
rm -rf "$tmp"
```

预期：`appname = techfunway-bill`、`version = 0.1.4`、`platform = x86`。

### 4.3 检查二进制与前端路径

```bash
tmp=$(mktemp -d)
tar xzf techfunway-bill_v0.1.4_amd64.fpk -C "$tmp"
tar xzf "$tmp/app.tgz" -C "$tmp"
file "$tmp/bill"
# 期望：ELF 64-bit LSB executable, x86-64, statically linked
grep -o '/app/techfunway-bill/assets/[^"]*' "$tmp/ui/index.html" | head -3
rm -rf "$tmp"
```

### 4.4 容器内启动自检

在 macOS 上无法直接运行 Linux 二进制，可以用 Docker 跑一次：

```bash
cd /path/to/bill
mkdir -p .smoke
tar xzf release/v0.1.4/techfunway-bill_v0.1.4_amd64.fpk -C .smoke app.tgz
tar xzf .smoke/app.tgz -C .smoke
rm .smoke/app.tgz

docker rm -f bill-smoke >/dev/null 2>&1
docker run -d --name bill-smoke --platform linux/amd64 \
  -v "$PWD/.smoke:/app" -p 18907:18907 \
  bill-gcc-amd64:1.0 \
  /app/bill -data-dir /app/data -web-dir /app/ui -port 18907 -logmode release

sleep 3
curl -s -o /dev/null -w 'health %{http_code}\n' http://127.0.0.1:18907/api/health
curl -s -o /dev/null -w 'root   %{http_code}\n' http://127.0.0.1:18907/
curl -s http://127.0.0.1:18907/api/version; echo
# 期望：HTTP 200 / 200，version 与 Git commit 正确

docker rm -f bill-smoke
rm -rf .smoke
```

ARM 包把 `--platform` 和镜像换成 `linux/arm64` / `bill-gcc-arm64:1.0`。

## 5. 安装、升级与数据

### 安装

应用中心 → 手动安装，或 SSH 到 NAS：

```bash
appcenter-cli install-fpk techfunway-bill_v0.1.4_amd64.fpk
```

- x86 NAS 装 `amd64`，ARM NAS 装 `arm64`，架构不匹配会安装失败。
- 安装向导可修改服务端口（默认 `8907`，范围 1024–65535），端口会持久化，后续启动沿用。
- 安装后在桌面注册直接 URL 入口；另有一个隐藏的网关入口（`/app/techfunway-bill/fnos-entry.html`），NAS 账号登录通过该网关完成。

### 数据位置

- 数据目录由 fnOS 注入的 `TRIM_PKGVAR` 决定：数据库、上传文件、日志都在这里。
- 日志：`${TRIM_PKGVAR}/logs/YYYYMM/info_YYYYMMDD.log`。
- 端口记录：`${TRIM_PKGETC}/service_port`。

升级时替换 fpk 即可，`TRIM_PKGVAR` 下的数据会保留。卸载时默认也保留数据；只有在卸载向导里选择“删除用户数据”，`cmd/uninstall_callback` 才会清空 `TRIM_PKGVAR`。

### 升级

重新打包并安装同 `appname` 的更高版本 fpk，fnOS 会走 `upgrade_init` / `upgrade_callback`。升级向导里端口留空表示保持当前端口。

## 6. 备选：本机 `make fnpack`

```bash
brew install FiloSottile/musl-cross/musl-cross
make fnpack
```

对应 [scripts/build-fnpack.sh](../scripts/build-fnpack.sh)：

- 用本机 `x86_64-linux-musl-gcc` / `aarch64-linux-musl-gcc` 编译，不依赖 Docker；
- 一次打包 amd64 + arm64；
- 结束时恢复 `fnpack/manifest` 备份。

注意：musl-cross 的 brew tap 走 GitHub，国内网络经常失败；中途退出还可能在 `fnpack/` 下留下 `manifest.bak`。这个文件只是备份，可直接删除，不影响打包。日常发版建议用 Docker 脚本。

## 7. 手工组装（特殊场景）

需要改包结构或跳过脚本时，可以手工准备目录后直接调用 fnpack：

```bash
# 1. 准备前端与二进制（同脚本第 1、2 步）
# 2. 按第 3 步的目录结构组装到任意目录，例如 /tmp/techfunway-bill
# 3. 在目录内执行：
cd /tmp/techfunway-bill
fnpack build

# 或者不切换目录：
fnpack build -d /tmp/techfunway-bill
```

从零创建模板工程可以用 `fnpack create`，本项目已有完整模板，不需要。

## 8. 关键配置对照

| 配置 | 所在文件 | 说明 |
| --- | --- | --- |
| 应用标识 `appname` | `fnpack/manifest`、`app.json` | 决定 fpk 文件名、安装目录、网关路径；改动需全链路同步 |
| 版本 | `VERSION` | 打包时自动写入 manifest 与二进制；manifest 里的值会被覆盖 |
| 更新日志 | `fnpack/manifest` 的 `changelog` | 手写，发版时更新 |
| 架构 | 脚本参数 | `amd64 → x86`，`arm64 → arm` |
| 默认端口 | `fnpack/wizard/install` | `initValue: 8907`，安装时可改 |
| 运行时端口 | `fnpack/cmd/port_config` | 读取 `TRIM_PKGETC/service_port`，回退默认值 |
| 网关入口 | `fnpack/app/ui/config` | `gatewayPrefix: /app/techfunway-bill`、`gatewaySocket: app.sock` |
| 前端 base path | 打包命令的 `VITE_BASE_PATH` | 必须等于 `/app/<appname>/` |
| 运行用户 | `fnpack/config/privilege` | `run-as: package` |
| 共享目录 | `fnpack/config/resource` | 声明 `techfunway-bill` 与 `techfunway-bill/data` |
| 服务端口声明 | `fnpack/manifest` 的 `service_port` | `8907`；`checkport = false`，由网关转发 |

## 9. 常见问题

**`fnpack: command not found`**

把 `~/.local/bin` 加入 `PATH`，或把 fnpack 放到 `/usr/local/bin`。

**`docker: command not found` / 无法连接 daemon**

启动 Docker Desktop，等待状态变为 running。

**`Error: x86_64-linux-musl-gcc not found`**

这是 `make fnpack` 路线的报错。改装 musl-cross，或改用 `./scripts/build-fnpack-docker.sh`。

**启动报 `go-sqlite3 requires cgo to work. This is a stub`**

编译时关了 cgo。必须 `CGO_ENABLED=1`，并使用带 gcc 的镜像。

**安装成功但打开白屏**

大概率是前端 base path 与 `appname` 不一致，或 `web/dist` 构建不完整。检查 `ui/index.html` 是否引用 `/app/techfunway-bill/assets/...`，然后重新打包。

**安装后服务起不来**

查看 `${TRIM_PKGVAR}/logs/` 下的日志与 `app.pid`；检查端口是否被占用。manifest 里 `checkport = false`，端口冲突不会在安装阶段被拦截。

**ARM NAS 安装失败**

装了 amd64 包。改打 `arm64`。

**打包后多出 `fnpack/manifest.bak`**

`make fnpack` 中断留下的备份，内容与 `fnpack/manifest` 相同，可删除。

## 10. 发版检查清单

- [ ] `VERSION` 已更新
- [ ] `fnpack/manifest` 的 `changelog` 已更新
- [ ] 代码已提交，`git status` 干净（或确认未提交改动也要发）
- [ ] `./scripts/build-fnpack-docker.sh amd64`（按需加 `arm64`）成功
- [ ] manifest 的 `version` / `platform` 正确
- [ ] 二进制为对应架构的静态 ELF
- [ ] `ui/index.html` 引用 `/app/techfunway-bill/`
- [ ] `/api/health`、`/api/version` 自检通过
- [ ] 记录 SHA-256
- [ ] 在目标架构 NAS 上实装验证，升级后数据仍在
