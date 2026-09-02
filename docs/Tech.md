# 技术选型

总结与选型建议
如果你拿 Go 的 httputil.ReverseProxy 与它们对比：

综合开发体验与稳定性：Go 依然是甜点区（Sweet Spot）
Go 将 ReverseProxy 放在标准库中，意味着你不需要引入任何第三方依赖，就能获得经过全球开发者十几年检验的生产级代理能力。对于绝大多数 API 网关、进程管理器（如你的 Gater 项目）来说，Go 是综合开发效率最高、稳定性最好的选择。

追求极致吞吐与最低延迟：选 Rust (Pingora)
如果你的网关需要处理 10万+ QPS、对 GC 延迟极度敏感，或者需要直接操作底层 TCP/TLS 握手，Rust 是唯一的升级方向。

追求绝对隔离与长连接高容错：选 Elixir (Plug + Mint)
如果你的应用重度依赖实时 WebSocket、需要防范各类不可预知的上游崩溃，且希望系统具备“任凭风浪起，自我修复（Let it crash）”的特性，Elixir 的生态表现会非常惊艳。

## Golang

常用参数说明
-v：输出详细日志（会打印 t.Log 以及每个测试函数的 PASS/FAIL 状态）。

-run <regexp>：使用正则表达式匹配要运行的测试函数名。

-count=1：强制禁用测试缓存（如果你改了配置文件或环境变量，建议加上）。

示例组合：

Bash
go test -v -count=1 -run TestEnsureRunning ./internal/app