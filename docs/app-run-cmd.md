# App 一次性命令设计

## 1. 设计目的

Phoenix 等应用除了长期运行的 Web server，还需要在相同的应用环境中执行一次性命令，例如：

```bash
mix ecto.migrate
mix ecto.rollback
mix run priv/repo/seeds.exs
```

一次性命令需要满足：

- 复用应用的工作目录、工具链和环境变量；
- 可以访问与当前 server 相同的数据库配置；
- 不需要停止、重启或替换当前 server；
- 不申请 Web server 端口，除非命令明确需要；
- 不改变 App 当前的运行状态、PID、日志和 idle timeout；
- 未来可以扩展为 seed、rollback、assets、console 等命令。

核心原则是：**server 和一次性命令共享运行上下文，但拥有独立的进程生命周期。**

## 2. 当前 server 运行模型

当前 App 启动流程大致如下：

```text
访问 App
  -> App.Run
  -> 分配 PORT 和 endpoint 端口
  -> 执行 AppType Prepare / BeforeStart
  -> 启动 cmd + args
  -> 等待端口监听
  -> StateRunning
  -> idle monitor 管理进程
```

这条流程只适合长期运行的 Web server。一次性命令不应直接复用 `App.Run`，原因是它会：

- 分配端口；
- 等待端口监听；
- 修改 `State`、`Cmd`、`Pid` 等 server 生命周期字段；
- 受 idle timeout 管理；
- 与当前 App server 的停止和异常退出逻辑耦合。

因此，`RunCommand` 应当是 App 的另一条执行路径，而不是 `Run` 的特殊参数。

## 3. 配置格式

在 `app.yaml` 中增加命令定义：

```yaml
name: phoenix-demo
domain_suffix: .s
app_type: phx
cwd: /path/to/phoenix-demo

cmd: mix
args:
  - phx.server

env:
  DATABASE_URL: postgres://localhost/phoenix_demo
  MIX_ENV: dev

commands:
  migrate:
    cmd: mix
    args:
      - ecto.migrate

  rollback:
    cmd: mix
    args:
      - ecto.rollback

  seed:
    cmd: mix
    args:
      - run
      - priv/repo/seeds.exs
```

命令可以只覆盖需要变化的字段，未填写的字段继承 App 的默认配置：

```yaml
commands:
  migrate:
    args:
      - ecto.migrate
    env:
      MIX_ENV: dev
    unset_env:
      - PHX_SERVER
```

推荐的配置类型：

```go
type CommandConfig struct {
    Cmd string            `yaml:"cmd,omitempty" json:"cmd,omitempty"`
    Args []string          `yaml:"args,omitempty" json:"args,omitempty"`
    Cwd  string            `yaml:"cwd,omitempty" json:"cwd,omitempty"`
    Env  map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
    UnsetEnv []string      `yaml:"unset_env,omitempty" json:"unset_env,omitempty"`
}

type AppConfig struct {
    // 现有字段...
    Commands map[string]CommandConfig `yaml:"commands,omitempty" json:"commands,omitempty"`
}
```

约定如下：

- `commands` 的 key 是命令名称，例如 `migrate`、`rollback`；
- `cmd` 为空时继承 App 的 `cmd`；
- `args` 不继承 App 的启动参数；命令未填写 `args` 时使用空参数；
- `cwd` 为空时继承 App 的 `cwd`；
- `env` 按 key 覆盖 App 的环境变量；
- `unset_env` 从继承后的环境中移除指定变量，例如 Phoenix 迁移时移除 `PHX_SERVER`；
- `unset_env` 中的变量必须是合法环境变量名，不能重复，也不能同时出现在同一命令的 `env` 中；
- 命令名称必须非空，并且在同一个 App 内唯一；
- 命令配置不参与 App 路由名称空间。

对于 Phoenix，常见的配置可以简化为：

```yaml
cmd: mix
args: [phx.server]

commands:
  migrate:
    args: [ecto.migrate]
  rollback:
    args: [ecto.rollback]
```

## 4. 运行上下文与生命周期分层

执行逻辑应拆成“构造上下文”和“运行进程”两层。

### 4.1 构造运行上下文

共享的上下文构造逻辑负责：

- 检查并解析 `cwd`；
- 查找可执行命令；
- 继承 Gater 的系统环境；
- 合并 App 配置中的 `env`；
- 展开 `${APP_NAME}`、`${APP_DOMAIN}` 等占位符；
- 执行 AppType 的准备逻辑；
- 生成最终的命令、参数和环境变量。

可以引入运行模式：

```go
type RunMode int

const (
    RunServer RunMode = iota
    RunCommand
)
```

上下文构造器根据模式决定是否分配端口：

```go
func (a *App) buildRunContext(
    ctx context.Context,
    mode RunMode,
    command *CommandConfig,
) (*types.TypeContext, error)
```

### 4.2 Server 生命周期

`RunServer` 继续负责：

- 分配 `PORT` 和 endpoint 端口；
- 使用 App 默认的 `cmd` 和 `args`；
- 启动长期运行进程；
- 等待主端口和 endpoint 端口就绪；
- 更新 `State`、`Pid`、`RuntimeEnv`；
- 接受代理请求并刷新 `LastActive`；
- 由 idle monitor 停止。

