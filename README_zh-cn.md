# Komari

[English](./README.md) | [简体中文](./README_zh-cn.md)

自托管服务器监控面板，支持实时指标、历史图表、Ping 监测和通知。前后端统一构建，网页直接嵌入 Go 服务，部署时只需运行一个服务。

本版本使用固定内置界面，已移除第三方主题、插件、网页终端和远程命令执行。仍保留远程文件管理（含写入）和 SQL 管理，不影响操作系统自身的 SSH 服务。

## 一键安装

获取本仓库源码，在仓库根目录执行。脚本会检查环境、构建镜像、启动服务并等待网页就绪。

Linux（已安装 Docker、Docker Compose，以及 curl 或 wget）：

```sh
bash install.sh
```

Debian/Ubuntu 尚未安装 Docker 时，可执行 `sudo bash install.sh --install-docker`，通过 Docker 官方 APT 仓库安装依赖。

Windows（先启动 Docker Desktop，使用 Linux 容器模式）：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\install.ps1
```

指定端口时，在命令后追加 `--port 8080`（Linux）或 `-Port 8080`（Windows）。仅检查环境可使用 `--check` / `-Check`；完整参数见 `--help` / `-Help`。

也可手动执行 `docker compose up -d --build` 部署。

浏览器访问 `http://服务器IP:25774`，按安装向导创建管理员账号并完成站点配置。

- 数据保存在仓库的 `data/` 目录，重建容器不会清空该目录。
- 脚本成功后将端口保存到 `.env`，下次运行自动沿用，保留其他配置。
- 查看日志：`docker compose logs -f monitor`。
- 停止服务：`docker compose down`。

## 源码构建

需要 Node.js 24+、`tar` 和 Docker。以下命令均在仓库根目录执行：

```sh
npm run setup
npm run build -- --docker
npm start -- --docker
```

Docker 提供 Go/CGO 编译环境，生成的 Linux 二进制位于 `dist/komari`，已包含前端。访问 `http://127.0.0.1:25774` 完成初始化。此启动方式默认仅供本机访问，数据保存在 `data/`。

如果本机已安装 Go 1.25.0（以 `go.mod` 为准）和 C 编译器，也可不使用 Docker：

```sh
npm run setup
npm run build
npm start
```

Windows 原生构建产物为 `dist/komari.exe`，其他原生构建产物为 `dist/komari`。

## 接入监控节点

1. 使用目标服务器能够访问的面板地址登录后台。
2. 打开“服务器列表”，添加节点，并复制对应操作系统的安装命令。
3. 在需要监控的服务器上运行命令，Agent 连接后即可在面板中查看数据。

## 更新与备份

更新前先通过后台下载备份。获取新版本源码后，重新运行对应安装脚本即可更新；源码构建方式需先退出当前服务，再构建并启动。保留原有 `data/` 目录，不要用空目录覆盖。

许可证见 [LICENSE](./LICENSE)。
