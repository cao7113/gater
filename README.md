# Gater - MacOS Web App Gateway

Gater 是一个面向 macOS 本地环境的按需启动反向代理。它把已注册的应用映射到指定的域名，如`demo.l.s`，第一次访问时才启动应用，应用空闲后自动停止。

## 启动模型

1. `gater` 启动时创建 Store 和 Manager，从 `~/.config/gater/store.yaml` 恢复已注册应用；恢复只创建内存实例，不会拉起子进程
2. 通过 Web 控制台、API 或客户端注册应用
3. 请求 `demo.l.s` 时，代理根据 Host 找到应用。应用不是 `running` 时进入 `starting`
4. 子进程使用独立进程组，标准输出和标准错误同时写入 Gater 日志与内存日志缓冲。
5. 每次代理请求会刷新 `LastActive`。后台监视器每 3 秒检查空闲时间，超过 `idle_timeout` 就向进程组发送 `SIGINT`，3 秒后仍未退出则发送 `SIGKILL`。
6. Gater 收到退出信号时取消全局 Context，停止所有应用，再关闭 HTTP 服务。

状态含义：`stopped` 未运行，`starting` 已启动但仍在等待端口，`running` 已通过端口探测，`crashed` 启动失败、就绪超时或运行中异常退出。

## Demo app.yaml

`lab/demo/app.yaml`

## 命令

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

## 发布构建

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
