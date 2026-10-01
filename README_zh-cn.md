# Komari

[English](./README.md) | [简体中文](./README_zh-cn.md)

自托管服务器监控面板，支持实时指标、历史图表和 Ping 监测。前后端统一构建，网页嵌入 Go 服务，部署时只需运行一个服务。

**服务端仅支持 Linux。** 本版本使用固定内置界面，不提供第三方主题、插件、网页终端、远程命令执行、远程文件管理或通知功能；仍保留 SQL 管理。

源码仓库：[oKafuChino/Monitor](https://github.com/oKafuChino/Monitor)，默认安装分支为 `main`。

## 1. 安装前准备

- 使用 Linux 服务器，以及 root 或具有 sudo 权限的账号。
- 服务器需要能访问 GitHub、Docker 镜像仓库及构建依赖源。首次安装会在 Docker 中编译前后端，无需在宿主机安装 Node.js 或 Go。
- 默认对外端口为 TCP `25774`。根据服务器防火墙或云安全组配置放行该端口；脚本不会修改防火墙。
- Debian/Ubuntu 可通过 `--install-docker` 补齐缺少的 Git、Docker 和 Compose。其他 Linux 发行版请先自行安装 Git、Docker、Docker Compose，以及 curl 或 wget。

Debian/Ubuntu 如果没有 curl，先执行：

```sh
sudo apt-get update
sudo apt-get install -y ca-certificates curl
```

## 2. 使用 curl 一键安装

下载脚本后执行，默认从本仓库的 `main` 分支获取完整源码：

```sh
curl -fsSL --retry 3 https://raw.githubusercontent.com/oKafuChino/Monitor/main/install.sh -o /tmp/monitor-install.sh &&
sudo bash /tmp/monitor-install.sh --install-docker --dir /opt/monitor
```

也支持直接通过管道执行：

```sh
curl -fsSL --retry 3 https://raw.githubusercontent.com/oKafuChino/Monitor/main/install.sh |
  sudo bash -s -- --install-docker --dir /opt/monitor
```

已有可用的 Git、Docker 和 Compose 时，可省略 `--install-docker`。首次安装目标目录应不存在或为空；脚本不会覆盖不相关的非空目录。单独下载的脚本请放在 `/tmp`，不要放入尚未安装的目标目录。

脚本依次完成：

1. 检查 Linux 环境和依赖。
2. 下载 GitHub 源码到 `/opt/monitor`；完整本地源码目录则直接使用。
3. 构建整合镜像，启动 Docker Compose 服务。
4. 等待网页可访问，成功后将端口保存到 `.env`。

安装结束后访问 `http://服务器IP:25774`，创建管理员账号并填写站点信息。若配置了域名或 HTTPS 反向代理，请通过相应地址访问，并确保代理支持 WebSocket。

## 3. 安装目录、端口与版本

以 `/opt/monitor` 为例：

| 路径 | 用途 |
| --- | --- |
| `/opt/monitor/` | Git 源码及部署配置 |
| `/opt/monitor/.env` | 保存端口等配置，脚本只修改 `MONITOR_PORT` |
| `/opt/monitor/data/` | 数据库、站点文件等持久化数据 |

重建镜像、重建容器或 `docker compose down` 不会主动删除上述 `data/` 目录。

首次安装时指定其他端口：

```sh
sudo bash /tmp/monitor-install.sh --install-docker --dir /opt/monitor --port 8080
```

已安装且源码未改变时，仅更换端口可复用已有镜像：

```sh
sudo bash /opt/monitor/install.sh --port 8080 --skip-build
```

之后访问 `http://服务器IP:8080`。未传 `--port` 时，使用环境变量/已有 `.env` 配置，否则使用 `25774`。

常用参数：

| 参数 | 说明 |
| --- | --- |
| `--dir /opt/monitor` | 安装目录。未指定时优先使用本地源码目录；远程执行时 root 默认 `/opt/monitor`，普通用户默认 `~/monitor` |
| `--repo oKafuChino/Monitor` | 首次下载的 GitHub 仓库，格式为 `OWNER/REPO` |
| `--ref main` | 分支或标签。首次默认为 `main`，后续沿用安装时记录的值 |
| `--port 8080` | 设置访问端口，部署成功后保存 |
| `--install-docker` | 在 Debian/Ubuntu 补齐缺少的 Git/Docker/Compose；不会自动卸载冲突软件包 |
| `--update` | 获取远端源码并快进更新，然后重新构建、部署 |
| `--skip-build` | 跳过构建，仅使用已有镜像；不适用于首次下载 |
| `--timeout 300` | 启动后的网页就绪等待时间，默认 180 秒，不包含镜像构建时间 |
| `--check` | 只检查已有源码、依赖与配置；不下载、不更新、不启动服务 |
| `--help` | 显示帮助 |

指定分支/标签时，该版本必须包含整合工程所需的前端、Dockerfile 和安装脚本。首次安装后会记录仓库与 ref，更新时可继续沿用。

## 4. 接入监控节点

1. 使用目标服务器能够访问的面板地址登录后台，不要通过目标服务器无法访问的 `127.0.0.1` 地址生成安装命令。
2. 打开“服务器列表”，添加节点，复制对应 Agent 的安装命令。
3. 在需要监控的服务器上运行命令。
4. 等待节点上线，在面板查看监控数据。

这里的 Linux 限制指服务端部署；节点 Agent 属于独立项目，其可用平台以 Agent 自身支持范围为准。

## 5. 更新与日常管理

更新前先在后台下载备份，并妥善保留 `.env` 和 `data/`。

更新源码并重新部署：

```sh
sudo bash /opt/monitor/install.sh --update
```

如需同时使用最新安装脚本，可重新下载后更新：

```sh
curl -fsSL --retry 3 https://raw.githubusercontent.com/oKafuChino/Monitor/main/install.sh -o /tmp/monitor-install.sh &&
sudo bash /tmp/monitor-install.sh --dir /opt/monitor --update
```

`--update` 只允许快进更新，不会降级到旧版本。存在未提交改动、未跟踪文件或仓库地址不一致时会停止，不会强制重置源码。请先提交或备份这些改动后重试。不加 `--update` 只重新部署当前源码，不会自动获取新版本。

以下管理命令在安装目录执行，例如先运行 `cd /opt/monitor`：

| 操作 | 命令 |
| --- | --- |
| 查看状态 | `sudo docker compose ps` |
| 查看日志 | `sudo docker compose logs --tail 100 -f monitor` |
| 重启服务 | `sudo docker compose restart monitor` |
| 暂停服务 | `sudo docker compose stop monitor` |
| 恢复服务 | `sudo docker compose start monitor` |
| 移除容器、保留数据目录 | `sudo docker compose down` |

## 6. 常见问题

- **GitHub 下载失败**：检查服务器 DNS 和到 `raw.githubusercontent.com`、`github.com` 的连接。下载失败后可重试；未完成的源码会留在临时目录中处理，不覆盖已有安装。
- **Docker 无法连接或权限不足**：确认 Docker 服务已启动。Linux 上可检查 `sudo systemctl status docker`，并使用有 Docker 权限的账号或 sudo 执行安装脚本。
- **端口已被占用**：选择其他 `--port`。脚本不会停止占用端口的其他服务。
- **镜像构建失败**：查看构建输出，检查依赖下载和宿主机资源。构建失败不会进入启动/重建服务步骤。
- **等待网页就绪超时**：进入安装目录查看 `docker compose logs --tail 100 monitor`，确认应用启动原因；必要时增加 `--timeout`。失败时不会保存新端口，修复后使用相同参数重试。
- **出现 `.deploy.lock` 或安装目录旁的 `.目录名.install.lock`**：先确认没有其他安装进程运行，再处理异常中断留下的空锁目录；不要在其他部署仍运行时删除它。

## 7. 手动部署与源码构建

已有完整源码和 Docker 环境时，可在仓库根目录直接部署：

```sh
docker compose up -d --build
```

本机源码构建需要 Linux、Node.js 24+、`tar`、Go 1.25.0（以 `go.mod` 为准）和 C 编译器：

```sh
npm run setup
npm run build
npm start
```

也可保留 Node.js 和 `tar`，由 Docker 提供 Go/CGO 编译环境：

```sh
npm run setup
npm run build -- --docker
npm start -- --docker
```

产物为包含前端的 Linux 二进制 `dist/komari`。`npm start` 默认监听 `127.0.0.1:25774`，仅供本机访问；对外部署可使用上述 Docker Compose 方式。开发模式使用 `npm run dev -- --docker`。

许可证见 [LICENSE](./LICENSE)。
