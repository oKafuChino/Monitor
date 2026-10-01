# 本项目探针

探针随 Monitor 源码放在 `komari-agent/`，保留独立 Go module，统一由根目录 npm 命令与根 `.github/workflows/agent.yml` 构建。用户提供的上游 ZIP 已展开到此目录，不是 Git 子模块；ZIP 未携带上游提交号，不能声称固定了上游 SHA。已有 LICENSE、锁定的 go.mod/go.sum 和非 Linux 实现保留。探针内原 `.github/` 不会作为本项目工作流执行。

## 构建和检查

```sh
npm run build:agent
npm run test:agent
# 有 Docker 的环境：
npm run build:agent -- --docker
npm run test:agent -- --docker
# 服务端、界面与探针的完整检查：
npm run check -- --docker
```

原生探针构建可在 Windows 运行，原生服务端仍仅支持 Linux。`dist/agent/` 内产物命名为 `komari-agent-<goos>-<goarch>`，Windows 加 `.exe`。本地 race 检查需要 C 编译器，Docker 入口提供对应 Linux 工具链。已有访问外部 Ping 目标的测试改为 `AGENT_TEST_EXTERNAL_NETWORK=1` 时显式执行。

## 磁盘 IO 配置

| 配置方式 | 自动选择 | 显式设备 | 关闭 |
| --- | --- | --- | --- |
| 参数 | `--disk-io-devices auto`（默认） | `--disk-io-devices nvme0n1,vda` | `--disk-io-devices disabled` |
| 环境变量 | `AGENT_DISK_IO_DEVICES=auto` | `AGENT_DISK_IO_DEVICES=nvme0n1,vda` | `AGENT_DISK_IO_DEVICES=disabled` |
| JSON | `"disk_io_devices":"auto"` | `"disk_io_devices":"nvme0n1,vda"` | `"disk_io_devices":"disabled"` |

采样沿用 `--interval`，默认 3 秒。一条采样循环更新所有指标，发送循环读取最新不可变报告；断网仍采样，无历史发送队列，不重放已取出的样本。WebSocket 写操作设置 10 秒超时，退出时取消并等待采样循环。

Linux 读取 `/proc/diskstats`，sector 固定换算成 512 字节。用 `/sys/class/block` 的分区标记、holders/slaves 建立关系；默认选择最上层整设备，排除分区、loop、ram、zram，避免 LVM/dm/md 与底层设备重复计算。显式选择允许下层整设备，但不能同时选择祖先／后代，也不能选分区或重复名称。关系缺失、设备消失或格式错误上报 unavailable，不静默把不同层累加。

拓扑正常情况下每分钟刷新；选定计数每轮一次读取。设备名称、major/minor、sysfs 路径和拓扑摘要参与身份判定。首次采样、集合或身份变化、计数回退、非正时间差及超过周期 5 倍的间隔建立新基线，下一轮连续有效采样恢复。以本机单调时钟计算差分，先整数相减再转换浮点。公开报告不携带设备列表、文件路径或采集错误。

容器可通过 `HOST_PROC`/`HOST_SYS`（或 JSON `host_proc`/`host_sys`）指定匹配的只读挂载；未提供完整宿主机视图时，只能统计容器当前可见设备。其他平台当前上报 unsupported。

## 契约与发行

新增 `disk_io` 为可选对象。`ok` 必须带两个非负有限速率及正 `sample_interval_ms`；`warming_up`、`unsupported`、`unavailable`、`disabled` 省略速率、间隔为 0。真实空闲是 `ok` 加两个 0。

```json
{"disk_io":{"status":"ok","read_bytes_per_sec":1048576,"write_bytes_per_sec":524288,"sample_interval_ms":2000}}
```

HTTP、gzip 与 WebSocket 都沿用 agent.report。服务端指标为 `disk.io.read.rate` 与 `disk.io.write.rate`，Gauge、bytes/s，默认 avg 为样本算术平均。旧报告不产生 IO 点，旧历史字段为 null。安装脚本、前端生成命令、自更新默认来源统一为 `oKafuChino/Monitor`，发行 CI 用实际 repository 注入更新来源。稳定版本与 Snapshot 同时构建探针资产；稳定版本另生成 `ghcr.io/okafuchino/monitor-agent:<tag>`，只有最新 release 才更新 latest。

工作流只是待运行的发行配置，本次没有上传资产、发布镜像、部署或更新节点。首个包含探针的 release/snapshot 产物发布之前，新的安装来源可能暂无可下载资产。源码构建仍可使用上述本地命令。

### Linux 安装下载排错

`install.sh` 不再给 Bash 只读变量 `EUID` 赋值，使用 `id -u` 获取权限。安装时依赖 `curl` 和 `jq`（root 可由包管理器安装），通过 GitHub Releases API 检查当前平台的 `komari-agent-<os>-<arch>` 资产，而不是假定 `/releases/latest/download/...` 存在。默认优先最新的、含该资产的稳定版；若仅有 Snapshot，显示警告后选用最新可用 Snapshot。`--install-version stable` 严格要求稳定版，`snapshot` 严格要求 Snapshot，也可指定发行 tag。查询最近 100 个发行版；更旧版本可指定 tag。

如果没有任何发行版包含该资产，或 API 不可访问，脚本会退出并说明原因，不会通过镜像或切回上游探针掩盖资产缺失。解析和下载成功之前，已有 Agent 二进制和服务不会被移除；下载使用临时文件，失败会清理临时文件。

已有 release 缺少探针时，维护者可在 Actions → Integrated probe → Run workflow 中选择 `main`，将 `release_tag` 填为已有发行 tag。工作流从当前 main 构建、运行测试，再上传各平台探针资产；它不会覆盖已有同名资产，也不会为手动补传更新容器标签。空 `release_tag` 仅构建 Actions artifacts。必须先推送这些修改，并等待 Snapshot 或补传工作流成功，线上安装才有可下载的产物。

离线安装器回归检查：`npm run test:installer`。测试在隔离目录使用模拟下载/服务，不连接真实节点；有 jq 时同时执行真实 JSON 资产选择过滤器。Linux CI 要求 Bash/jq，不能静默跳过该检查。安装命令包含 Token，分享日志前应隐藏它；已公开的 Token 应在面板重置。

网络事件入口只允许 Ping、启动配置读取和版本切换；旧 exec/terminal/file 分发和能力声明已移除。兼容的 `disable_web_ssh` 运行配置固定为 true，版本切换独立保留。上游的旧辅助实现暂留源码，但没有网络分发入口。

升级顺序为服务端、探针、确认展示。回退探针时服务端兼容省略 IO；回退服务端应停用新版探针 IO 或恢复旧探针，旧版额外字段容忍行为尚未执行实测。管理员已有图表模板不会被改写，通过“添加图表 → 磁盘 I/O”加入双线图。
