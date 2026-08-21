# Gater

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

