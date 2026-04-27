# 异步任务技术说明

这份文档专门解释 `open-xiaoai-agent` 当前的异步任务实现。

目标不是泛泛讨论“Agent 怎么做任务”，而是说明这个仓库现在到底怎么把“用户一句话派任务”落成后台执行、进度追踪、结果补报和任务续做。

## 设计目标

当前异步任务设计主要解决四件事：

1. 用户可以在正常对话里派发复杂任务，不阻塞当前对话。
2. 任务要有明确状态、进度和结果，而不是一句“我去做了”之后就丢失。
3. 任务执行器可以替换，任务总线本身不和某个具体 AI 工具强耦合。
4. 任务完成后，系统不是立即强行插话，而是在合适的后续对话时机补报结果。

## 1. 异步任务的基础模型

在这个项目里，普通工具和异步任务的分界很明确：

- 普通工具：同步执行，执行完后通常再走一次 `reply` 模型整理口语化回复。
- 异步任务：工具先返回一个“已受理”的短回执，真正执行在后台继续进行。

异步任务的入口不是独立子系统，而是插件系统的一种返回模式。

当某个工具返回：

- `OutputMode = async_accept`
- 并附带 `plugin.AsyncTask`

assistant 就会把它视为异步任务受理，而不是普通同步工具结果。

## 2. 通用任务数据模型

当前任务主字段包括：

- `id`
- `plugin`
- `kind`
- `title`
- `input`
- `parent_task_id`
- `state`
- `summary`
- `result`
- `report_pending`
- `created_at`
- `updated_at`

当前状态只有五种：

- `accepted`
- `running`
- `completed`
- `failed`
- `canceled`

这里有两个关键约束：

1. 主任务表只保存通用任务信息，不保存执行器私有状态。
2. `parent_task_id` 用来表达“这次任务是在上一次任务基础上继续做”的关系，而不是覆盖原任务。

## 3. 持久化与事件流

当前任务系统是 JSON 文件持久化，不依赖数据库。

通用任务状态保存在本地任务状态 JSON 文件里。

其中包含两类数据：

- `tasks`
- `events`

`tasks` 负责保存任务当前快照，`events` 负责保存任务过程事件，比如：

- `accepted`
- `running`
- `progress`
- `completed`
- `failed`
- `canceled`
- 以及执行器自定义事件，例如 `claude_init`

当前保存方式是：

- 先写临时文件
- 再用 rename 替换

也就是说当前实现已经至少保证了基础的原子替换，而不是直接覆盖写主文件。

## 4. 一个异步任务是怎么跑起来的

完整链路如下：

1. `intent` 识别出应该调用异步任务工具，比如 `complex_task`。
2. 插件返回 `plugin.Result`，其中带有：
   - 一句短回执文本
   - `OutputModeAsyncAccept`
   - `plugin.AsyncTask`
3. `assistant.handleAsyncTask()` 调用 `tasks.Manager.Submit()`。
4. `Submit()` 先写入一条状态为 `accepted` 的任务记录和一条 `accepted` 事件。
5. assistant 立刻向用户播报“收到，这个任务我先去处理”。
6. 后台 goroutine 再真正执行 `AsyncTask.Run(...)`。

这就是“不阻塞当前对话”的核心：  
前台先完成受理和回执，后台再继续慢慢干活。

## 5. 后台执行期间怎么上报进度

`tasks.Manager` 在启动后台执行时，会给任务运行函数注入一个 `plugin.AsyncReporter`。

这个 reporter 目前提供三种能力：

- `TaskID()`
- `Update(summary string)`
- `Event(eventType, message string)`

含义分别是：

- `TaskID()`：告诉执行器当前这次任务在主任务表里的任务 ID
- `Update()`：更新对用户最重要的阶段摘要，同时追加一条 `progress` 事件
- `Event()`：记录更细粒度的内部事件，不一定直接面向用户播报

当前 `summary` 是任务进度查询和任务补报时最重要的一段信息，所以设计上把它放在任务主记录里，而不是只存在事件流中。

## 6. 任务完成、失败、取消时为什么不立刻播报

这是当前异步任务设计里很重要的一个产品选择。

任务完成、失败或取消后，系统不会主动立即开启一轮新的回复去打断用户，而是：

1. 更新任务最终状态
2. 设置 `report_pending = true`
3. 等用户下一次正常进入对话流程时，再顺带补报

