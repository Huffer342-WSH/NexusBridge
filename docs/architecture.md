# 架构与代码导航

本文从系统边界逐层下钻到后端分层、核心业务域和关键运行流程，最后给出代码入口。业务细节继续查看[站点抓取](modules/site.md)、[订阅](modules/subscriptions.md)和[任务恢复](modules/recovery.md)；HTTP API、运行配置和测试边界分别见 [API](api.md)、[配置](config.md)和[测试](testing.md)。

## 1. 系统全景

NexusBridge 通过同一套 `core` 服务支持 Web、CLI 和桌面端。外部系统只经基础设施适配器接入，SQLite 保存业务状态和任务进度。

```mermaid
flowchart TB
    User[用户]

    subgraph NexusBridge[NexusBridge]
        direction TB
        UI[WebUI]
        CLI[CLI]
        Desktop[Wails Desktop]
        Adapter[HTTP / Desktop 适配层]
        Core[Core 业务服务]
        DB[(SQLite)]

        UI --> Adapter
        CLI --> Adapter
        Desktop --> Adapter
        Adapter --> Core
        Core --> DB
    end

    Sites[PT 站点] -->|HTML / torrent| Core
    Core -->|Web API| QB[qBittorrent]
    Core -.->|可选| LLM[OpenAI-compatible LLM]
    Core -.->|可选| Mihomo[Mihomo 配置]
    User --> UI
    User --> CLI
    User --> Desktop
```

图中实线是主要运行路径，虚线是可选能力。LLM 目前仍是 demo 能力，不进入核心抓取、订阅和恢复链路。

## 2. 后端分层与依赖方向

后端遵循“入口与适配 → 业务编排 → 基础设施”的单向依赖。`core` 决定业务流程，各适配层只负责协议转换和生命周期管理。

```mermaid
flowchart TB
    subgraph Entry[入口与运行时]
        Cmd[cmd/nexusbridge]
        DesktopMain[desktop]
        Runtime[runtimeconfig]
    end

    subgraph Adapter[交互适配层]
        CLIAdapter[cli]
        HTTP[server]
        DesktopAdapter[internal/desktop]
        WebAssets[webui assets]
    end

    subgraph Domain[业务层]
        Core[core]
        CoverCache[core/covercache]
    end

    subgraph Infra[基础设施层]
        Storage[(storage)]
        Parser[parser]
        Fetcher[fetcher]
        Policy[requestpolicy]
        QBClient[qbittorrent]
        Organizer[organizer]
        LLMClient[llm]
        Config[config / network / mihomo]
    end

    Cmd --> CLIAdapter
    DesktopMain --> DesktopAdapter
    Runtime --> CLIAdapter
    Runtime --> DesktopAdapter
    CLIAdapter --> HTTP
    DesktopAdapter --> HTTP
    HTTP --> Core
    WebAssets --> HTTP

    Core --> CoverCache
    Core --> Storage
    Core --> Parser
    Core --> Fetcher
    Core --> Policy
    Core --> QBClient
    Core --> Organizer
    Core --> Config
    CoverCache --> Storage
    Organizer --> LLMClient
    Parser --> Policy
```

依赖约束：

- `server`、`cli` 和 `desktop` 不复制业务流程，只调用 `core`。
- `core` 可以组合多个基础设施包，基础设施包不得反向依赖 `core`。
- `parser` 只解析定义和 HTML，不直接访问网络、数据库或 UI。
- `storage` 负责持久化语义，不负责调度、下载和页面解析。
- `stringutil`、`urlutil` 是无业务状态的叶子工具包，可被各层复用。

## 3. Core 业务域

`core` 保持单包以共享事务编排和领域模型，但文件按四个业务域组织。`app.go` 只负责组装共享依赖和生命周期。

```mermaid
flowchart LR
    App[app.go<br/>依赖组装与生命周期]

    subgraph SiteDomain[站点与种子目录]
        SiteCatalog[site_catalog / site_requests]
        SiteFetch[site_fetch / scheduler]
        TorrentCatalog[torrent_catalog / torrent_detail / torrent_files]
        Covers[covers / covercache]
    end

    subgraph Automation[规则与订阅自动化]
        Rules[rule_service / filter / title_expression]
        Subs[subscriptions]
        Candidates[subscription_candidates / execution]
        Pipeline[subscription_planning / persistence / sending]
        Batch[batch_download / download_plan]
    end

    subgraph QBTasks[qB 与任务]
        QB[qb / qb_catalog]
        Sync[qb_poll / qb_sync]
        Tasks[task_service]
    end

    subgraph Recovery[文件与恢复]
        Files[file_manager / torrent_size_index]
        Search[recovery_search / recovery_match]
        Recover[recovery / recovery_control / recovery_batch]
        Path[recovery_path / recovery_category]
    end

    App --> SiteCatalog
    App --> Rules
    App --> QB
    App --> Files

    SiteCatalog --> SiteFetch --> TorrentCatalog
    TorrentCatalog --> Covers
    SiteFetch --> Candidates
    Rules --> Subs --> Candidates --> Pipeline
    Batch --> Pipeline
    Pipeline --> QB
    QB --> Sync --> Tasks
    Sync --> Files
    Files --> Search --> Recover
    Path --> Recover
    Recover --> QB
```

