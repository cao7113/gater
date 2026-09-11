# App 列表排序方案

## 1. 背景

App 列表最初由 `GET /api/apps` 返回。运行时的 App 集合保存在 Go map 中，而 Go map 不保证遍历顺序，因此不能直接依赖 map 的遍历结果作为 UI 展示顺序。

当前方案为：

- 运行时继续使用 map 保存 App 实例，保证按名称查找简单高效；
- 在 store 持久化文件中增加独立的 `apps_order` 列表，保存 UI 展示顺序；
- 前端通过拖拽调整顺序；
- 后端校验并持久化完整顺序；
- 新注册的 App 自动放到列表头部。

排序信息属于列表展示状态，不属于 App 自身运行配置，因此不放入单个 App 的 `app.yaml`。

## 2. 设计目标

1. App 重启后保持用户调整的顺序。
2. 前端可以直接拖拽排序，不需要修改 App 名称或访问域名。
3. 新增 App 自动出现在头部，方便发现刚注册的服务。
4. 删除 App 时清理对应排序记录。
5. 旧版本 store 文件可以继续读取。
6. 排序错误或请求不完整时，不破坏现有 App 配置。
7. 尽量保持现有按名称查找、别名解析和反向代理逻辑不变。

## 3. 持久化数据结构

`store.yaml` 使用顶层 `apps` 和 `apps_order`：

```yaml
apps:
  livebook:
    name: livebook
    domain_suffix: .s
    cmd: /path/to/livebook
  jupyter:
    name: jupyter
    domain_suffix: .s
    cmd: /path/to/jupyter

apps_order:
  - jupyter
  - livebook
```

`apps_order` 中保存 App 的规范名称 `name`，不保存 alias、endpoint 名称或展示文本。

App 名称目前作为稳定标识使用。当前更新接口会强制保留原名称，因此暂不支持通过普通编辑操作修改 `name`。

## 4. 为什么使用独立的 `apps_order`

### 4.1 保留 map 的收益

运行时的 `Manager.apps` 继续是：

```go
map[string]*app.App
```

这样可以保持：

- `GetApp(name)` 按名称 O(1) 查找；
- `GetEntry(name)` 根据名称、alias 或 endpoint 快速找到 App；
- 注册和删除时的唯一性检查逻辑基本不变；
- 现有代理、API 和服务端代码改动较小。

### 4.2 为什么不把 `apps` 改成数组

也可以将 store 改成数组：

```yaml
apps:
  - name: jupyter
    domain_suffix: .s
    cmd: /path/to/jupyter
  - name: livebook
    domain_suffix: .s
    cmd: /path/to/livebook
```

这样顺序天然存在，不需要 `apps_order`，但代价是：

- 按名称查找需要遍历数组，变为 O(n)；
- 保存、更新、删除和唯一性校验都要重写；
- 现有 map 格式 store 文件需要迁移；
- Manager 仍然需要 `names` 和 `entries` 两个索引 map 来支持 alias、endpoint 和代理路由；
- 实际上会变成“App 数组 + 多个索引 map”，并不会完全消除 map；
- 对当前功能来说，改造范围明显大于增加一个独立顺序列表。

因此在当前项目阶段，`map + apps_order` 是更低风险、职责也更清楚的方案。

## 5. 排序规则

### 5.1 新注册 App

新 App 注册成功后，插入 `apps_order` 的头部：

```text
旧顺序: [livebook, jupyter]
注册 api 后: [api, livebook, jupyter]
```

这个行为在 `Store.Save` 中处理。只有真正新增的 App 才会头插，更新已有 App 不会改变其当前位置。

### 5.2 前端拖拽

前端拖拽结束后，将当前列表中的全部 App 名称按顺序提交给后端。后端只接受完整顺序，不接受局部的“移动第几个”操作。

完整顺序的好处是：

- API 语义简单，容易测试；
- 不需要后端理解前端拖拽前后的两个位置；
- 多次拖拽最终只保存一个明确结果；
- 可以校验是否漏掉或重复了 App。

### 5.3 缺失或无效顺序

加载 store 时会规范化 `apps_order`：

1. 保留仍然存在于 `apps` 中的名称；
2. 去掉不存在的名称；
3. 去掉重复名称；
4. 对没有出现在 `apps_order` 中的现有 App，按名称排序后追加到末尾。

例如：

```yaml
apps_order:
  - jupyter
  - deleted-app
  - jupyter
```

如果现有 App 是 `jupyter`、`livebook`、`api`，规范化后为：

```text
[jupyter, api, livebook]
```

其中 `api` 和 `livebook` 是缺失项，按名称排序后追加。

API 返回列表时也会对运行时中未出现在顺序列表的 App 做相同的兜底：先返回已保存顺序，再将剩余 App 按名称排序追加，避免任何 App 从 UI 消失。

## 6. 后端职责

### 6.1 Store

`internal/store/store.go` 负责：

