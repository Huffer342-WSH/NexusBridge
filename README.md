# NexusBridge

NexusBridge 是一个面向 NexusPHP 站点和 qBittorrent 的跨平台管理工具，用于检索与持久化种子、同步下载状态、按规则自动添加任务，以及从保留的本地文件恢复被误删的 qB 任务。

项目提供两种发布形态：

- **NexusBridge Desktop**：自带桌面窗口，适合在 Windows 或 Linux 桌面环境直接使用。
- **NexusBridge WebUI/CLI**：以常驻服务或命令行方式运行，适合 NAS、服务器和远程管理。

两种形态共用同一套 Go 业务层、SQLite 数据和 Vue WebUI。

## 主要功能

### 站点与种子管理

- 通过 JSON 站点定义适配 NexusPHP 站点，站点运行时配置来自 `sites_dir`。
- 抓取并持久化种子列表、详情和 `.torrent` 文件。
- 将站点种子与 qBittorrent 任务状态同步到本地 SQLite。
- 在 WebUI 中按站点、站点分类、促销和 qB 任务状态筛选媒体，查看下载状态并启动或停止 qB 任务。
- 从媒体页或文件页播放 NexusBridge 所在机器上的源文件，支持浏览器 Range 请求和 MKV 内嵌文本字幕，不进行转码。
- 可把多个本机目录组成电视剧、动漫或系列；按需扫描视频、恢复最后选集，并可选择启用保守的集数识别。
- 文件浏览器和播放列表按需显示视频缩略图或原图片预览；视频缩略图由 FFprobe/FFmpeg 首次请求时生成并缓存。
- 删除 qB 任务时可明确选择保留下载文件或同时删除文件，默认保留文件。
- WebUI 可实时查看当前进程最近日志，并在卡片消息与带等级颜色的纯文本视图之间切换。

### 规则与订阅自动化

- 筛选规则支持站点、分类、标签、关键词、促销、大小、做种/下载/完成数和发布时间等条件。
- 订阅可为命中的种子设置 qB 分类、保存路径、标签、名称、暂停状态和配额。
- 支持手动预览、批量确认和周期执行，预览会展示最终下载计划及阻塞原因。
- 站点周期和订阅默认关闭，需要用户显式启用。

### 文件浏览与 qB 任务恢复

- “文件”页可浏览 NexusBridge 所在机器的文件与目录，并标记已归属 qB 的内容。
- qB 任务被误删但本地数据仍存在时，可从本地数据库、站点搜索或直接 torrent URL 查找恢复候选。
- 三种来源都要求所选文件或目录与 torrent 的完整文件大小多重集合一致；数据库使用倒排索引缩小候选，旧数据由用户在文件页手动重建一次，之后保存、覆盖或删除 torrent BLOB 时自动维护。
- 恢复不会移动、复制或重命名磁盘文件；任务暂停且跳过初始校验添加，再通过 qB `renameFile` 把任务路径映射到现有结构。只有强制校验确认 100% 完成才开始做种；失败时保留唯一暂停任务，用户可选择仍然启动或仅删除任务并保留文件。
- 高级扫描先只读返回候选；网页搜索默认关闭，用户可显式启用。预览后可一次确认串行自动恢复全部唯一候选，失败项单独报告且不会阻断后续任务。

> 当前站点恢复会合并最多 5 个名称变体的第一页结果，并先按站点展示总大小的容差缩小 torrent 文件下载范围；多页检索与搜索记录持久化尚待完成。

完整流程、分类路径继承和失败处理见 [qB 任务恢复文档](docs/modules/recovery.md)。版本变化见 [CHANGELOG.md](CHANGELOG.md)。

## 面向用户：安装与使用

### 选择版本

| 使用场景           | 推荐版本  | 发布文件                                                                               |
| ------------------ | --------- | -------------------------------------------------------------------------------------- |
| Windows 桌面       | Desktop   | `nexusbridge-desktop-windows-amd64.zip`                                                |
| Linux 桌面         | Desktop   | `nexusbridge-desktop-linux-amd64.tar.gz` 或 `nexusbridge-desktop-linux-aarch64.tar.gz` |
| Windows 服务器/NAS | WebUI/CLI | `nexusbridge-webui-windows-amd64.zip`                                                  |
| Linux 服务器/NAS   | WebUI/CLI | `nexusbridge-webui-linux-amd64.tar.gz` 或 `nexusbridge-webui-linux-aarch64.tar.gz`     |

`aarch64` 对应 Go 的 `arm64` 架构名。Linux 桌面版还需要系统安装 GTK4 和 WebKitGTK 6.0 运行库。

### 安装

1. 从发布页下载对应系统、架构和产品形态的压缩包。
2. 解压到独立目录。
3. 保留压缩包中的 `data/config.example.json` 和 `nexusbridge.bootstrap.example.json`，它们是配置参考。
4. 按下文启动 Desktop 或 WebUI/CLI 版。

#### Desktop 桌面版

Windows 直接运行：

```powershell
.\nexusbridge-desktop.exe
```

Linux 在安装 GTK4 和 WebKitGTK 6.0 运行库后执行：

