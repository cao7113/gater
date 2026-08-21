# Gater

## 构建与发布

项目包含两个命令：`gater` 是命令行客户端，`server` 是后台服务。

本地构建：

```bash
go build -o bin/gater ./cmd/client
go build -o bin/server ./cmd/server
```

查看版本：

```bash
bin/gater -version
bin/server -version
```

发布版本使用 GoReleaser。创建并推送一个 `v*` tag 后，GitHub Actions 会自动发布多平台产物：

```bash
git tag v0.1.0
git push origin v0.1.0
```

发布后可以通过 mise 的 GitHub backend 安装客户端：

```bash
mise use github:cao7113/gater
```

