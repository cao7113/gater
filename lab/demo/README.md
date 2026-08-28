# Gater Demo

这个 demo 验证 Gater 的最小启动闭环：注册一个目录、首次代理访问时启动 Python、注入 `$PORT`、查看日志并手动停止。

## 运行

在仓库根目录执行：

```bash
go run ./cmd/server -store /tmp/gater-demo-store.yaml
```

另开终端，在 `lab/demo` 目录注册应用：

```bash
curl -X POST http://localhost:8080/api/apps \
  -H 'Content-Type: application/json' \
  -d '{"name":"demo","cwd":"'"$PWD"'"}'
```

注册成功后，以下请求会触发子进程启动，并由 Gater 转发到 Python HTTP server：

```bash
curl -H 'Host: demo.lab:8080' http://localhost:8080/
```

观察状态和日志：

```bash
curl http://localhost:8080/api/apps/demo
curl http://localhost:8080/api/apps/demo/logs
```

停止应用：

```bash
curl -X POST http://localhost:8080/api/apps/demo/stop
```

`app.yaml` 中的 `cmd`、`args`、`env` 和 `idle_timeout` 会在注册时作为缺省配置使用，注册请求中的字段优先。