```sh
chmod +x nexusbridge-desktop
./nexusbridge-desktop
```

#### WebUI/CLI 服务版

Windows：

```powershell
.\nexusbridge-webui.exe serve
```

Linux：

```sh
chmod +x nexusbridge-webui
./nexusbridge-webui serve
```

服务默认监听 `0.0.0.0:8090`。发布二进制已内嵌 WebUI，不需要额外携带 `webui/dist/`。

#### Docker 一体化版

一体化镜像同时包含 qBittorrent-nox、NexusBridge 和 mihomo，支持 `linux/amd64` 与 `linux/arm64`。所有应用配置统一持久化到 `/config`，下载数据使用 `/downloads`：

```sh
docker compose -f docker/compose.yaml up -d
docker compose -f docker/compose.yaml logs -f nexusbridge
```

默认端口为 mihomo 代理 `7890`、mihomo WebUI `9090`、qB WebUI `8080`、NexusBridge `8090`、BT `16881/tcp+udp`。qB 首次启动生成的 `admin` 临时密码会出现在容器日志中；登录后应立即修改。配置目录结构、mihomo 控制器安全设置和多架构构建方法见 [Docker 说明](docker/README.md)。

### 数据目录与配置

正式构建默认使用系统用户配置目录：

- Windows：`%AppData%\NexusBridge`
- Linux：`$XDG_CONFIG_HOME/NexusBridge` 或 `~/.config/NexusBridge`

首次启动会创建主配置、SQLite 数据库和日志目录，并安装内置站点定义。配置字段说明见 [docs/config.md](docs/config.md)。

如需便携数据目录，将 [nexusbridge.bootstrap.example.json](nexusbridge.bootstrap.example.json) 复制为可执行文件旁的 `nexusbridge.bootstrap.json`。相对 `data_dir` 以可执行文件目录为基准。

也可使用：

- `NEXUSBRIDGE_DATA_DIR`：临时覆盖默认数据目录。
- `--config path/to/config.json`：显式选择主配置，并以该文件所在目录作为相对路径的数据根目录。

> 当前规则模型升级不保留旧 SQLite 数据库兼容性。从旧版本升级时，请先备份数据，再按需要重新初始化数据库。

### 基本使用流程

1. 在“设置 / 站点”配置站点定义与访问信息。
2. 在“设置 / qBittorrent”配置 qB WebAPI 连接。
3. 抓取站点数据，在“媒体”页查看种子和 qB 状态。
4. 根据需要创建筛选规则和订阅，先预览命中与下载计划，再执行或启用周期。
5. 需要恢复任务时，在“文件”页选择已保留的文件或目录，先查看候选和诊断；也可扫描当前目录，选择是否启用网页搜索后一次确认自动恢复全部唯一候选。

不要把站点 Cookie、qB 密码、API Key 或 passkey 提交到仓库。

## 面向开发者：开发与构建

### 开发环境

- Go 1.25+
- Node.js 22+
- pnpm 11+
- Wails v3 CLI `v3.0.0-alpha2.117`

初始化依赖：

```sh
go mod download
cd webui
pnpm install
cd ..
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-alpha2.117
```

开发运行和测试默认使用仓库下的 `data/`。可从 [data/config.example.json](data/config.example.json) 开始配置；不要把真实凭据写入该示例。

### 开发运行

#### WebUI/CLI 服务版

使用两个终端。第一个启动后端：

```sh
go run ./cmd/nexusbridge serve
```

第二个启动 Vite：

```sh
cd webui
pnpm run dev
```

后端默认监听 `0.0.0.0:8090`；Vite 默认使用 `http://127.0.0.1:5173`，并代理 `/api` 和 `/rss`。

#### Desktop 桌面版

```sh
wails3 dev
```

Wails 会启动桌面壳和前端开发服务。桌面入口、资源和平台任务位于 `desktop/`。

### CLI 调试

```sh
go run ./cmd/nexusbridge config check
go run ./cmd/nexusbridge fetch <site>
go run ./cmd/nexusbridge qb sync
go run ./cmd/nexusbridge organize pending
```

`serve` 会启动已显式启用的站点计划；计划无论是否存在启用订阅都会按时抓取，抓取后仅由已启用订阅消费候选。`fetch` 使用统一的增量分页流程，逐页持久化新种子、消费订阅队列并按现有配额和幂等规则发送到 qB；单次命令结束后不保留后台调度器。

### 测试

```sh
go test ./...
go vet ./...
cd webui
pnpm run build
pnpm run test:e2e
```

真实站点、qBittorrent、LLM 和恢复闭环测试必须通过环境变量和本地输入显式启用。具体命令与安全约束见 [docs/testing.md](docs/testing.md)。

### 本地构建

WebUI/CLI 服务版：

```sh
wails3 task build:webui
```

Desktop 桌面版：

```sh
wails3 build
```

两个任务都会检查前端依赖、生成 bindings、执行 `pnpm run build` 并构建 Go 程序。可以显式指定平台和架构：

