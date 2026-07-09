# AGENTS.md

## 常用命令

- Go 全量测试：`go test ./...`
- 前端类型与构建检查：`cd webui && pnpm run build`
- WebUI/CLI 服务版构建：`wails3 task build:webui`
- 桌面版构建：`wails3 build`
- 后端开发服务：`go run ./cmd/nexusbridge serve`
- WebUI 开发服务：`cd webui && pnpm run dev`
- 桌面开发壳：`wails3 dev`

真实站点、qBittorrent、LLM、解析细分测试见 `docs/testing.md`；不要把真实 cookie、账号密码、真实 HTML 响应或含 cookie 的 curl 示例提交到仓库。

## 项目约定

- `internal/core` 是共享业务层；CLI、HTTP Server、WebUI 和桌面端都应调用 core，不重复实现业务流程。
- `internal/parser` 只负责站点 JSON、搜索框补全、种子列表和详情页 HTML 解析；不要在 parser 中引入 HTTP、SQLite 或 UI 依赖。
- 站点配置保持 JSON，运行时站点定义来自 `sites_dir`，内置站点放在 `internal/builtin/sites/`。
- SQLite 使用 `modernc.org/sqlite`，保持无 CGO 构建。
- 前端包管理固定使用 pnpm；浏览器 WebUI 和 Wails 桌面版共用 `webui/`。
- 添加符合GoDoc规范的中文注释
- 测试代码统一放在 `tests/`；真实访问测试只能通过环境变量和本地测试输入显式启用，不硬编码真实 PT 站点凭据。

## 文档维护

- 命令、配置、接口、数据模型或模块边界变化时，同步更新相关 `README.md` 和 `docs/` 文档。
- 后端 HTTP API 变化时，同时更新 `docs/api/openapi.yaml`、`docs/api.md`、`webui/src/api.ts`/类型定义和测试。
- 前端需要尚未实现的后端接口时，先在 `docs/api/requests.md` 记录场景、请求、响应和验收标准。