### 4.3 一次性命令生命周期

`RunCommand` 负责：

- 解析命令配置；
- 使用独立的 `exec.Cmd`；
- 将 stdout 和 stderr 返回或转发给调用方；
- 等待命令结束；
- 返回命令退出错误和退出码。

一次性命令不应：

- 调用 `App.Run`；
- 修改 `App.State`；
- 覆盖 `App.Cmd` 或 `App.Pid`；
- 调用 `waitForPortOrExit`；
- 触发 idle timeout；
- 停止或重启当前 server。

目标调用形式：

```go
func (a *App) RunCommand(ctx context.Context, name string) error
```

内部执行模型：

```go
func (a *App) RunCommand(ctx context.Context, name string) error {
    command, ok := a.Config.Commands[name]
    if !ok {
        return fmt.Errorf("命令不存在: %s", name)
    }

    runContext, err := a.buildRunContext(ctx, RunCommand, &command)
    if err != nil {
        return err
    }

    cmd := exec.CommandContext(ctx, runContext.Command, runContext.Args...)
    cmd.Dir = runContext.WorkingDir
    cmd.Env = ToEnvList(runContext.Env)
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr

    return cmd.Run()
}
```

上面的代码表达设计意图，具体字段名可以根据现有 `TypeContext` 的结构调整。

## 5. 端口和环境变量规则

### 5.1 默认不为一次性命令分配端口

迁移命令通常只需要数据库连接，不需要监听 HTTP 端口。因此：

```text
RunServer  -> 分配 PORT 和 endpoint 端口
RunCommand -> 默认不分配端口
```

一次性命令默认不应注入 `PORT`、`IFRAME_PORT` 等动态端口变量，避免命令误以为自己需要启动 Web server。

如果未来某个命令确实需要端口，可以增加显式配置，例如：

```yaml
commands:
  inspect:
    args: [run, priv/tools/inspect.exs]
    use_server_env: true
```

`use_server_env` 需要谨慎设计，第一版可以暂不支持。

### 5.2 环境合并顺序

推荐的优先级从低到高为：

```text
Gater 进程环境
  -> App env
  -> Command env
  -> 运行模式生成的变量
```

对于一次性命令，最后一层默认不生成动态端口变量；对于 server，则生成 `PORT` 和 endpoint 端口变量。

命令环境示例：

```yaml
env:
  DATABASE_URL: postgres://localhost/app
  MIX_ENV: dev

commands:
  migrate:
    env:
      MIX_ENV: dev
```

此时 `migrate` 继承 `DATABASE_URL`，并使用命令自己的 `MIX_ENV`。

## 6. 与当前 server 并行运行

命令和 server 应当是两个独立的子进程：

```text
gater
  ├── mix phx.server
  └── mix ecto.migrate
```

迁移执行期间：

- 当前 server 继续监听原端口；
- 代理请求继续正常处理；
- server 的 `State`、`Pid`、`StartedAt` 不变；
- 迁移命令拥有自己的 PID、stdout、stderr 和退出状态；
- 迁移失败不会自动停止 server。

需要明确数据库层面的并发语义：Gater 不负责替数据库处理迁移锁。Phoenix/Ecto、数据库或迁移工具本身应保证迁移并发安全。Gater 只负责进程级别的隔离。

## 7. 命令并发策略

第一版建议：

- 允许 server 与一次性命令并行；
- 同一个 App 同时只运行一个一次性命令；
- 不同 App 的一次性命令可以并行；
- 命令失败只返回失败结果，不改变 server 状态。

可以在 `App` 中增加独立的命令锁：

```go
type App struct {
    // server 生命周期字段...
    commandMu      sync.Mutex
    commandRunning bool
}
```

如果同一个 App 已有命令运行，再次执行时返回冲突错误，例如：

```text
应用 [phoenix-demo] 已有命令正在运行
```

后续如果需要支持多个长任务，可以把命令执行从 `App` 内的布尔状态升级为 Job Manager，而不改变 server 生命周期模型。

## 8. CLI 和 API

### 8.1 CLI

支持命令组：

```bash
gater-client cmd list <app>
gater-client cmd run <app> <command>
gater-client cmd show <app> <command>
gater-client cmd gen <app> <command>
```

`cmd` 也可以使用别名 `commands`。其中：

- `list` 列出应用已配置的一次性命令；
- `run` 执行指定命令并等待完成；
- `show` 查看命令的最终配置，包括继承后的 `cmd`、`args`、`cwd` 和环境变量。
- `gen` 生成可直接粘贴到 shell 执行的命令片段，但不会执行命令。

为了兼容已有用法，也支持直接执行：

```bash
gater-client run <app> <command>
```

示例：

```bash
gater-client run phoenix-demo migrate
gater-client run phoenix-demo rollback
gater-client run phoenix-demo seed
```

例如，若 App 配置如下：

```yaml
cwd: /workspace/phoenix-demo
cmd: mix
env:
  MIX_ENV: dev

commands:
  migrate:
    args: [ecto.migrate]
    env:
      MIX_ENV: test
```