领域之间通过明确的业务动作连接，而不是共享隐式全局状态：站点扫描产生种子和订阅候选，订阅流水线产生下载任务，qB 同步更新任务状态，恢复域利用本地种子元数据重新挂载文件。

### 领域模型与转换

```mermaid
flowchart LR
    ParserModel[parser types]
    StorageModel[storage records]
    CoreModel[core models_*]
    APIModel[server JSON]
    WebModel[webui types]

    ParserModel -->|converters.go| CoreModel
    StorageModel -->|converters.go| CoreModel
    CoreModel -->|handlers_*.go| APIModel
    APIModel -->|api.ts| WebModel
```

`models_site.go`、`models_torrent.go`、`models_subscription.go`、`models_qb_catalog.go`、`models_download.go`、`models_recovery.go` 和 `models_tasks.go` 按领域承载跨适配层模型；`converters.go` 集中处理 parser、storage 与 core 之间的转换。

## 4. 关键运行流程

### 4.1 站点扫描到 qB 下载

```mermaid
sequenceDiagram
    participant Trigger as 用户 / Scheduler
    participant Core as core.site_fetch
    participant Site as PT 站点
    participant Parser as parser
    participant DB as storage
    participant Sub as subscription pipeline
    participant QB as qBittorrent

    Trigger->>Core: 发起站点扫描
    Core->>DB: 创建或恢复抓取任务
    loop 按分页增量扫描
        Core->>Site: 使用站点策略请求页面
        Site-->>Core: HTML
        Core->>Parser: 解析种子列表
        Parser-->>Core: 标准化结果
        Core->>DB: 持久化种子与扫描进度
        Core->>Sub: 为新增种子生成候选
    end
    Sub->>DB: 原子领取候选并创建下载任务
    Sub->>QB: 发送 torrent、分类、标签和保存路径
    QB-->>Sub: 添加结果
    Sub->>DB: 更新候选与任务状态
```

站点计划和订阅计划相互独立：站点计划负责按时抓取，即使当前没有启用订阅也会执行；订阅只消费扫描产生的候选。更细的分页、增量边界和配额规则见[站点抓取](modules/site.md)与[订阅](modules/subscriptions.md)。

### 4.2 qB 同步、整理与恢复

```mermaid
flowchart LR
    QB[qBittorrent] -->|增量轮询 / 全量同步| Snapshot[(qB snapshots)]
    Snapshot --> Status[本地任务状态]
    Status -->|下载完成| Organize[整理任务]
    Organize -.->|可选建议| LLM[LLM demo]
    Organize --> Files[媒体文件]

    Files --> Scan[恢复路径扫描]
    TorrentIndex[(torrent 元数据与大小索引)] --> Match[候选匹配]
    Scan --> Match
    Match --> Validate[路径映射与文件校验]
    Validate -->|唯一有效候选| ReAdd[重新添加并校验 torrent]
    ReAdd --> QB
```

同步链路以 qB 快照为状态事实来源；恢复链路优先使用本地 torrent 元数据和文件大小索引，必要时再搜索站点。具体安全边界见[任务恢复](modules/recovery.md)。

## 5. 代码导航

以下按“从入口找到业务，再从业务找到基础设施”的顺序列出主要路径。无需从文件表逐项阅读。

### 5.1 入口与适配

| 路径 | 定位 |
| --- | --- |
| `cmd/nexusbridge/main.go` | 服务版进程入口。 |
| `internal/cli/root.go` | Cobra 命令、参数和进程生命周期。 |
| `desktop/main.go` | Wails 窗口、托盘和单实例入口。 |
| `internal/desktop/runtime.go` | 桌面端共享 core/server 生命周期。 |
| `internal/server/server.go` | HTTP 服务组装和路由。 |
| `internal/server/handlers_*.go` | 按 session、torrent、automation、recovery、settings、task 分组的 JSON 适配。 |
| `internal/server/http_helpers.go` | JSON 响应、查询参数和静态 WebUI 服务。 |
| `internal/runtimeconfig/` | 数据目录、配置文件、内置站点和构建模式。 |

