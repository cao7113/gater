# AGENTS.md

## 项目概览

- 这是一个以 Go 为主的 macOS 本地 Web 网关项目。
- 服务端代码位于 `cmd/server`，客户端代码位于 `cmd/client`。
- Web 资源位于 `web/src`，生成文件位于 `web/dist`。

## 开发环境

- 必须使用 `mise.toml` 管理工具版本，不要绕过项目版本直接使用系统工具。
- Go 版本：`1.26.6`。
- Bun 版本：`1.3.14`。

## 常用命令

- Go 测试：`go test ./...`
- Go 构建：`go build -o bin/server ./cmd/server` 和 `go build -o bin/client ./cmd/client`
- 前端资源构建：`bun run build.css`、`bun run build.js`、`bun run cp.pages`

## 本地部署运行环境

- gater 以 macOS `launchd agent` 运行，使用类似`lab/agent.plist`的配置
- agent 直接启动 `bin/server`，例如传入`--port 8888`；进程由 `launchd` 管理并保持运行
- 本地可使用 `lctl json lab.gater` 查看相关配置

## 托管App的生成过程和流量接管（多层反向代理透传）

- `launchd daemon` 运行caddy server，拦截 443和80端口的网络流量，卸载tls加密，并`reverse_proxy localhost:8888` 到本地端口，开发模式下为8080端口
- `launchd agent` 运行的gate进程监听8888端口的流量，根据HOST（如demo.s)，定位到具体的应用配置app.yaml
- gater基于golang的`httputil.ReverseProxy` 将流量分发给动态启动的App进程
- 具体的App，如livebook，可能再根据需要产生其它动态进程

## 代码约定

- 优先保持 Go 标准库风格，提交前运行 `gofmt`，并为行为变化补充或更新测试。
- 修改 Web 资源时保持现有 Tailwind、DaisyUI 和 Alpine.js 的用法；不要提交可生成的 `web/dist` 文件，除非项目流程明确要求。
- 保持改动聚焦，不要提交构建产物、临时文件或本地配置。