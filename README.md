# Gater

Gater 是一个面向 macOS 本地开发环境的按需启动反向代理。它把已注册的应用映射到
`<name>.lab:<port>`，第一次访问时才启动应用，应用空闲后自动停止。

## 启动模型

当前实现的完整链路如下：

1. `gater` 启动时创建 Store 和 Manager，从 `~/.config/gater/store.yaml` 恢复已注册应用；恢复只创建内存实例，不会拉起子进程。
2. 通过 Web 控制台、API 或客户端注册应用。Manager 会解析并校验工作目录；如果目录中有 `app.yaml`，仅在请求没有提供 `cmd` 或 `args` 时补齐这两个字段，然后写入 Store。
3. 请求 `demo.lab:8080` 时，代理根据 Host 找到应用。应用不是 `running` 时进入 `starting`，检查工作目录和 `Cmd`，展开参数中的 `$PORT`/系统环境变量，设置工作目录和环境后启动子进程。
4. 子进程使用独立进程组，标准输出和标准错误同时写入 Gater 日志与内存日志缓冲。Gater 每 100ms 连接 `127.0.0.1:<内部端口>`，最多等待 30 秒；端口可连接即认为就绪，然后转发原始 HTTP/WebSocket 请求。
5. 每次代理请求会刷新 `LastActive`。后台监视器每 3 秒检查空闲时间，超过 `idle_timeout` 就向进程组发送 `SIGINT`，3 秒后仍未退出则发送 `SIGKILL`。
6. Gater 收到退出信号时取消全局 Context，停止所有应用，再关闭 HTTP 服务。

状态含义：`stopped` 未运行，`starting` 已启动但仍在等待端口，`running` 已通过端口探测，`crashed` 启动失败、就绪超时或运行中异常退出。

### 启动配置

应用注册数据保存在 Store 中，结构等价于：

```yaml
name: demo
path: /absolute/path/to/demo
app_type: phx
cmd: sh
args: ["-c", "python3 -m http.server $PORT"]
env:
	APP_ENV: development
idle_timeout: 5m
```

`app.yaml` 中的 `cmd`、`args`、`env` 和 `idle_timeout` 会在注册时作为缺省配置使用，注册请求中的字段优先。Store 使用 YAML 保存注册数据。参数中的 `$PORT` 会被替换成 Gater 分配的内部端口（当前从 `50001` 开始），应用也会收到同名环境变量。`cmd` 通过 `exec.LookPath` 查找，因此依赖 shell 初始化的 PATH 时，建议在 launchd 配置中显式设置 PATH。

注册时请求字段优先于 `app.yaml` 的 `cmd`/`args`。已存在同名应用会先停止旧实例，再保存新配置并重新注册；Gater 重启后内部端口可能变化，应通过列表 API 或控制台读取最新端口，不要硬编码 `50001`。
`app_type` 用于启用常用应用的内置环境配置。设置为 `phx` 时，应用进程会收到 `PHX_HOST=<当前访问域名>`；未设置或使用其他类型时保持通用行为。

Web 和 CLI 的注册接口只有两类：`POST /api/apps/from-yaml-file` 接收指向 Server 本地 `app.yaml` 的 `path`，由 Server 读取、解析和校验；`POST /api/apps/from-config` 接收完整的应用配置 JSON，由 Server 统一校验并注册。调用端不解析 YAML。

## 最小 Demo

Demo 位于 [`lab/demo`](lab/demo)。它使用 Python 标准库，不需要安装项目依赖：

```bash
cd lab/demo
python3 -m http.server 18080
```

另开终端启动 Gater（使用临时 Store，避免修改个人注册表）：

```bash
go run ./cmd/server \
	-store /tmp/gater-demo-store.yaml \
	-port 8080 \
	-admin-host admin.lab
```

注册 demo。`path` 必须是目录的绝对路径，`cmd`/`args` 也可以省略，Manager 会从目录中的 `app.yaml` 补齐：

```bash
curl -i -X POST http://localhost:8080/api/apps \
	-H 'Content-Type: application/json' \
	-d '{
		"name": "demo",
		"path": "'"$PWD"'",
		"env": {"APP_ENV": "development"},
		"idle_timeout": "5m"
	}'
```

验证按需启动：

```bash
curl -H 'Host: demo.lab:8080' http://localhost:8080/
curl http://localhost:8080/api/apps/demo
curl -X POST http://localhost:8080/api/apps/demo/stop
curl http://localhost:8080/api/apps/demo/logs
```

浏览器访问 `http://admin.lab:8080` 可管理应用；如果本机没有把 `admin.lab` 和 `*.lab` 解析到 `127.0.0.1`，先用上面的 `Host` 方式验证，或在本地 DNS 配置中加入相应解析。

## 构建与发布

项目包含两个命令：`gater` 是后台服务，`gater-client` 是 HTTP 管理客户端。

本地构建：

```bash
go build -o bin/gater ./cmd/server
go build -o bin/gater-client ./cmd/client
```

查看版本：

```bash
bin/gater -version
bin/gater-client -version
```

发布版本使用 GoReleaser。创建并推送一个 `v*` tag 后，GitHub Actions 会自动发布多平台产物：

```bash
git tag v0.1.0
git push origin v0.1.0
```

发布后可以通过 mise 的 GitHub backend 安装服务端：

```bash
mise use github:cao7113/gater
```

管理客户端可以配置为同一 release 的独立工具：

```toml
[tool_alias]
gater-client = "github:cao7113/gater"

[tools.gater-client]
version = "latest"
matching = "gater-client"
```

然后执行：

```bash
mise use gater-client
```

mise 默认会隐藏刚发布、尚未达到 `minimum_release_age` 的版本。发布后如果需要立即安装，可以临时关闭这个保护：

```bash
mise settings set minimum_release_age 0
mise use github:cao7113/gater
```

安装完成后可以恢复默认设置：

```bash
mise settings unset minimum_release_age
```

## 启动环节的优化建议

建议按以下顺序演进：

1. **先修正启动状态机**：为每次启动保存一个完成通道和唯一进程句柄，让并发请求共享同一次启动结果；将 `cmd.Wait` 集中到一个 goroutine，`Stop` 只发信号并等待该结果，避免重复 `Wait` 和持锁等待。
2. **把配置解析前移并统一**：注册时一次性解析 `app.yaml` 的 `name/path/cmd/args/env/idle_timeout`，定义字段优先级和校验错误；不要让 `App` 再隐式读取进程环境来决定关键行为。
3. **使用显式 readiness**：保留端口探测作为默认策略，同时支持 `healthcheck`（HTTP URL、状态码、超时）和 `stop_timeout`。仅能连接端口不代表应用已经可以处理请求。
4. **明确进程退出原因**：区分启动失败、readiness 超时、用户停止、空闲回收和应用崩溃，保存退出码/信号/时间，并在重试前清理旧句柄和日志状态。
5. **稳定端口与并发控制**：用持久化端口或按应用名分配端口，避免重启后变化；增加端口冲突检查；对注册、启动、停止操作建立单应用锁，避免重复进程。
6. **强化环境与安全边界**：为 launchd 显式配置 PATH、HOME 和必要环境；考虑 `clear_env`、允许目录/命令白名单、敏感环境变量不回显；将日志从固定内存缓冲扩展为可轮转文件。
7. **补齐可观测性和测试**：增加启动耗时、readiness 耗时、退出原因指标；为 `$PORT`、YAML 合并、进程提前退出、readiness 超时、重复启动、进程组清理和 idle stop 添加集成测试。