- 保存 `AppsOrder []string`；
- 新 App 头插；
- 删除 App 时移除名称；
- 返回顺序副本，避免调用方直接修改内部切片；
- 接收并持久化新的完整顺序；
- 加载时规范化顺序；
- 兼容旧版 store 文件。

Store 的顺序写入和 App 配置写入都使用同一把锁，避免并发持久化时互相覆盖。

### 6.2 Manager

`internal/manager/manager.go` 负责：

- 暴露当前顺序；
- 校验提交的顺序中每个 App 都存在；
- 拒绝重复名称；
- 拒绝缺少 App 的不完整列表；
- 校验通过后委托 Store 持久化。

`names` 和 `entries` 仍然用于路由索引，不应与 `apps_order` 混用：

```text
apps_order: UI 展示顺序
names:      name/alias 到规范 App name 的索引
entries:    name/alias/endpoint 到 App 和 endpoint 的索引
```

### 6.3 API

获取列表：

```http
GET /api/apps
```

返回的数组已经是服务端确定的展示顺序。

保存顺序：

```http
PUT /api/apps/order
Content-Type: application/json
```

请求体：

```json
{
  "order": ["jupyter", "livebook", "api"]
}
```

成功返回：

```json
{
  "ok": true
}
```

以下情况返回 `400`：

- JSON 格式错误；
- 名称不存在；
- 名称重复；
- 顺序没有包含全部 App。

## 7. 前端职责

`web/src/app.js` 负责拖拽交互和请求保存：

1. `/api/apps` 返回什么顺序，就按什么顺序显示；
2. 用户搜索时不允许拖拽，避免对过滤后的局部列表产生歧义；
3. 拖拽期间暂停 2 秒一次的自动刷新；
4. 拖拽完成后先更新本地列表，提供即时反馈；
5. 调用 `PUT /api/apps/order` 保存完整名称数组；
6. 保存失败时重新请求 `/api/apps`，恢复服务端顺序并提示错误；
7. 保存期间继续暂停自动刷新，避免轮询覆盖乐观更新。

当前使用浏览器原生 HTML5 Drag and Drop，不增加第三方拖拽依赖。

## 8. 生命周期行为

### 注册

```text
校验配置
  -> 保存 App 配置
  -> 将 App name 插入 apps_order 头部
  -> 创建运行时实例
  -> 建立 name/alias/endpoint 索引
```

### 更新

普通更新保持原 App name 和原排序位置：

```text
更新配置
  -> 保存同名 App
  -> apps_order 不变
```

当前不支持通过普通更新修改 name。

### 删除

```text
停止实例
  -> 删除运行时 App
  -> 删除 name/alias/endpoint 索引
  -> 从 apps_order 删除 App name
  -> 持久化 store
```

### 重启

```text
读取 store.yaml
  -> 加载 apps
  -> 加载并规范化 apps_order
  -> 恢复运行时实例
```

## 9. 兼容性

旧版 store 可能没有 `apps_order`，或者使用旧的顶层 App map 格式。加载时：

- 旧格式仍然可以读取；
- 没有顺序记录的 App 按名称生成初始顺序；
- 新增、删除或拖拽保存后，会使用包含 `apps_order` 的新格式写回。

因此升级不要求用户手动迁移现有 App 配置。

## 10. 名称修改与未来重命名

当前 `name` 是稳定标识，普通更新会执行：

```go
cfg.Name = canonical
```

因此不会出现只修改 App 配置名称却忘记同步 `apps_order` 的问题。

如果未来需要支持真正的重命名，不能只修改 `AppConfig.Name`，需要增加专门的原子操作，至少同步更新：

1. `apps` map 的 key；
2. `AppConfig.Name`；
3. `apps_order` 中的名称；
4. `names` 索引；
5. `entries` 索引；
6. 相关别名和 endpoint 引用；
7. 持久化文件。

重命名失败时应保持旧名称和旧排序不变。除非确实需要支持重命名，否则继续把 name 作为不可变标识更容易维护。

## 11. 测试重点

后续修改排序逻辑时，应覆盖：

- 新 App 插入头部；
- 更新 App 不改变顺序；
- 删除 App 同步删除排序记录；
- 拖拽提交的顺序可以持久化；
- 顺序包含未知名称时返回 `400`；
- 顺序包含重复名称时返回 `400`；
- 顺序漏掉 App 时返回 `400`；
- 旧 store 没有 `apps_order` 时可以正常加载；
- 缺失顺序项会按名称追加；
- 服务重启后列表顺序保持不变。

## 12. 当前结论

暂时采用：

```text
运行时:       map[string]*app.App
持久化配置:   apps map
展示顺序:     apps_order []string
用户操作:     前端拖拽
保存接口:     PUT /api/apps/order
新增行为:     新 App 放在头部
name:         暂作为不可变标识
```

这个方案避免了为了消除一个字段而重构整个 App 存储和路由索引体系，同时保留了后续增加“恢复默认排序”或专门重命名操作的空间。