```sh
wails3 task build:webui GOOS=windows ARCH=amd64
wails3 task build:webui GOOS=linux ARCH=amd64
wails3 task build:webui GOOS=linux ARCH=arm64

wails3 build GOOS=windows ARCH=amd64
wails3 build GOOS=linux ARCH=amd64
wails3 build GOOS=linux ARCH=arm64
```

默认输出到 `bin/`。`webui/dist/` 是前端临时构建目录，已被 Git 忽略，不应提交。Linux Desktop 构建需要原生目标架构环境，以及 `build-essential`、`pkg-config`、GTK4 和 WebKitGTK 6.0 开发库。

### GitHub Actions 打包与发布

`.github/workflows/build-release.yml` 负责构建，可通过 `workflow_dispatch` 单独选择 `linux-amd64`、`linux-arm64`、`windows-amd64` 或构建全部平台，也会在推送 `v*` tag 时构建全部平台。构建 workflow 专注打包，不执行测试或 vet；发布步骤拆到可复用的 `.github/workflows/publish-release.yml`。

推送 `v*` tag 时，六个包构建成功后会直接发布 GitHub Release；带连字符的版本 tag（例如 `v0.2.0-beta`）发布为 prerelease，其余 `v*` tag 发布为正式版本。手动运行时，只有选择 `all` 并启用 `publish_release` 才会发布；`release_tag` 留空会按 `Asia/Shanghai` 日期自动创建 `manual-YYYY.MM.DD.<run_number>` tag，并发布为 prerelease。同一天的多次手动发布通过 GitHub run number 区分。发布前会确认 tag 指向生成这些产物的 commit，随后用 `changelogithub` 汇总 Git commit message 和贡献者生成 Release Notes。

手动调试单个平台时保持 `publish_release=false`，不会创建 tag 或 Release。需要手动指定 tag 时只能使用 `v*` 或 `manual-*` 前缀。

复用 CI 构建脚本：

```sh
bash scripts/ci/build-release-linux.sh --arch amd64 --artifact-arch amd64
bash scripts/ci/build-release-linux.sh --arch arm64 --artifact-arch aarch64
```

```powershell
.\scripts\ci\build-release-windows.ps1 -Arch amd64 -ArtifactArch amd64
```

Linux CI 会通过 `--install-system-deps` 安装桌面构建依赖；本地环境如已安装可以省略。

### 使用 act 在 Windows 本地打包

安装 `act` 并启动 Docker Desktop 的 Linux 容器模式后，可以在 PowerShell 中复用同一份 GitHub Actions workflow：

```powershell
# 只检查 workflow 和 runner 映射
.\scripts\ci\run-act.ps1 -Job linux-arm64 -DryRun

# 通过 ARM64 Linux 容器仿真构建 Linux aarch64
.\scripts\ci\run-act.ps1 -Job linux-arm64

# 构建 Linux amd64
.\scripts\ci\run-act.ps1 -Job linux-amd64
```

本地 runner 镜像由 `scripts/ci/act-runner.Dockerfile` 首次自动构建。`.actrc` 将 Ubuntu runner 映射到本地 amd64/arm64 镜像，并复用容器与工具缓存。Windows x86-64 主机使用 Docker Desktop/QEMU 运行 ARM64 容器，速度会明显慢于 GitHub 原生 `ubuntu-24.04-arm` runner。

脚本在正式运行 ARM64 job 前会先验证 Docker Desktop 的 ARM64 容器执行能力。若提示无法执行 ARM64 Linux 容器，请重启 Docker Desktop、确认正在使用 Linux 容器模式，并运行以下命令；正常输出应为 `aarch64`：

```powershell
docker run --rm --platform linux/arm64 ubuntu:24.04 uname -m
```

若基础镜像检查正常，但本地 runner 镜像检查失败，可强制重建 runner：

```powershell
.\scripts\ci\run-act.ps1 -Job linux-arm64 -RebuildRunner
```

构建完成后，产物位于仓库根目录的 `dist/`：

- Desktop 桌面版：`dist/nexusbridge-desktop-linux-aarch64.tar.gz`
- WebUI/CLI 服务版：`dist/nexusbridge-webui-linux-aarch64.tar.gz`

如需使用主机 HTTP 代理，从容器内通过 `host.docker.internal` 访问：

```powershell
.\scripts\ci\run-act.ps1 -Job linux-arm64 -ProxyUrl http://host.docker.internal:7890
```

如果 workflow 已成功构建，但产物复制阶段被中断，可以不重新编译，直接收集复用容器中的产物：

```powershell
.\scripts\ci\run-act.ps1 -Job linux-arm64 -CollectOnly
```

默认指令不会绕过 PowerShell 执行策略。如果 `.ps1` 被策略拦截，先确认脚本可信，再为当前用户启用 `RemoteSigned`，或仅对单次命令显式使用 `-ExecutionPolicy Bypass`。后者不是 `act` 的必需参数。

## 更多文档

- [配置说明](docs/config.md)
- [架构说明](docs/architecture.md)
- [HTTP API](docs/api.md)
- [OpenAPI 定义](docs/api/openapi.yaml)
- [测试说明](docs/testing.md)
