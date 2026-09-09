# App Endpoints 设计

## 1. 设计目的

部分 Web 应用会由同一个进程监听多个端口。以 Livebook 为例：

```text
PORT         -> 主 Web 服务，对应 https://livebook.s
IFRAME_PORT  -> iframe 服务，对应 https://livebook-iframe.s
```

两个端口属于同一个应用进程，因此需要满足：

- 访问不同入口时仍然启动同一个 App 进程；
- 每个入口可以代理到该进程的不同端口；
- 主端口和普通 App 的现有行为保持不变；
- 额外端口只对确实需要的应用启用；
- 所有端口共享同一个运行状态、空闲计时器、日志和进程生命周期。

本设计将额外入口建模为可选的 `endpoint`，而不是创建第二个 App，也不把它建模成远程代理。

## 2. 核心模型

App 继续保留现有主端口：

```text
App name + domain_suffix -> PORT -> 主端口
```

App 可以额外声明 endpoint：

```text
endpoint entry_name + domain_suffix -> endpoint 对应端口
```

例如：

```yaml
name: livebook
domain_suffix: .s

endpoints:
  - entry_name: livebook-iframe
    label: iframe
    port_env: IFRAME_PORT
```

生成的访问入口：

```text
livebook.s        -> PORT
livebook-iframe.s -> IFRAME_PORT
```

其中：

- `name` 是 App 的规范名称，也是主入口名称；
- `aliases` 是主入口的别名，只映射到主端口；
- `endpoints` 是同一 App 的额外端口入口；
- `entry_name` 是对外路由使用的入口名称；
- `label` 是可选的人类可读标签，用于 UI、日志和展示；
- `port_env` 是启动子进程时注入端口的环境变量名。

## 3. 配置格式

推荐的配置结构：

```yaml
name: livebook
domain_suffix: .s
aliases:
  - lb
app_type: phx
cmd: /path/to/livebook
args:
  - start

env:
  PHX_HOST: "${APP_DOMAIN}"

endpoints:
  - entry_name: livebook-iframe
    label: iframe
    port_env: IFRAME_PORT
```

`label` 可以省略：

```yaml
endpoints:
  - entry_name: livebook-iframe
    port_env: IFRAME_PORT
```

此时展示标签默认使用 `entry_name`。等价于：

```yaml
endpoints:
  - entry_name: livebook-iframe
    label: livebook-iframe
    port_env: IFRAME_PORT
```

建议的 Go 配置类型：

```go
type EndpointConfig struct {
    EntryName string `yaml:"entry_name" json:"entry_name"`
    Label     string `yaml:"label,omitempty" json:"label,omitempty"`
    PortEnv   string `yaml:"port_env" json:"port_env"`
}

type AppConfig struct {
    // 现有字段...
    Endpoints []EndpointConfig `yaml:"endpoints,omitempty" json:"endpoints,omitempty"`
}
```

`EndpointConfig` 可以提供展示名称方法：

```go
func (e EndpointConfig) DisplayLabel() string {
    if strings.TrimSpace(e.Label) != "" {
        return e.Label
    }
    return e.EntryName
}
```

## 4. 名称与路由关系

### 4.1 统一名称空间

App 的以下名称进入同一个名称空间，并且必须全局唯一：

- App `name`；
- App `aliases`；
- endpoint `entry_name`。

例如：

```yaml
App A:
  name: livebook
  endpoints:
    - entry_name: livebook-iframe
```

则下面的配置必须被拒绝：

```yaml
App B:
  name: livebook-iframe
```

因为它会和 App A 的 endpoint 入口冲突。

名称比较继续沿用现有规则：去除首尾空白并转换为小写。

```text
LiveBook == livebook
```

`label` 不参与路由，也不参与全局唯一性校验。修改 label 不应影响已经存在的访问地址。

### 4.2 `name`、`aliases` 与 `endpoint`

```text
name:
  App 的规范名称，映射到主端口。

aliases:
  App 主入口的别名，同样映射到主端口。

endpoint entry_name:
  App 额外端口的公开入口名称，映射到指定 endpoint 端口。
```

