# Prompt

构建macOS本地服务代理

前提：
- 主要是web服务，如phoenix，包含http和websocket流量
- *.lab 泛域名 本地已被 dnsmasq 解析到 127.0.0.1，后期泛域名和地址都可以通过启动参数配置

主要配置:
- 每个应用的输入是一个 应用路径，如 /path/to/phx-demo
- 每个应用路径下有app.yaml配置文件，包括启动命令，参数，环境变量，log目录等

主要功能：
- 代理服务运行在 admin.lab:<port> 下
- 当访问 phx-demo.lab:<port>时，查看对应应用是否运行；如果没有运行，根据应用配置，自动拉启
- 当 phx-demo 空闲一定时间时，需要能关闭服务，节约资源消耗
- 访问admin 的页面
  - 查看应用列表，运行状态，距离关闭还有多久，查看运行log

## todo

- 更好的配置方法
- launchd后台运行
- 运行在80端口
- 添加https