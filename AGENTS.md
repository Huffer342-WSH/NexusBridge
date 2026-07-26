# AGENTS.md

## 工作方式

- 新增或改写测试前，先向用户说明测试目标、使用真实服务还是 fake、所需环境和可能副作用，得到批准后再创建测试。fake 测试不能代替站点或 qBittorrent 的真实行为验证。
- 一个功能部分得到用户确认后，再集中更新相关文档；实现过程中不为每次小改动反复扩写文档。
- 不提交真实 Cookie、账号密码、API Key、真实 HTML 响应或包含凭据的命令示例。

## Review 准则

- 仅在执行代码 Review 时读取并遵循 `docs/review.md`；其他任务无需加载。

## 代码注释

- 使用符合 GoDoc 规范的中文注释

## 文档入口

- 文件职责和模块关系：`docs/architecture.md`
- 代码 Review（仅 Review 时读取）：`docs/review.md`
- 运行配置：`docs/config.md`
- 测试环境与真实服务安全边界：`docs/testing.md`
- HTTP API：`docs/api.md` 和 `docs/api/openapi.yaml`
- WebUI 美术风格：`docs/ui-style.md`

## 常用检查

- Go：`go test ./...`
- WebUI：`cd webui && pnpm run build`
- 开发服务：`go run ./cmd/nexusbridge serve`