例如：

```yaml
name: livebook
aliases:
  - lb
endpoints:
  - entry_name: livebook-iframe
    label: iframe
    port_env: IFRAME_PORT
```

路由结果：

```text
livebook.s        -> livebook App, main endpoint
lb.s              -> livebook App, main endpoint
livebook-iframe.s -> livebook App, iframe endpoint
```

暂不自动生成 `lb-iframe.s`。alias 只代表主入口，额外 endpoint 必须通过自己的 `entry_name` 显式声明，避免路由数量和冲突规则隐式扩张。

### 4.3 不使用 `label` 作为入口

`label` 主要服务于人类阅读，例如：

```yaml
entry_name: livebook-iframe
label: iframe
```

如果使用 `label` 作为域名或路由键，会导致展示文案修改时访问地址和内部索引同时变化。因此路由、持久化引用和唯一性校验都应使用稳定的 `entry_name`。

## 5. 请求处理流程

当前普通 App 的流程保持不变：

```text
请求 Host
  -> 去掉允许的 domain_suffix
  -> 查找 App name 或 alias
  -> 启动 App
  -> 代理到 App.Port
```

增加 endpoint 后，路由表应能解析为 App 和入口类型：

```text
请求 Host
  -> 去掉允许的 domain_suffix
  -> 查询统一名称索引
  -> 得到 App + endpoint 名称
  -> 启动同一个 App
  -> 根据 endpoint 选择目标端口
  -> ReverseProxy
```

内部路由记录可以抽象为：

```go
type Entry struct {
    Name     string // name、alias 或 endpoint entry_name
    AppName  string
    Endpoint string // main 或 endpoint 的内部标识
}
```

Manager 可以保留现有接口：

```go
GetApp(name string)
```

并新增面向请求路由的接口：

```go
GetEntry(name string) (*app.App, string, bool)
```

普通管理 API 和旧逻辑继续使用 `GetApp`；代理请求使用 `GetEntry`，从而避免一次性改写整个 App 管理模型。

## 6. 端口分配与环境变量

主端口继续使用现有 `App.Port` 逻辑：

- 配置了固定端口时使用固定端口；
- 否则动态分配本地端口；
- 主端口注入环境变量 `PORT`，除非现有配置明确覆盖该值。

endpoint 端口只在 App 声明 endpoint 时分配：

```text
PORT        = 43121
IFRAME_PORT = 43122
```

不能通过 `PORT + 1` 推算额外端口。每个端口都必须独立申请，以避免端口被其他进程占用。

运行时可以在 App 内部增加额外端口表，例如：

```go
map[string]int{
    "iframe": 43122,
}
```

其中 key 使用 endpoint 的内部标识，建议由 `entry_name` 或配置数组中的稳定 endpoint 记录确定。额外端口属于同一个 App，停止 App 时必须一起清理。

## 7. 启动与探活

请求任意入口时都启动同一个 App：

```text
首次访问 livebook.s 或 livebook-iframe.s
  -> 分配 PORT 和 IFRAME_PORT
  -> 构造完整进程环境
  -> 启动一个 Livebook 进程
  -> 等待主端口和 endpoint 端口就绪
  -> 进入 Running
```

第一版建议所有声明的 endpoint 都是 required：

- 主端口可连接；
- 每个 endpoint 端口可连接；
- 任一端口探活失败，则整个 App 启动失败。

这样可以避免 App 已显示运行，但 `livebook-iframe.s` 永远返回 502 的半可用状态。

未来如果确实存在可选端口，再增加 `required` 字段；当前不为少数场景提前增加配置复杂度。

## 8. ReverseProxy 设计

普通 App 继续使用现有单端口代理对象。

endpoint 请求需要根据路由选择目标端口：

```text
main endpoint   -> 127.0.0.1:App.Port
iframe endpoint -> 127.0.0.1:extraPorts["iframe"]
```

可以采用以下低侵入方式：