执行 `gater-client cmd show phoenix-demo migrate` 时，显示的结果会包含：

```json
{
  "name": "migrate",
  "cmd": "mix",
  "args": ["ecto.migrate"],
  "cwd": "/workspace/phoenix-demo",
  "env": {
    "MIX_ENV": "test"
  }
}
```

CLI 应同步等待命令完成，并使用命令退出码作为自身退出码。这样 shell、脚本和 CI 可以直接判断迁移是否成功。

### 8.2 HTTP API

建议增加同步接口：

```http
POST /api/apps/{name}/commands/{command}
GET  /api/apps/{name}/commands
GET  /api/apps/{name}/commands/{command}
```

其中两个 `GET` 接口用于客户端的 `cmd list` 和 `cmd show`，返回内容已经完成 App 默认配置的继承和占位符展开。
命令详情还包含 `shell` 字段，可用于 `cmd gen` 或 Web 页面复制。

成功响应可以包含：

```json
{
  "ok": true,
  "app": "phoenix-demo",
  "command": "migrate",
  "exit_code": 0
}
```

失败响应需要区分：

- 命令不存在：`404` 或 `422`；
- 命令正在运行：`409 Conflict`；
- 命令执行失败：返回非零退出码和可读错误信息；
- Gater 无法创建进程：`500`。

同步接口适合迁移、回滚、seed 等短任务。未来需要长时间运行任务时，再增加 Job 模型：

```http
POST /api/apps/{name}/jobs
GET  /api/jobs/{id}
POST /api/jobs/{id}/cancel
```

Job 模型可以记录：

- Job ID；
- App 和命令名称；
- 创建时间、开始时间、结束时间；
- 运行状态；
- PID；
- 退出码；
- stdout/stderr 日志；
- 取消原因。

## 9. AppType 扩展

AppType handler 当前包含：

```go
type Handler interface {
    Prepare(*TypeContext) error
    BeforeStart(context.Context, *TypeContext) error
    AfterStart(context.Context, *TypeContext) error
}
```

一次性命令不应直接假设所有 `BeforeStart` 和 `AfterStart` 逻辑都适合命令。后续可以将接口扩展为显式区分 server 和 command：

```go
type Handler interface {
    Prepare(*TypeContext) error
    BeforeStart(context.Context, *TypeContext) error
    AfterStart(context.Context, *TypeContext) error
    BeforeCommand(context.Context, *TypeContext) error
    AfterCommand(context.Context, *TypeContext, error) error
}
```

但第一版不建议为了迁移功能立即增加过多 hook。推荐先复用 `Prepare`，只有确实存在差异时再增加命令专用 hook。

例如 Phoenix 的通用环境准备可以复用于 server 和 migrate：

```text
Prepare
  -> 解析 Phoenix 应用上下文
  -> 准备 MIX_ENV、DATABASE_URL 等环境

server
  -> phx.server

command
  -> ecto.migrate
```

## 10. 安全与可观测性

命令配置来自已注册的本地应用，因此执行前仍需遵循 server 启动时的基本校验：

- 校验工作目录存在且为目录；
- 校验可执行文件可以通过 `PATH` 找到；
- 使用独立进程组；
- 使用 `exec.CommandContext` 响应取消；
- 不通过 shell 拼接整条命令；
- 不在普通 API 响应中泄露敏感环境变量。

命令日志应与 server 日志区分，至少包含：

```text
[App: phoenix-demo] command=migrate started
[App: phoenix-demo] command=migrate exited code=0
```

如果输出需要写入现有 `LogBuf`，应增加命令日志的标识，避免把迁移输出误认为 server 输出。更推荐为一次性命令提供独立的输出缓冲区或 Job 日志。

## 11. 分阶段实现计划

### 第一阶段：内部能力

- 增加 `CommandConfig` 和 `AppConfig.Commands`；
- 抽取共享的运行上下文构造逻辑；
- 保持现有 `App.Run` 行为不变；
- 增加 `App.RunCommand`；
- 增加命令配置、环境合并和 server 并行运行测试。

### 第二阶段：CLI 和同步 API

- 增加 `gater-client run <app> <command>`；
- 增加 `POST /api/apps/{name}/commands/{command}`；
- 返回退出码和命令输出；
- 增加命令并发冲突处理。

### 第三阶段：异步 Job

当出现以下需求时再引入 Job：

- 命令执行时间较长；
- 需要在 Web UI 查看实时输出；
- 需要取消正在运行的命令；
- 需要保存命令历史。

## 12. 设计结论

一次性命令应被视为 App 的独立进程任务，而不是 server 生命周期的一部分。

最终关系为：

```text
App
  ├── Server Runtime
  │     ├── PORT
  │     ├── State
  │     ├── Proxy
  │     └── idle timeout
  │
  └── Command Runtime
        ├── migrate
        ├── rollback
        ├── seed
        └── 独立退出码和日志
```

两者共享配置和运行环境，但不共享进程状态。这样可以在不打扰当前 Web server 的前提下执行数据库迁移，也为后续增加更多应用命令保留清晰的扩展边界。
