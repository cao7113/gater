# 技术选型

总结与选型建议
如果你拿 Go 的 httputil.ReverseProxy 与它们对比：

综合开发体验与稳定性：Go 依然是甜点区（Sweet Spot）
Go 将 ReverseProxy 放在标准库中，意味着你不需要引入任何第三方依赖，就能获得经过全球开发者十几年检验的生产级代理能力。对于绝大多数 API 网关、进程管理器（如你的 Gater 项目）来说，Go 是综合开发效率最高、稳定性最好的选择。

追求极致吞吐与最低延迟：选 Rust (Pingora)
如果你的网关需要处理 10万+ QPS、对 GC 延迟极度敏感，或者需要直接操作底层 TCP/TLS 握手，Rust 是唯一的升级方向。

追求绝对隔离与长连接高容错：选 Elixir (Plug + Mint)
如果你的应用重度依赖实时 WebSocket、需要防范各类不可预知的上游崩溃，且希望系统具备“任凭风浪起，自我修复（Let it crash）”的特性，Elixir 的生态表现会非常惊艳。