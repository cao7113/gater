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

- copy app config from exited app with new name in register form
- improve exec model
  - pre-hook and env setup
- livebook app
- beautiful docs
- gater run mode: dev or prod? with standalone store path
- what relationship app-log and gater-log
- apps registry
- app target host config, now only 127.0.0.1

## AppType specific extensions / hooks

- refactor app type Common interface to impl. PHX inject logic
- app type match with app templ and keep simple!

目前App的运行模型 基本都在 app.EnsureStarted

准备基于config.Config.AppType的值（目前可能为： phx，bun等）进行 应用运行机制扩展，插入某些应用配置逻辑
如：
- 运行前 执行preHook 检查和准备环境
- 运行时 插入特定的环境变量，如 PHX_HOST for phx
- postHook

将这个值配合appTemplate进行扩充，因为一般 内置应用都有配置模版，不过这可以放到第二步，先设计前面的hook 插入机制
先分析用什么技术方案 易懂好理解又足够灵活

## App host domain name

- App respect Config port first, then auto-gen
- host domain name config when deploy other than config???
- app uniqueness by app-name other than app-domain?

优化App的host domain管理，目前状况
- 应用的访问域名 是根据 顶部的域名后缀 选择来动态生成的，没必要且不应该，域名后缀应该属于app的属性之一
- 考虑在App中增加domain_suffix字段，用于存储选择的值
- server 配置添加 允许的后缀 列表，默认为： .l.h (for http), .l.s (for https)
- 在注册表单，cli client注册时要明确指定 域名后缀，编辑页面允许修改，检查在允许列表中
- 重构页面展示逻辑，直接通过 app name和 domain_suffix 即可 生成 url并展示，可以移除顶部的域名选择和对应逻辑
- 同理考虑是否应该 增加 scheme字段，目前暂不支持

目前暂不考虑同一个 app 多域名问题，在域名层解决就行

先评估方案合理性并优化方案

有个设计权衡，现实中不太可能出现demo.l.h和demo.l.s，因为注册时就会发现有同名应用了，通过不同的name解决类似需求。之前考虑过使用域名唯一解决，感觉方案有点复杂，而且域名本身可通过域名解析层重定向，这里只处理底层应用启动需求

真实设计是：
App 名称是注册表中的唯一标识
不考虑同名 App 使用不同后缀并存
domain_suffix 只是 App 的访问配置和默认展示信息
域名层负责解析、转发或重定向
Gater 只负责根据最终请求找到 App、按需启动底层服务
Gater 不需要额外维护复杂的“域名唯一性”或“请求后缀必须等于 App 后缀”规则

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