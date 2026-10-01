# Komari

[English](./README.md) | [简体中文](./README_zh-cn.md)

自托管服务器监控面板，支持实时指标、历史图表、Ping 监测和通知。前后端统一构建，网页直接嵌入 Go 服务，部署时只需运行一个服务。

本版本使用固定内置界面，已移除第三方主题、插件、网页终端和远程命令执行。仍保留远程文件管理（含写入）和 SQL 管理，不影响操作系统自身的 SSH 服务。

## Docker Compose 安装

安装 Docker 和 Docker Compose，获取本仓库源码后，在仓库根目录执行：

```sh
docker compose up -d --build
```

浏览器访问 `http://服务器IP:25774`，按安装向导创建管理员账号并完成站点配置。

- 数据保存在仓库的 `data/` 目录，重建容器不会清空该目录。
- 如需更改对外端口，在仓库根目录的 `.env` 文件中设置 `MONITOR_PORT=8080`，再启动服务。
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

更新前先通过后台下载备份。获取本仓库的新版本源码后，按所选安装方式重新构建并启动；源码方式需先退出当前服务再构建。保留原有 `data/` 目录，不要用空目录覆盖。

许可证见 [LICENSE](./LICENSE)。
