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

- preset app tmpls
- domain belongs to app when start but can change
- beautiful docs
- gater run mode: dev or prod?
- what relationship app-log and gater-log
- improve exec model
  - pre-hook and env setup
- livebook app
- apps registry

## 预制应用模版

注册新应用页面，优化预设应用模版设置：
- “常用命令快捷预设” 改为“常用快捷应用”
- 目前通过app.js前台设置，评估从后台api获取是否更好
  - 点击行为：当启动命令等设置项 非空时点击，提示将要覆盖已有内容
- 目前仅需支持Phoenix（Elixir），Bun（Dev），Python HTTP

## 注册新应用

Web UI上分两种方式：
- 导航用户选择一个包含app.yaml的目录，解析app.yaml获得配置 来生成应用
- 用户填充填写所有的 注册必填项进行注册，参照现在的方式
- 目前的预制应用是通过前端app.js实现的，考虑是否可通过后端api和配置支持，更灵活和稳定

CLI 部分
- 主要通过选择 app.yaml方式创建

先评估和优化方案，确认后再执行