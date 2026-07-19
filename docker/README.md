# NexusBridge 一体化 Docker 镜像

此镜像在一个容器中运行三个服务：LinuxServer qBittorrent-nox、NexusBridge WebUI/CLI 和 mihomo。qBittorrent 原有的 s6 启动、PUID/PGID、临时管理员密码输出和 `/config` 数据布局保持不变，NexusBridge 与 mihomo 作为额外的 s6 longrun 服务运行。

## 启动

从仓库根目录执行：

```sh
docker compose -f docker/compose.yaml up -d
docker compose -f docker/compose.yaml logs -f nexusbridge
```

首次启动时，qBittorrent 会在容器日志中打印 `admin` 用户的临时密码。使用该密码登录 `http://主机地址:8080` 后立即修改用户名和密码。不要只查看 NexusBridge 的文件日志；临时密码来自 qBittorrent 标准输出，必须通过上面的容器日志查看。

默认端口：

| 服务 | 容器端口 | 用途 |
| --- | ---: | --- |
| mihomo | 7890/tcp | mixed HTTP/SOCKS 代理 |
| mihomo | 9090/tcp | WebUI / External Controller |
| qBittorrent | 8080/tcp | WebUI |
| NexusBridge | 8090/tcp | WebUI/API |
| qBittorrent | 16881/tcp、16881/udp | BT 传入连接 |

mihomo 控制器默认监听 `0.0.0.0:9090`，Compose 会发布 `9090:9090`，WebUI 可通过 `http://主机地址:9090/ui` 访问。默认模板中的 `secret` 为空，将端口暴露给其他主机前，必须在 `/config/mihomo/config.yaml` 中设置非空 `secret` 并重启容器。

## 配置和数据

所有应用配置均位于同一个 `/config` 卷：

```text
/config/
├── qBittorrent/              # qBittorrent 配置
├── nexusbridge/
│   ├── config.json           # NexusBridge 主配置
│   ├── nexusbridge.db
│   ├── logs/
│   └── sites/
└── mihomo/
    ├── config.yaml           # mihomo 主配置
    ├── ui/                   # 预置的 MetaCubeXD WebUI
    ├── GeoIP.dat
    ├── GeoSite.dat
    ├── country.mmdb
    ├── ASN.mmdb
    └── cache.db             # mihomo 运行时生成
```

首次启动只会在对应文件不存在时安装默认配置，后续重建容器不会覆盖用户配置。下载内容使用独立的 `/downloads` 卷。默认 NexusBridge 配置通过 `http://127.0.0.1:8080` 连接同容器内的 qBittorrent；登录 qB 后，在 NexusBridge 的设置页填写实际 qB 凭据。

mihomo 默认配置不内置任何节点。在 `/config/mihomo/config.yaml` 的 `proxy-providers` 中填写订阅 URL 后，地区组、自动选择、AI 节点筛选和负载均衡会从所有提供者动态生成。默认分流包含自定义直连、代理、AI、Telegram、微软和国内规则；本地参考用的 `config2.yaml` 已从 Git 和 Docker 构建上下文排除，不会进入镜像。

构建镜像时会直接读取默认 `config.yaml` 中的 `external-ui-url` 和 `geox-url`，将 MetaCubeXD、GeoIP、GeoSite、Country MMDB 和 ASN MMDB 打包到 `/defaults/mihomo` 中。容器启动时只会将缺失的文件安装到 `/config/mihomo`，不会覆盖已有数据库或 `ui/` 目录。`cache.db` 包含运行时状态，仍由 Mihomo 首次启动时生成。

镜像不会把默认模板直接写进 `/config`：模板保存在镜像内不会被 volume 遮蔽的 `/defaults`，Docker 完成 `/config` 挂载后，s6 启动脚本才把缺失的模板复制到已经挂载的目录。若直接在 Dockerfile 中 `COPY` 到 `/config`，容器启动时确实会被 bind mount 或 volume 覆盖；当前 `/defaults -> /config` 的启动时复制正是为了避免这个问题。

运行镜像显式安装并刷新 `ca-certificates`，NexusBridge、qBittorrent 和 mihomo 可以使用系统 CA 信任库访问 HTTPS 服务。镜像还安装了 `nano`，可在容器中直接执行 `nano /config/mihomo/config.yaml` 修改配置。

## 多架构构建

本地构建当前平台：

```sh
docker build -f docker/Dockerfile -t nexusbridge:local .
```

使用 Buildx 构建 amd64 与 arm64：

```sh
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --build-arg MIHOMO_VERSION=v1.19.29 \
  -f docker/Dockerfile \
  -t ghcr.io/owner/nexusbridge:latest \
  --push .
```

`.github/workflows/build-docker.yml` 分别使用 GitHub 的 `ubuntu-24.04` 和 `ubuntu-24.04-arm` 原生 runner 并行构建 `linux/amd64` 与 `linux/arm64`，无需 QEMU。两个架构构建完成后会合并并推送一个 GHCR manifest，同时生成版本号、主次版本号和 `latest` tag。手动运行 workflow 时可指定 mihomo 稳定版 tag 和目标镜像 tag，默认发布为 `edge`。

Dockerfile 默认使用 `lscr.io/linuxserver/qbittorrent:latest` 作为运行底座，并固定使用 mihomo `v1.19.29`。需要复现其他组合时可分别传入 `QBITTORRENT_IMAGE` 和 `MIHOMO_VERSION` build arg。
