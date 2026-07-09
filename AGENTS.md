# AGENTS.md

## 构建

- WebUI/CLI 服务端：`wails3 task build:webui`
- WebUI：`cd webui && pnpm run build`
- 桌面端：`wails3 build`，产物名为 `nexusbridge-desktop`

## 测试

- Go 测试代码统一放在 `tests/` 目录；真实抓取、搜索配置解析、种子解析分离，详细说明见 `docs/testing.md`。
- Go 测试：设置 `NEXUSBRIDGE_TEST_BASE_URL` 和 `NEXUSBRIDGE_TEST_CURL_FILE` 后运行 `go test ./...`
- 前端类型与构建检查：`cd webui && pnpm run build`
- 真实页面抓取测试：设置 `NEXUSBRIDGE_TEST_BASE_URL` 和 `NEXUSBRIDGE_TEST_CURL_FILE` 后运行 `go test ./tests -run TestFetchTorrentsPageHTML -v`
- `TestFetchTorrentsPageHTML` 会把 cookie 写入 `data/tests/cookies.db`，后续解析测试从该数据库读取 cookie；`fetch.sh` 只允许作为测试输入，不是正式应用配置。

## 运行

- 校验配置：`go run ./cmd/nexusbridge config check`
- 启动 API/WebUI：`go run ./cmd/nexusbridge serve`
- 单次抓取入库：`go run ./cmd/nexusbridge fetch <site>`
- 抓取、筛选并发送 qBittorrent：`go run ./cmd/nexusbridge run-once <site>`
- 同步 qBittorrent 下载状态：`go run ./cmd/nexusbridge qb sync`
- 处理待整理任务：`go run ./cmd/nexusbridge organize pending`
- WebUI 开发服务器：`cd webui && pnpm run dev`
- 桌面端开发壳：`wails3 dev`

## 约定

- 核心逻辑放在 `internal/core`；CLI、HTTP、WebUI、桌面端都应调用共享服务，不要重复实现业务逻辑。
- 配置格式保持 JSON，除非项目明确决定迁移。
- SQLite 优先使用 `modernc.org/sqlite`，保持无 CGO 的跨平台构建能力。
- 前端包管理统一使用 pnpm。
- GUI 和 WebUI 共用 `webui/` 前端；桌面专属能力要隔离在小型适配层中。
- 新增 Go 函数需要简短中文 docstring，说明函数职责。
- 测试不得硬编码真实 PT 站点；真实访问测试只能通过环境变量和本地测试输入触发。
- 不得提交 cookie 文件、包含 cookie 的 curl 示例、账号密码或真实 HTML 响应。

## 文档维护

- 命令、配置、接口或模块边界变化时，同步更新 `README.md`、`docs/architecture.md`、`docs/config.md` 和 `docs/api.md`。
- 后端 HTTP 接口变化时，`docs/api/openapi.yaml` 是机器可读契约，必须和 `internal/server` 路由、`webui/src/api.ts` 调用、`docs/api.md` 说明、测试一起更新。
- 前端需要后端新增接口时，先把场景、请求、响应和验收标准写入 `docs/api/requests.md`，不要把未实现接口写成已实现 API。
- 本文件保持简短；详细设计说明放在 `docs/`。
