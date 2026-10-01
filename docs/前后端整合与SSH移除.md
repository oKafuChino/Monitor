# 前后端整合与 SSH／远程命令移除

更新：2026-10-01。本记录对应用户追加要求“完全移除 ssh 功能，并把前后端整合”，覆盖上一阶段“保留独立 exec”的范围约定。

## 当前功能边界

已移除项目侧的交互终端和任意远程命令执行：

- `/admin/exec` 页面、菜单与五组语言中的专用文案。
- `admin:exec` 以及五个命令任务查询 RPC。
- `/api/admin/task/*` 命令执行、任务查询和结果接口；退场路径返回 JSON 404。
- `agent.exec` 下发、`agent.taskResult` 回传以及关联协议类型、数据库访问代码、历史结果定时清理。
- 安装表单中的 WebSSH/远程控制选项及命令生成分支。
- 开发 CI 中的 SSH 密钥、ssh-keyscan、ssh/scp 生产部署步骤。开发工作流仅构建产物。

Agent 出站事件现在只允许固定的文件操作、Ping、启动配置读取和版本切换；shell、未知事件不能直接写入连接或加入离线队列。有效管理员/Agent 调用旧功能也会失败，而非仅隐藏前端菜单。

文件管理、Monaco 编辑、资源小窗、在线 SQL、监控/Ping、通知、账户与 Agent 管理继续保留。因此本项目并非只读监控。没有修改系统 sshd、端口/防火墙，也没有修改仓库外的独立 Agent 二进制。

旧 `tasks` / `task_results` 数据表不再自动创建或访问，也不主动删除；已有历史记录仍随数据库备份保留。`database/tasks` 中的 Ping 任务功能保留。原 `/terminal?uuid=...` 兼容跳转仅通向文件管理，不产生终端会话。

## 整合后的工程结构

- 根目录 `package.json`：统一安装、开发、构建、启动与测试入口。
- `komari-web/`：同一仓库内的前端源码。该目录的 `package-lock.json` 是前端依赖锁；删除了原来的 `komari-web: file:` 自引用。
- `scripts/project.mjs`：跨平台命令编排，支持本机 Go/CGO 或 `--docker` 工具链；直接使用参数数组启动进程。
- `scripts/embed-frontend.mjs`：把真实前端 dist 和固定设置声明打包给 Go embed。
- Go 编译输出包含完整网页与 API，正式运行只需要一个服务、一个端口，不需要另外运行 Node/Vite。
- `Dockerfile` 默认 `runtime` 目标从源码构建前端和后端；既有多架构发行 CI 使用同文件的 `release` 目标装配已编译产物。
- 根目录 `.github/workflows/` 是唯一 CI/发行入口，删除了前端子目录里独立发布工作流。根目录构建 action 复用相同的项目命令。

## 开发与构建命令

前置：Node.js 24+；本机构建需要 go.mod 声明的 Go 版本（当前 1.25.0）和 C 编译器。可以使用 Docker 承担 Go/CGO 编译。

在仓库根目录安装锁定依赖：

```sh
npm run setup
```

当前 Windows 环境没有本机 Go，已实测以下方式：

```sh
npm run dev -- --docker
```

该命令构建后端所需的内置界面并同时启动：

- 开发网页：`http://127.0.0.1:5173`，支持 Vite HMR。
- 后端及嵌入网页：`http://127.0.0.1:25774`。
- 网页的 `/api` HTTP 和 WebSocket 请求代理到该后端，无需单独配置另一个仓库。
- 默认开发数据位于 `dist/dev/data`，与项目正式 `data/` 分开。
- 退出时关闭前后端子进程，并停止本次创建的开发容器。已有端口被占用时会报错，不接管其他服务。
- 后端源码修改后重新运行开发命令；前端源码由 Vite 热更新。
- 已有最新构建时可用 `npm run dev -- --docker --no-build`。

统一构建和启动：

```sh
npm run build -- --docker
npm start -- --docker
```