### 5.2 Core 业务入口

| 业务域 | 主要文件 |
| --- | --- |
| 应用组装 | `app.go`、`services.go`、`converters.go`、`helpers.go` |
| 站点与种子 | `site_catalog.go`、`site_requests.go`、`site_fetch.go`、`scheduler.go`、`torrent_*.go`、`covers.go` |
| 规则与订阅 | `rule_*.go`、`filter.go`、`title_expression.go`、`subscriptions.go`、`subscription_*.go` |
| 下载与 qB | `download_plan.go`、`batch_download.go`、`qb.go`、`qb_catalog.go`、`qb_poll.go`、`qb_sync.go` |
| 文件与恢复 | `file_manager.go`、`torrent_size_index.go`、`recovery*.go` |
| 任务与可选能力 | `task_service.go`、`mihomo.go`、`network.go` |
| 领域模型 | `models_site.go`、`models_torrent.go`、`models_subscription.go`、`models_qb_catalog.go`、`models_download.go`、`models_recovery.go`、`models_tasks.go` |

### 5.3 基础设施包

| 包 | 职责与主要文件 |
| --- | --- |
| `internal/storage` | `sqlite.go` 管理连接，`migrations.go` 管理 schema；其余文件按 torrent、rule、subscription、task、qB snapshot、credential 和 cover cache 分域持久化。 |
| `internal/parser` | `definitions.go` 加载站点定义，`parser.go` 提供解析入口，`search_form.go`、`torrent_rows.go`、`pagination.go` 分解页面解析。 |
| `internal/fetcher` | HTTP 请求、重定向和 curl 输入解析。 |
| `internal/requestpolicy` | 域名规则、Cookie 白名单和请求决策。 |
| `internal/qbittorrent` | qB 认证、torrent、文件、分类、标签、控制、同步和元数据 API。 |
| `internal/core/covercache` | 封面文件缓存、状态机和并发控制；持久状态仍由 storage 保存。 |
| `internal/llm`、`internal/organizer` | OpenAI-compatible 客户端和媒体整理建议校验；当前为可选 demo 链路。 |
| `internal/config`、`internal/network`、`internal/mihomo` | 配置校验、代理环境和 Mihomo Provider 管理。 |
| `internal/stringutil`、`internal/urlutil` | 无状态、无业务依赖的共用工具。 |

### 5.4 WebUI、构建与部署

| 区域 | 主要路径 |
| --- | --- |
| WebUI 入口 | `webui/src/main.ts`、`App.vue`、`api.ts`、`types.ts` |
| 业务界面 | `webui/src/components/MediaView.vue`、`SubscriptionsView.vue`、`FileManagerView.vue`、`TasksView.vue`、`Settings*.vue` |
| 前端状态与工具 | `webui/src/composables/`、`webui/src/utils/`、`webui/src/config/` |
| WebUI 构建 | `webui/package.json`、`vite.config.ts`、`pnpm-lock.yaml` |
| 桌面构建 | `Taskfile.yml`、`desktop/tasks/`、`desktop/resources/` |
| 发布流水线 | `.github/workflows/build-release.yml`、`publish-release.yml`、`build-docker.yml` |
| 容器部署 | `docker/Dockerfile`、`docker/root/`、`docker/compose.yaml` |
| CI 脚本 | `scripts/ci/` |

## 6. 修改时如何定位

```mermaid
flowchart TD
    Change[需要修改功能]
    Change --> Contract{是否改变外部契约?}
    Contract -->|HTTP / JSON| Server[先看 server handlers<br/>再追到 core]
    Contract -->|WebUI 行为| Web[先看 component / api.ts<br/>再追到 handler]
    Contract -->|否，业务规则| Core[从对应 core 业务域开始]

    Server --> Core
    Web --> Server
    Core --> NeedIO{是否涉及外部 I/O?}
    NeedIO -->|数据库| Storage[storage]
    NeedIO -->|站点页面| Site[fetcher + parser + requestpolicy]
    NeedIO -->|qB| QB[qbittorrent]
    NeedIO -->|否| Model[models / filter / planning]
```

完成实现后，仅在外部契约变化时同步 API 文档，在运行行为或配置变化时同步配置文档；业务流程细节写入对应的 `docs/modules/` 文档，本文只维护稳定的依赖关系和代码导航。