```go
app.Proxy.ServeHTTP(w, r)              // 主入口，保持现有路径
app.ProxyEndpoint(w, r, endpointName) // 额外入口
```

代理仍然使用 Go 标准库 `httputil.ReverseProxy`，并继续支持 HTTP、WebSocket 和流式响应所需的连接透传行为。

代理对象可以按 endpoint 分别创建，或者在 Rewrite 阶段按请求选择端口。优先保持实现简单：主端口保留现有代理，额外 endpoint 使用按端口创建的代理对象。

## 9. 生命周期与空闲回收

endpoint 不创建新的 App，也不创建独立的生命周期：

- 所有入口共享一个进程；
- 所有入口共享一个 `State`；
- 所有入口访问都会刷新同一个 `LastActive`；
- 日志归属于同一个 App；
- 空闲超时后停止整个进程；
- 停止时释放主端口和全部 endpoint 端口。

因此访问 `livebook-iframe.s` 不会再启动第二个 Livebook 进程。

## 10. 配置校验

注册和更新 App 时应校验：

- `entry_name` 必填；
- `entry_name` 经过规范化后必须唯一；
- `entry_name` 不能与任意 App `name`、`alias` 或其他 endpoint 冲突；
- `entry_name` 应是合法的 DNS label 前缀，不包含点号和端口；
- `entry_name` 不应包含首尾连字符；
- `port_env` 必填且必须是合法环境变量名；
- 同一个 App 的多个 endpoint 不能重复使用同一个 `port_env`；
- endpoint 生成的完整入口必须使用该 App 的 `domain_suffix`；
- `label` 可以为空，不参与路由冲突判断。

建议的名称示例：

```text
iframe
assets
api-v2
```

不建议：

```text
iframe.s       # entry_name 不包含 suffix
iframe:4001    # entry_name 不包含端口
-livebook      # 不能以连字符开头
```

## 11. AppType Handler 的边界

多端口能力属于 App 的通用生命周期能力，不应只实现于 Phoenix Handler。未来 Bun、Node 或其他应用类型也可能需要多个端口。

App 层负责：

- 分配主端口和 endpoint 端口；
- 构造端口环境变量；
- 等待所有 required 端口就绪；
- 选择代理目标；
- 统一停止和释放资源。

AppType Handler 负责应用类型相关的逻辑：

- 补充或调整类型专属环境变量；
- 处理 Phoenix 等框架的启动准备；
- 执行类型专属的启动前、启动后检查。

例如 Phoenix 可以读取通用端口上下文并使用 `IFRAME_PORT`，但不负责维护端口生命周期。

## 12. 兼容性与实施边界

第一阶段只增加可选 endpoint，不重构主端口模型：

- `App.Port` 保留；
- 普通 App 的 `app.yaml` 不需要修改；
- 现有 `name`、`aliases` 和主域名行为保持兼容；
- endpoint 配置为空时，运行逻辑与现在相同；
- 不自动生成 alias 对应的 endpoint 入口；
- 不引入远程 URL 或跨 App 代理语义；
- 不把 endpoint 注册成第二个独立 App。

后续如果多端口应用成为常见场景，再考虑把主端口也统一抽象为 `Endpoint`。当前优先采用增量方案，控制对现有架构的影响。

## 13. 完整示例

```yaml
name: livebook
domain_suffix: .s
aliases:
  - lb
app_type: phx
cmd: /path/to/livebook
args:
  - start

env:
  PHX_HOST: "${APP_DOMAIN}"
  PHX_SERVER: "1"

endpoints:
  - entry_name: livebook-iframe
    label: iframe
    port_env: IFRAME_PORT
```

运行时环境：

```text
PORT=43121
IFRAME_PORT=43122
PHX_HOST=livebook.s
```

访问映射：

```text
https://livebook.s/path
  -> http://127.0.0.1:43121/path

https://lb.s/path
  -> http://127.0.0.1:43121/path

https://livebook-iframe.s/path
  -> http://127.0.0.1:43122/path
```

三个入口都指向同一个 Livebook 进程，并共享同一个 App 生命周期。
