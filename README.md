# NexusBridge

NexusBridge 是一个 Go + Vue 的跨平台应用，用于检索 NexusPHP 站点、持久化 torrent 文件、同步 qBittorrent 状态并整理媒体。浏览器版、CLI 和 Wails 桌面版共用 `internal/core` 与同一套 WebUI。

## 环境要求

- Go 1.25+
- Node.js 22+
- pnpm 11+
- 桌面开发需要 Wails v3 CLI `v3.0.0-alpha2.117`

```sh
go mod download
cd webui
pnpm install
cd ..
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-alpha2.117
```

## 数据目录与配置

开发运行和测试默认使用仓库下的 `data/`：首次启动会创建 `data/config.json`、SQLite、日志目录，并安装内置站点定义。可参考 [data/config.example.json](data/config.example.json) 修改配置。

正式构建默认使用系统用户配置目录：Windows 为 `%AppData%\NexusBridge`，Linux 通常为 `$XDG_CONFIG_HOME/NexusBridge` 或 `~/.config/NexusBridge`。如需便携或自定义目录，将 [nexusbridge.bootstrap.example.json](nexusbridge.bootstrap.example.json) 复制为可执行文件旁的 `nexusbridge.bootstrap.json`；相对 `data_dir` 以可执行文件目录为基准。

`NEXUSBRIDGE_DATA_DIR` 可临时覆盖未显式指定配置时的数据目录。显式传入 `--config` 时，该配置文件所在目录作为数据根目录，配置中的数据库、站点和日志相对路径都从这里解析。

## WebUI/CLI 服务版

WebUI/CLI 服务版使用 `nexusbridge-webui` 作为发布二进制名称。它既可以启动浏览器 WebUI，也可以直接执行抓取、同步和整理命令。

### 开发运行

开发 WebUI 时使用两个终端。第一个终端启动后端：

```sh
go run ./cmd/nexusbridge serve
```

第二个终端启动 Vite：

```sh
cd webui
pnpm run dev
```

后端默认监听 `http://127.0.0.1:8090`；Vite 默认监听 `http://127.0.0.1:5173` 并代理 `/api` 与 `/rss`。

常用 CLI 命令：

```sh
go run ./cmd/nexusbridge config check
go run ./cmd/nexusbridge fetch <site>
go run ./cmd/nexusbridge run-once <site>
go run ./cmd/nexusbridge qb sync
go run ./cmd/nexusbridge organize pending
```

需要使用其他配置时追加 `--config path/to/config.json`。

### 构建

推荐使用一键构建命令：

```sh
wails3 task build:webui
```

该任务会自动检查前端依赖、生成 bindings、执行 `pnpm run build`，再使用 `production` 标签构建并嵌入 WebUI。指定目标平台和架构：

```sh
wails3 task build:webui GOOS=windows ARCH=amd64
wails3 task build:webui GOOS=linux ARCH=amd64
wails3 task build:webui GOOS=linux ARCH=arm64
```

也可以手动执行同一构建流程：

```sh
cd webui
pnpm run build
cd ..
go build -tags production -trimpath -o bin/nexusbridge-webui ./cmd/nexusbridge
```

Windows 手动构建时输出文件需要添加 `.exe`：

```powershell
go build -tags production -trimpath -o bin/nexusbridge-webui.exe ./cmd/nexusbridge
```

### 运行构建产物

Linux：

```sh
./nexusbridge-webui serve
```

Windows：

```powershell
.\nexusbridge-webui.exe serve
```

启动后通过浏览器访问 `http://127.0.0.1:8090`。发布二进制已经内嵌 WebUI，不需要额外携带 `webui/dist`。

`webui/dist/` 是 `pnpm run build` 生成的临时目录，已被 Git 忽略，不提交到仓库。开发和测试构建不要求该目录存在；`production` 构建会在嵌入前自动生成它。

## Desktop 桌面版

桌面版使用 `NexusBridge Desktop` 作为应用名称，发布二进制名称为 `nexusbridge-desktop`。

### 开发运行

```sh
wails3 dev
```

Wails 会启动桌面壳和前端开发服务器；桌面专属入口、资源及平台任务都位于 `desktop/`。

### 构建

```sh
wails3 build
```

`wails3 build` 会自动安装或检查前端依赖、生成 bindings、执行 `pnpm run build`，最后构建桌面程序，不需要提前手动构建 WebUI。

指定目标平台和架构：

```sh
wails3 build GOOS=windows ARCH=amd64
wails3 build GOOS=linux ARCH=amd64
wails3 build GOOS=linux ARCH=arm64
```

Windows 产物为 `bin/nexusbridge-desktop.exe`，Linux 产物为 `bin/nexusbridge-desktop`。Linux 桌面构建需要原生目标架构环境，以及 `build-essential`、`pkg-config`、GTK4 和 WebKitGTK 6.0 开发库。

### 运行构建产物

Windows 可以直接运行 `nexusbridge-desktop.exe`；Linux 需要先确保 GTK4 和 WebKitGTK 6.0 运行库已安装，然后执行：

```sh
./nexusbridge-desktop
```

## 测试

```sh
go test ./...
go vet ./...
cd webui
pnpm run build
pnpm run test:e2e
```

真实站点和 qBittorrent 测试只通过本地环境变量启用，默认产物写到 `data/tests/`。具体见 [docs/testing.md](docs/testing.md)。

## 自动构建与发布

`.github/workflows/build-release.yml` 仅通过 GitHub Actions 的 `workflow_dispatch` 手动触发，不执行测试或 vet。`target_job` 可选择 `all`、`linux-amd64`、`linux-arm64` 或 `windows-amd64`，用于只调试单个平台构建。桌面版使用 `nexusbridge-desktop`，CLI/浏览器服务版使用 `nexusbridge-webui`，两种形态分别生成以下平台产物：

- Windows amd64
- Linux amd64
- Linux aarch64（Go 架构名为 `arm64`）

Linux aarch64 桌面版在原生 arm64 runner 上构建。需要发布时，将 `target_job` 设为 `all`、启用 `publish_release`，并通过 `release_tag` 指定已存在的 `v*` 标签；未填写 `release_tag` 时使用手动运行选择的 ref 名称。

GitHub Actions 调用的构建脚本也可以本地自测：

```sh
bash scripts/ci/build-release-linux.sh --arch amd64 --artifact-arch amd64
bash scripts/ci/build-release-linux.sh --arch arm64 --artifact-arch aarch64
```

```powershell
powershell -ExecutionPolicy Bypass -File scripts/ci/build-release-windows.ps1 -Arch amd64 -ArtifactArch amd64
```

Linux CI 会额外传入 `--install-system-deps` 安装 GTK4 和 WebKitGTK 6.0 开发库。本地如已安装依赖，可以省略该参数。

## 更多文档

- [架构](docs/architecture.md)
- [配置](docs/config.md)
- [HTTP API](docs/api.md)
- [测试](docs/testing.md)