有本机 Go/CGO 时去掉 `-- --docker` 即可。原生 Windows 产物为 `dist/komari.exe`；Docker 工具链产物为 Linux `dist/komari`，通过 Docker 启动或部署到兼容 Linux 环境。

生产启动的默认运行目录为项目根目录（数据在 `data/`）；可通过以下环境变量调整本地运行，不写入源码：

| 环境变量 | 含义 | 默认值 |
| --- | --- | --- |
| `MONITOR_PORT` | 本地后端端口 | 25774 |
| `MONITOR_DEV_PORT` | Vite 开发端口 | 5173 |
| `MONITOR_RUNTIME_DIR` | 运行目录，包含 data/cache | 开发时 dist/dev；start 时项目根目录 |

完整检查：

```sh
npm run check -- --docker
```

该入口串联 lint、前端构建、UI/引导测试、归档一致性测试与 `go test ./...`。也提供 `build:web`、`build:server`、`test:web`、`test:server` 子命令。后端子命令要求已生成嵌入网页，缺失时明确提示先执行 `build:web`。

## 单镜像使用

可直接从当前源码构建，无需提前手工生成二进制：

```sh
docker build -t monitor:local .
docker run --rm -p 25774:25774 -v ./data:/app/data monitor:local
```

也提供 `compose.yaml`：

```sh
docker compose up --build -d
```

Compose 保留 `./data` 持久化；这是供用户明确启动服务时使用的入口，本次没有对该目录执行 Compose 启动或部署。既有发行工作流固定使用 `target: release`，保持原来的跨架构二进制装配流程。

`.dockerignore` 排除 `.git`、node_modules、环境文件、本地 data/cache、日志和已有构建输出，避免把本地数据或旧前端误送入源码镜像构建上下文。

## 实测验证

- 根目录 `npm run setup` 和最终 `npm run check -- --docker` 均成功；TypeScript/Vite 构建、Go 编译成功。
- lint：0 错误、22 个既有警告；未因本次机械删除扩展到无关格式/依赖升级。
- UI/引导测试 7 项通过；新增嵌入一致性测试 2 项通过，验证归档中的 index 与实际入口资源逐字节匹配本地 dist，且不包含旧命令客户端调用。
- `go test ./...` 通过：包含有效管理员的旧命令 REST/RPC、有效 Agent 的旧结果回传拒绝、出站事件白名单，以及保留的文件/Ping/通知等既有回归。
- 新数据库不会创建退场命令任务表；保留任务表的历史数据库没有自动删除迁移。
- 实际启动 `npm run dev -- --docker --no-build`，使用独立 `dist/integration-dev-smoke` 目录和 5175/25810 端口，完成 6 项联调：安装 API 代理、Vite HMR、同一后端登录、旧命令接口拒绝、设置持久化、有效 Agent WebSocket 代理。退出后两个临时端口关闭，无遗留开发容器。
- `docker build --target runtime --tag monitor:local .` 成功。单镜像在临时容器内完成 5 项检查：嵌入网页及入口资源、新装与登录、旧命令/终端拒绝、保留接口、无命令任务新表。
- `docker compose config --quiet` 通过；没有执行 Compose 部署。
- 没有调用真实节点执行命令，也没有推送镜像、触发远端 CI 或部署到生产机。测试容器和开发监听端口已停止。

日志与结构化证据：

- `ssh-integration-setup.log`
- `ssh-integration-server-build.log`
- `ssh-integration-ui-tests.log`、`ssh-integration-bundle-tests.log`
- `ssh-integration-go-tests.log`、`ssh-integration-lint.log`
- `ssh-integration-docker-build.log`
- `ssh-integration-dev-smoke.json`、`ssh-integration-image-smoke.json`
- `ssh-integration-check.log`：根目录一键检查的最终结果。

前一阶段的主题/插件迁移与文件管理截图仍见 [前一阶段执行记录](./主题插件与远程终端移除执行记录.md)。其中“保留独立 exec”已被此次追加范围覆盖。
