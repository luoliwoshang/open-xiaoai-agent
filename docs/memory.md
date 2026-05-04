# 长期记忆设计说明

这份文档只讲当前 Phase 1 记忆系统的职责边界、主流程接入方式，以及默认文件型实现的行为。

## 1. 先区分两个概念

当前仓库里有两类“记忆”，它们不是一回事：

### 1.1 会话上下文

- 指最近几分钟的滑动窗口对话历史
- 存在 MySQL 的 conversations / conversation_messages
- 主要给 intent 和 reply 看“刚刚这几轮说了什么”

### 1.2 长期记忆

- 指需要跨多轮、跨时段保留下来的用户背景信息
- 当前默认实现是本地文本文件，默认扩展名是 `.md`
- 主要给 reply 和复杂任务执行器参考

例如：

- 常用服务地址
- 个人偏好
- 固定环境说明
- 之后还可能用到的账号备注或非敏感背景信息

## 2. 抽象接口与本地实现必须分开

当前已经明确做了两层拆分：

### 2.1 抽象接口层

位置：

- `internal/memory`

这里只保留 assistant 主流程真正依赖的最小接口：

- `Recall(memoryKey)`：取回当前完整记忆文本
- `UpdateFromSession(memoryKey, history)`：在一段 session 自然结束后，把那次完整会话 history 交给实现者更新记忆

这个接口不关心：

- 记忆具体存在哪里
- 是否支持手动编辑
- 是否要记录 diff 日志
- dashboard 怎么展示

### 2.2 默认文件型实现

位置：

- `internal/memory/filememory`

这是当前 Phase 1 的具体实现者，它除了实现 `memory.Service` 之外，还额外负责：

- 管理记忆文件
- 保存 dashboard 手动编辑后的正文
- 记录记忆更新日志
- 给 dashboard 提供文件读取 / 保存 / 日志列表能力

这些“管理语义”是当前实现者自己的职责，不属于抽象接口。

## 3. 当前默认实现怎么存

### 3.1 存储目录

目录配置保存在 settings 表里：

- `memory.storage_dir`

默认值：

- `.open-xiaoai-agent/memory`

### 3.2 文件命名

每个 memory key 对应一个记忆文件。

当前主流程默认使用：

- `main-voice`

因此默认文件大致会落在：

- `.open-xiaoai-agent/memory/main-voice.md`

文件首次创建时默认是空白内容，不会自动写入说明模板。

### 3.3 更新日志

当前文件型实现会把每次更新额外写一份日志到 MySQL：

- `memory_update_logs`

日志里至少保留：

- `memory_key`
- `source`
- `messages_json`
- `before_text`
- `after_text`
- `created_at`

这里的日志只属于“当前文件型实现的变更轨迹”，不是抽象记忆接口的一部分。

## 4. 主流程怎么接入长期记忆

### 4.1 intent 不使用长期记忆

这是当前明确的产品决策。

原因很直接：

- intent 只负责判断“这轮应该走普通聊天、工具还是异步任务”
- 不应该让长期记忆干扰路由判断

所以当前 `intent.Decide(...)` 看到的仍然只是最近会话窗口 history。

### 4.2 reply 会使用长期记忆

assistant 在每轮开始时，会先按当前 `historyKey` 召回长期记忆。

然后会把这段记忆包装成一条 system message，插到 reply history 最前面，再交给：

- 普通聊天 reply
- 工具结果整理 reply
- 任务结果汇报 reply

这条 system message 还会额外约束：

- 只在相关时参考
- 不要机械复述
- 不要伪装成用户刚刚说过的话
- 没有明确需要时不要主动泄露 URL / Token / 密钥

### 4.3 会话结束后才会反向写回长期记忆

当前不再按每一轮对话高频追加长期记忆。

现在的规则是：

- assistant 仍然会把稳定问答写进短期会话窗口
- 当某个 `historyKey` 对应的 session 因为超时而自然关闭后
- 后台会拿“刚结束的那一整段 session history”调用 `UpdateFromSession(...)`
- 默认实现会把“当前记忆内容 + 本次完整会话”交给模型整理
- 模型只应该保留真正值得长期记住的信息，琐碎助手回复不应进入记忆文件
- 模型被明确约束为禁止凭空补全姓名、关系、身份等未出现事实；如果新对话和旧记忆冲突，应直接更新或删减旧记忆

### 4.4 complex_task / continue_task 也会带上长期记忆

当前主流程会把已经召回好的记忆挂进 plugin 调用上下文。

因此：

- `complex_task`
- `continue_task`

都可以拿到这份长期记忆，并继续传给底层执行器。

当前 Claude 执行器会把这段长期记忆拼进任务 prompt，但同时明确约束：

- 可以参考
- 不要在面向用户的进度汇报或最终总结里泄露内部 URL / Token / Key

## 5. Dashboard 当前提供什么调试能力

当前 dashboard 对记忆系统提供两类入口。

### 5.1 Settings 页

现在可以调整：

- `memory.storage_dir`

也就是默认文件型记忆实现的存储目录。

### 5.2 长期记忆页

现在有单独的 `长期记忆` 页面，用来：

- 查看 `main-voice` 当前记忆文件
- 手动编辑并保存记忆正文
- 查看记忆更新日志
- 对于 `session_summary` 类型的系统整理记录，可以展开看到这次总结所依据的原始对话消息
- 用 diff 风格查看某次更新前后变化

这些日志当前主要来自两种来源：

- 一次 session 自然结束后的系统整理更新
- dashboard 上的手动编辑保存

相关后端接口：

- `POST /api/settings/memory`
- `GET /api/memory/file`
- `POST /api/memory/file`
- `GET /api/memory/logs`

## 6. 当前阶段刻意不做什么

这次 Phase 1 暂时不做：

- 让 intent 直接读取长期记忆
- 做记忆召回排序、片段检索、向量检索
- 做多 memory key 管理 UI
- 把“手动编辑 / 更新日志”塞进抽象接口层

当前目标很明确：

- 先把主流程和复杂任务对长期记忆的接入打通
- 先给一个可手工维护、可调试、可观察变更的默认实现
