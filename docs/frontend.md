# WebUI 路由与 URL 状态

WebUI 使用 Vue Router History 模式。浏览器版和 Wails 桌面版使用相同的页面路径；页面导航会产生浏览器历史记录，页面内部的筛选、分页和文件路径使用 `replace` 更新当前记录，避免连续操作产生大量历史项。

## 页面路由

| 路径 | 页面 |
| --- | --- |
| `/media` | 媒体库 |
| `/files` | 文件与恢复 |
| `/subscriptions` | 订阅与筛选 |
| `/tasks` | 任务 |
| `/settings/sites` | 站点设置 |
| `/settings/network` | 网络代理 |
| `/settings/llm` | LLM 设置 |
| `/settings/qbittorrent` | qBittorrent 设置 |
| `/settings/mihomo` | Mihomo 设置 |

`/` 重定向到 `/media`，`/settings` 重定向到 `/settings/sites`。其他未知路径显示 404 页面，不自动改写为首页。直接访问或刷新上述路径时仍停留在对应页面；需要登录时，登录完成后继续显示原路由。

## 媒体页查询参数

媒体页把服务端查询相关的界面状态写入 URL：

| 参数 | 含义 | 默认值与规范化 |
| --- | --- | --- |
| `page` | 页码 | 正整数，默认 `1`；默认值不写入 URL |
| `page_size` | 每页数量 | 仅允许 `20`、`50`、`100`，默认 `50` |
| `site` | 站点 ID | 默认全部站点，此时省略 |
| `q` | 标题、分类或站点搜索词 | 去除首尾空白，空值省略 |
| `pinned` | 是否包含置顶种子 | `0` 表示不包含；默认包含并省略 |

例如：

```text
/media?page=3&page_size=20&site=demo&q=movie&pinned=0
```

刷新或直接打开该地址会恢复页码、每页数量、站点、搜索词和置顶开关。站点、搜索词、置顶开关或每页数量变化时页码重置为 `1`；搜索输入使用 300 ms 防抖。无效值、默认值和不支持的查询参数会被规范化并从 URL 移除。

媒体页读取本地缓存后会提交增量抓取：选择单个站点时只请求该站点；选择默认的“全部站点”时，并发请求所有已保存 Cookie 的站点。每个站点在当前 WebUI 生命周期内只会成功自动提交一次，正在提交的站点也不会重复请求；手动扫描和后台周期计划仍可独立触发。

## 文件页路径参数

文件页使用 `path` 保存当前浏览目录：

```text
/files?path=C%3A%2FMedia%2FFilms
```

页面使用后端返回的规范化路径更新 URL，因此首次打开 `/files` 后也会写入实际根路径。刷新、直接访问带 `path` 的地址、路径输入、双击目录以及文件页内部的前进/后退都会保持输入框、目录内容和 URL 一致。

文件路径变化同样使用 `replace`，目录历史由文件页自身维护；没有可用目录历史时，鼠标侧键交还浏览器处理页面导航。无法读取的路径保留在 URL 中并显示后端错误，便于修正或重试。`path` 会暴露 NexusBridge 所在机器的本地目录结构，不应把包含敏感目录名的地址公开分享。

## 状态边界与路由回退

- 设置表单草稿、订阅页内部标签、弹窗和任务临时状态不进入 URL。
- 媒体卡片布局等纯显示偏好保存在浏览器 `localStorage`。
- Web 服务的 SPA catch-all 对未命中的非 `/api`、`/rss` 路径返回 `index.html`。
- Wails 只对 `GET/HEAD`、接受 HTML、没有文件扩展名且原响应为 404 的页面导航回退 `index.html`；缺失静态资源、API 和 Runtime 请求不会被前端路由吞掉。

代码入口是 `webui/src/router.ts`、`webui/src/App.vue`、`webui/src/components/MediaView.vue` 和 `webui/src/components/FileManagerView.vue`；Web 与桌面回退分别位于 `internal/server/http_helpers.go` 和 `internal/desktop/assets.go`。