assistant 在每次正常回复、工具回复或异步任务受理播报结束后，都会调用 `deliverPendingReports()`：

- 从任务系统里取出待补报项目
- 组织成结构化上下文
- 再走一次 `reply.StreamPendingTaskNotice(...)`
- 把补报润色成自然口语
- 播放成功后，再调用 `MarkReported()` 把对应任务的 `report_pending` 清掉

这意味着：

- 补报内容会尽量自然，不是硬编码模板直出
- 只有真正播报成功后，任务才算“已通知”
- 补报语句会进入会话历史，供后续 `intent` / `reply` 继续参考

## 7. 查询进度和取消任务

当前系统已经有两类基础操作：

### 7.1 查询进度

`query_task_progress` 最终调用 `Manager.SummarizeProgress(3)`。

它不会只返回模糊摘要，而是尽量把这三件事说清楚：

- 任务标题
- 真实状态
- 当前阶段 summary

这样做的原因是：有些 summary 看起来像完成了，但真实状态可能还在 `running`。

### 7.2 取消任务

`cancel_task` 当前调用 `Manager.CancelLatest()`，取消最近一个仍处于：

- `accepted`
- `running`

状态的任务。

取消动作会：

- 更新任务状态为 `canceled`
- 写入取消事件
- 触发对应 `context.CancelFunc`
- 把 `report_pending` 设为 `true`

也就是说，“取消”本身也会被作为一个待补报事件，在后续对话里告诉用户。

## 8. 任务续做为什么是插件自管

当前续做能力通过 `continue_task` 实现，但它不是一个全局无差别的“继续执行器”。

设计上，续做归插件自己负责，原因是：

- 不同执行器恢复上下文的方式不一样
- 不同执行器需要保存的私有状态也不一样
- 主任务表应该保持通用，不应该混入某个执行器的私有字段

所以当前流程是：

1. `continue_task` 接收 `plugin_name`、`task_id`、`request`
2. 从主任务表里找到原任务
3. 校验这个任务确实属于指定插件
4. 创建一条新的异步任务记录
5. 通过 `ResumeRegistry` 把恢复动作分发给对应插件

这里新的任务会：

- 保留新的任务 ID
- 设置 `parent_task_id = 原任务 ID`
- 不覆盖旧任务

这让“补充要求”“继续做”“在旧结果上追加修改”都有明确链路可追踪。

## 9. 为什么要把通用任务状态和 Claude 私有状态分开

当前异步任务系统故意拆成两层存储：

### 9.1 通用层

- 归属：通用任务层
- 内容：通用任务字段、通用事件流、对所有执行器都成立的状态

### 9.2 Claude 私有层

- 归属：Claude 执行器私有层
- 内容：Claude 会话恢复所需的私有数据

这样拆的好处是：

- 主任务表保持通用
- 将来接入 Codex、OpenClaw、OpenCode 时不需要污染主任务模型
- 每个执行器都可以只管理自己的恢复状态和运行细节

## 10. Claude Code 目前是怎么接进来的

当前 `Claude Code` 是通过 `complex_task` 这条插件链路接入的。

启动时的装配逻辑大致是：

1. 创建通用任务管理器 `tasks.NewManager(...)`
2. 创建 Claude 私有状态存储 `complextask.NewStore(...)`
3. 创建执行器 `complextask.NewClaudeRunner(...)`
4. 用 store + runner 组装 `complextask.Service`
5. 把 `complex_task` 注册到插件系统
6. 把 `complex_task` 注册到 `ResumeRegistry`，供 `continue_task` 使用

也就是说：

- `complex_task` 负责“受理复杂任务”
- `ClaudeRunner` 负责“真正调用 claude CLI 干活”
- `continue_task` 负责“找到要续做的旧任务，并交回原插件恢复”

## 11. Claude Code 对应的任务受理逻辑

复杂任务插件会把一个用户请求转换成异步任务规格。

当 `complex_task` 命中后，它会返回：

- 对用户的短回执：
  - `收到，这个任务我先去处理，Claude 在后台开始干活了。`
- 一个 `plugin.AsyncTask`

这个 `AsyncTask` 里最重要的字段是：

- `Plugin = "complex_task"`
- `Kind = "complex_task"`
- `Title = summarizeTitle(request)`
- `Input = 原始 request`
- `Run = service.runner.Run(...)`

也就是说，Claude Code 当前不是直接写进 assistant 主流程里的，而是被包在通用异步任务接口后面。

## 12. Claude Runner 如何启动新任务

当前新任务命令形态是：

```sh
claude --dangerously-skip-permissions --print --output-format stream-json --verbose "<task>"
```

在代码里，真正传给 Claude 的不是原始用户句子，而是经过 prompt 包装后的任务说明，当前重点约束是：

- 持续汇报阶段性进度
- 进度要简短
- 不要使用影响 TTS 的 Markdown 噪音或特殊符号
- 任务未完成时不要提前说完成
- 最终总结尽量说明：
  - 完成了什么
  - 产出放在哪里
  - 用户接下来怎么用

这些约束的本质是：  
Claude 不只是要“做出来”，还要“能被语音系统稳定播报和持续追踪”。

## 13. Claude Runner 如何处理流式输出

`ClaudeRunner` 通过 `stdout pipe + scanner` 逐行读取 `stream-json` 输出。

当前处理三类消息：

### 13.1 `system/init`

如果拿到 `session_id`：

- 写入 Claude 私有状态文件
- 记录一条 `claude_init` 事件

### 13.2 `assistant`

如果拿到 Claude 的中间文本：

- 先做清洗
- 提取一句尽量适合播报和展示的阶段 summary
- 写入 Claude 私有状态里的 `LastSummary` / `LastAssistantText`
- 再通过 `reporter.Update(...)` 回写到通用任务表

这样 dashboard 和任务查询接口看到的是统一的任务进度，而不是执行器专属格式。

### 13.3 `result`

如果拿到最终结果：

- 保存为最终 `result`
- 返回给任务管理器
- 由任务管理器把主任务状态切成 `completed`

如果 `result` 带错误，或命令执行失败，则任务会转成 `failed`。

## 14. Claude 的私有状态里保存了什么

当前 Claude 的私有状态主要保存：

- `task_id`
- `session_id`
- `prompt`
- `working_directory`
- `status`
- `last_summary`
- `last_assistant_text`
- `result`
- `error`

最关键的是 `session_id`。  
因为后续续做任务时，恢复 Claude 上下文靠的就是它，而不是靠主任务表。

## 15. Claude Code 如何续做同一个任务

当前续做命令形态是：

```sh
claude --dangerously-skip-permissions --resume "<session_id>" --print --output-format stream-json --verbose "<follow-up request>"
```

具体流程是：

1. 用户说“刚刚那个网页再炫酷一点”之类的话
2. `intent` 选择 `continue_task`
3. `continue_task` 找到一个已完成的旧任务
4. 根据旧任务的 `plugin` 找到对应 resumer
5. `complex_task.Service.ResumeTask(...)` 调用 `ClaudeRunner.Resume(...)`
6. `ClaudeRunner` 从 Claude 私有状态中取出旧任务 `session_id`
7. 创建一条新的异步任务记录，并把 `parent_task_id` 指回原任务
8. 用 `claude --resume` 在同一 Claude 会话基础上继续执行

这里的重点不是“重新做一遍”，而是“在之前那个执行上下文里接着做”。

## 16. 当前实现的边界

当前异步任务系统已经能支撑：

- 受理复杂任务
- 后台执行
- 追踪进度
- 查询进展
- 取消最近活跃任务
- 在已完成任务基础上继续做
- 在后续对话里自然补报任务结果

但目前仍有几个明确边界：

- 任务补报不是主动抢占式插话，而是挂到后续对话时机里再说
- 取消任务当前是“取消最近一个活跃任务”，不是任意精确选择
- `continue_task` 当前面向“已完成任务续做”，不是任意运行中任务热接管
- Claude 目前是第一种执行器实现，不代表通用异步任务模型只能服务 Claude

## 17. 后续接入其他执行器时应该遵守什么边界

如果后面接入 Codex、OpenClaw、OpenCode，建议继续遵守当前边界：

1. 主任务表只保存通用任务信息，不保存执行器私有恢复状态。
2. 每个执行器都在自己的插件目录管理私有状态。
3. 统一通过 `plugin.AsyncTask` + `AsyncReporter` 接到通用任务总线上。
4. 任务续做通过插件自管恢复，不把恢复逻辑硬编码进全局任务层。

这样扩展时，新增的只是“一个新的执行器插件”，而不是重写整个异步任务系统。
