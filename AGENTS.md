# AGENTS.md

这份文件用于记录 `open-xiaoai-agent` 当前实际可用的开发上下文，这样以后在其他工作区继续开发时，不需要重新摸索一遍项目现状。

## 项目标识

- 仓库：`https://github.com/luoliwoshang/open-xiaoai-agent`
- 上游生态参考：[`open-xiaoai`](https://github.com/idootop/open-xiaoai)
- 这个项目是 `open-xiaoai` 设备 / client 流程背后的独立服务端。
- 它不应该再依赖 `open-xiaoai` 仓库的源码树。
- 当前 Go module 路径：
  - `github.com/luoliwoshang/open-xiaoai-agent`

## 这个项目是做什么的

这是一个挂在 `open-xiaoai` client 后面的独立 Agent Server 原型。

当前职责：

- 通过 WebSocket 接收 XiaoAI Rust client 转发过来的最终 ASR 文本
- 接收由 Dashboard 手动注入、用于调试 assistant 主流程的识别文本
- 在 ASR 之后按需打断原始 XiaoAI 流程
- 执行 `intent` 路由，分流到工具 / 异步任务 / 任务继续等路径
- 生成普通对话回复，以及工具结果总结回复
- 通过现有 client 协议驱动设备侧本地 TTS 播放
- 提供主流程长期记忆，并让 intent / reply / complex_task 复用它
- 维护轻量异步任务系统
- 提供一期 IM 网关能力：支持微信文本投递，以及默认渠道的图片 / 文件调试发送
- 持久化后端运行日志，并通过 dashboard API 暴露出来
- 通过 API + 前端工作区提供一个 React 调试 Dashboard

当前非目标 / 已知缺失项：

- 还没有 IM 入站会话处理
- 还没有文本之外的自动 IM 媒体镜像
- 还没有 IM 网关的群路由
- 还没有真正完善的语音打断检测
- 之前一些关于延迟优化的实验性实现有意没有保留
- 持久化目前使用的是 MySQL，不是 Redis / MQ

## 技术栈

- Go `1.24.0`
- React `19.x`
- Vite `6.x`
- TypeScript `5.8.x`
- Node.js / npm 用于 web dashboard 和并发开发命令

当前 `go.mod` 里声明的 Go 依赖：

- `github.com/gorilla/websocket v1.5.3`
- `gopkg.in/yaml.v3 v3.0.1`

`package.json` 中的前端 / 运行时工具说明：

- 根 workspace 名称：`open-xiaoai-agent`
- 使用 `concurrently` 同时跑 Go 和前端
- 当前有效的前端工作区位于 `frontend/`
- 旧的 `web/` 仍然保留在仓库中用于过渡，但新的 UI 工作应当落在 `frontend/`

重要说明：

- 前端是 `React + Vite`
- 不是 `Vue`

## 仓库结构

这里只列高信号目录与文件。

- `main.go`
  - 应用入口
  - 负责 wiring：config、tasks、plugins、Claude runner、dashboard、assistant
- `internal/assistant`
  - 主编排流程
  - 会话历史
  - speculative reply 处理
  - 任务结果汇报投递
- `internal/llm`
  - OpenAI-compatible client
  - intent 识别器
  - reply 生成器
- `internal/plugin`
  - tool / async-task 注册中心
- `internal/memory`
  - assistant 依赖的最小记忆抽象接口
- `internal/memory/filememory`
  - 当前默认文件型记忆实现
  - 记忆文件管理与更新日志
- `internal/plugins`
  - 实际内置工具
  - 每个 plugin 都在自己的子目录里
- `internal/tasks`
  - 基于 MySQL 的任务存储与管理器
- `internal/server`
  - WebSocket server / session / RPC 协议
- `internal/voice`
  - assistant 使用的通用语音通道抽象
  - 分块播报的 stream speaker helper
- `internal/voice/xiaoai`
  - 当前 XiaoAI 语音适配实现
- `internal/dashboard`
  - dashboard 状态与接口的 API 侧实现
- `internal/logs`
  - 后端运行日志持久化
  - 标准 logger 捕获
  - 分页日志读取
- `internal/im`
  - 一期 IM 网关
  - 微信登录 / 账号 / 目标 / 镜像投递
- `frontend/`
  - 当前 React dashboard 工作区
- `web/`
  - 旧 dashboard 工作区，暂时保留用于迁移过渡

## 运行命令

同时启动后端和前端：

```sh
npm install
npm run dev
```

根级脚本：

- `npm run dev`
- `npm run dev:go`
- `npm run dev:fe`
- `npm run build:fe`
- `npm run dev:web`
- `npm run build:web`
- `npm run preview:web`

只直接跑后端：

```sh
go run .
```

`main.go` 当前常用后端参数：

- `-addr`
  - 默认 `:4399`
- `-dashboard-addr`
  - 默认 `:8090`
- `-claude-cwd`
  - Claude CLI 任务的工作目录
- `-debug`
- `-abort-after-asr`
  - 默认 `true`
- `-post-abort-delay`
  - 默认 `1s`
- `-parallel-intent-chat`
  - 默认 `true`

## 配置文件

- `SOUL.md`
  - 常见的人设文件命名约定之一
  - 当前不会自动读取它，只有 `config.yaml` 里的 `soul_path` 显式指向它时才会加载
  - 这类人设文件按当前约定应保留在本地，不再提交到仓库
- `config.example.yaml`
  - 提交到仓库中的示例配置
- `config.yaml`
  - 本地真实配置
  - 已被 git ignore，因为它可能包含密钥

当前使用到的配置域：

- `soul_path`
- `database.dsn`
- `task.artifact_cache_dir`
- `openai.base_url`
- `intent.model / base_url / api_key`
- `reply.model / base_url / api_key`
- `amap.api_key`
- `im.media_cache_dir`

重要说明：

- `config.yaml` 故意被忽略
- `SOUL.md` 这类本地人设文件也应忽略，不要提交
- 不要提交真实 API key
- `soul_path` 是必填项，支持相对路径和绝对路径；相对路径按仓库根目录解析
- 运行时数据库配置只从 `config.yaml` 的 `database.dsn` 读取

## 持久化状态

当前项目使用 MySQL 做持久化。

逻辑上的存储分类：

- 通用异步任务记录 + 任务事件
- 任务 artifact 元数据
- 滑动窗口会话历史
- Claude plugin 私有状态
- 运行时设置，例如 `session.window_seconds`
- 后端运行日志
- IM 网关账号 / 目标 / 事件
- 长期记忆更新日志

### 各类存储的含义

通用任务存储：

- 通用任务表
- 通用任务事件
- 任务 artifact 表
- 不存执行器私有内部状态，例如 Claude session id

任务 artifact 缓存：

- 任务产出的文件缓存于本地磁盘
- 缓存目录来自 `config.yaml` 的 `task.artifact_cache_dir`
- 当前默认值是仓库根目录下的 `.cache/task-artifacts`
- plugin 对任务系统上报的是 artifact 内容和元数据，不会把裸本地路径跨边界传递

会话存储：

- 持久化的会话窗口，供 `intent` 和 `reply` 复用
- 当前主语音流程使用统一的会话键 `main-voice`
- 真实 XiaoAI ASR 和 dashboard 调试 ASR 都写进这同一段主语音会话
- 会话窗口仍然按 `last_active + session.window_seconds` 过期

运行时设置存储：

- 存在 MySQL 中的小型 key/value 运行时设置
- 当前主要用于：
  - `session.window_seconds`
  - `im.delivery.enabled`
  - `im.delivery.selected_account_id`
  - `im.delivery.selected_target_id`
  - `memory.storage_dir`
- 服务启动时应确保默认设置行存在

长期记忆存储：

- 当前默认实现是本地 Markdown 文件
- 文件目录来自 settings 表里的 `memory.storage_dir`
- 当前主流程默认 memory key 是 `main-voice`
- dashboard 的手动编辑与 diff 日志查看能力属于 `internal/memory/filememory` 这个实现者，不属于抽象接口

IM 网关存储：

- 一期微信网关状态持久化在 MySQL
- 内容包括：
  - 已登录 IM 账号
  - 默认文本投递目标
  - 最近 IM 网关事件
- 当前范围是文本投递，以及默认渠道的图片 / 文件调试发送

媒体缓存：

- 上传到 IM 调试发送的图片 / 文件，在适配层投递前会先缓存到本地磁盘
- 缓存目录来自 `config.yaml` 的 `im.media_cache_dir`
- 当前默认值是仓库根目录下的 `.cache/im-media`
- 这些文件目前不会自动清理

运行日志存储：

- 后端标准 logger 输出会被持久化到 MySQL
- 每条日志行会保留：
  - 时间戳
  - 推断出的等级
  - 文件 / 行号（如果能拿到）
  - 解析后的 message
  - 原始格式化日志行
- dashboard 通过专门的分页 API 读取这些日志

Claude 私有存储：

- plugin 私有存储
- 用来把项目任务 id 映射到 Claude 私有状态，例如：
  - Claude `session_id`
  - prompt
  - last summary
  - last assistant text
  - result

这个拆分是刻意设计的：

- 主任务存储保持通用
- plugin 特有的 continuation 状态留在 plugin 内部

当前 dashboard API 还提供了一个会清空运行时状态的 reset 接口：

- `POST /api/reset`

## 会话模型

会话历史会被持久化并复用。

当前规则：

- 会话复用基于滑动窗口
- 共享的主语音会话只要 `last_active + session.window_seconds` 没过期就保持活跃
- 当前默认值是 `300` 秒，并且可以通过 dashboard settings API 调整

会写入会话历史的内容：

- 用户输入
- assistant 回复
- 异步任务通知在真正播报成功之后的文本

这意味着：

- 未来的 `intent` 调用能看到最近历史
- 未来的 `reply` 调用能看到最近历史
- 任务结果汇报在播报成功后也会进入历史

## 设备 / XiaoAI 流程

当前假定的流程：

1. 用户唤醒原始 XiaoAI
2. 原始 XiaoAI 完成 ASR
3. Rust client 转发 `SpeechRecognizer.RecognizeResult`
4. 后端把 XiaoAI 连接包装成 voice-channel adapter
5. 后端在播放准备阶段按需打断原生语音流程
6. 后端执行 routing + reply/tool/task 逻辑
7. TTS 通过选定的语音通道回放

当前语音轮次调度规则：

- 同一时刻只允许一条会发声的 assistant 流程运行
- 如果上一条语音轮次还没结束时来了新的 ASR，新 ASR 会被忽略
- 异步任务结果汇报与普通主流程共用同一条语音通道，只会在通道空闲时播放

这个项目目前仍然本质复用：

- XiaoAI 唤醒 + ASR 链路
- 当前 XiaoAI / open-xiaoai 语音适配链路

关于语音抽象的重要说明：

- assistant 主流程现在面向的是通用 voice channel，不再直接依赖 `*server.Session`
- `HandleUserText(historyKey, channel, text)` 是 assistant 内部主文本入口
- XiaoAI 专属的播放细节，例如 daemon restart 和 `tts_play.sh`，都放在 `internal/voice/xiaoai`

## Intent / Reply 行为

Intent 阶段：

- 非流式
- 感知工具定义
- 当前只接受模型返回的原生 tool call，不再兼容 JSON fallback
- 能看到最近会话历史
- 会读取当前 historyKey 对应的长期记忆，并以 system message 形式参与路由判断
- 能看到最近可继续的任务链摘要，最新节点可能是 `accepted / running / completed`，用于 `continue_task` 风格的路由
- 普通聊天、澄清输入这类“不需要外部动作”的路径，也会先命中 `continue_chat` 这个路由工具，再回到 reply 主线

Reply 阶段：

- 流式
- 用于普通聊天
- 用于整理普通工具输出
- 用于任务结果汇报
- 会读取当前 historyKey 对应的长期记忆

长期记忆行为：

- intent 会把召回的长期记忆作为 system message 拼进上下文
- reply 会把召回的长期记忆作为 system message 拼进上下文
- 短期会话窗口超时结束后，会把那次完整 history 低频整理进长期记忆
- `complex_task` / `continue_task` 会把这份长期记忆继续传给执行器

Speculative 行为：

- `intent` 和 `reply` 可以并行执行
- 如果最终没有选中工具，就会复用 speculative reply 的结果

## 内置 Plugins

Plugins 通过 `internal/plugins/register.go` 注册。

当前内置工具包括：

- `continue_chat`
- `ask_weather`
- `list_tools`
- `complex_task`
- `query_task_progress`
- `cancel_task`
- `continue_task`

每个 plugin 都应该保持在自己的子目录里。这是一个明确做过的设计决策。

### 工具输出模式

plugin 系统支持不同的输出模式。当前最重要的实际区分是：

- direct output
- 交给 reply model 整理后输出
- async task acceptance

对普通工具的默认预期：

- 工具结果通常会再经过 reply model 整理，而不是原样直接播报

## 异步任务模型

异步任务系统刻意保持得比较小。

当前任务状态：

- `accepted`
- `running`
- `completed`
- `failed`
- `canceled`
- `superseded`

任务记录包含的通用字段有：

- `id`
- `plugin`
- `kind`
- `title`
- `input`
- `parent_task_id`
- `state`
- `summary`
- `result`
- `result_report_pending`
- 时间戳

任务 artifact 故意与任务主行分开：

- artifact 归属于当前 `task_id`
- 当前阶段不构建 parent/root task 的 artifact 聚合视图
- 任务一旦进入 `completed`，就不应再接受新的 artifact
- plugin 可以标记哪些 artifact 以后是要投递的，但暂时不引入 delivery-status tracking

重要行为决策：

- 异步任务完成后会设置 `result_report_pending=true`
- 当语音通道空闲且 assistant 仍然持有可用 session 时，任务结果汇报可以被主动播报
- 如果 assistant 当前正在处理另一轮语音主流程，任务结果汇报会保持 waiting-to-report，等当前轮次结束后再重试
- 任务结果汇报由 reply model 基于结构化任务数据生成
- 系统不应该机械地照读诸如“xxx 已经完成了”这种标题

### 查询任务进度

这个区域经过多轮迭代。

当前预期：

- 任务进度应该至少包含：
  - 标题
  - 真实任务状态
  - 当前 summary
- 而不只是 summary 本身

这样改是因为有些 summary 会“听起来像做完了”，但实际任务状态还没切到 completed。

## Claude Code 异步执行器

当前 `complex_task` 使用 Claude Code CLI 作为异步执行器之一。

典型命令形式：

```sh
claude --dangerously-skip-permissions --print --output-format stream-json --verbose "<task>"
```

继续执行的形式：

```sh
claude --dangerously-skip-permissions --resume "<session_id>" --print --output-format stream-json --verbose "<follow-up request>"
```

### Claude Runner 行为

已实现行为：

- 按行解析 stream-json
- 从 `system/init` 中提取 `session_id`
- 捕获 assistant 文本
- 捕获最终结果
- Claude 成功退出后，可以从 manifest 索引文件导入任务 artifacts
- Claude 私有状态存入 Claude-private MySQL store

当前对新 Claude 任务的 prompt shaping 规则：

- 前缀固定加上“执行以下任务：...”
- 要求给出简短的进度更新
- 避免进度里出现奇怪符号或 markdown 噪音
- 面向用户的进度 / 最终摘要不应出现 workdir 路径、manifest 路径、终端命令以及其他执行器内部细节
- 如果 Claude 产出了要交付给系统的文件，这些文件必须统一放在 `.open-xiaoai-agent/deliverables/<task_id>/` 下
- 面向用户的最终摘要也不应直接说“保存为 xxx.png / xxx.html / xxx.txt”
- 面向用户的措辞应保持朴素、普通用户可理解，不要过度专业或冗长
- 如果 Claude 需要把文件交还给系统，应在 `.open-xiaoai-agent/artifacts/<task_id>.json` 下写一个 manifest 索引文件
- 最终摘要仍应保持简洁且适合 TTS

Claude artifact handoff 规则：

- manifest 文件本身只是一个可交付文件位置与元数据的索引
- manifest 里声明的 path 只能引用 `.open-xiaoai-agent/deliverables/<task_id>/` 下的文件
- Claude adapter 会读取这些本地文件，并调用通用任务 artifact API
- 原始本地路径不会跨过任务系统边界

### Resume / Continue Task

任务 continuation 是 plugin 自己负责的，而不是全局统一实现。

含义是：

- 主任务表只保存 `plugin` 和通用任务信息
- 当继续一个任务时：
  - 路由先找到一条可继续任务链的最新节点
  - 从任务记录里识别 plugin
  - 由 plugin 读取自己的私有状态
  - plugin 再取出自己的 Claude `session_id`
  - 然后继续执行

这点非常重要：

- `session_id` 不会存到主任务表
- plugin 私有执行状态不能在 plugins 之间共享

当前 continuation 行为：

- 继续任务时会新建一条任务行
- 用 `parent_task_id` 把它关联回原任务
- 不会覆盖旧任务
- 如果继续的是一条仍在 `accepted / running` 的任务链，旧任务会先被中断，再标记成 `superseded`

## 天气集成

天气能力接的是高德 / AMap。

重要细节：

- 对外公开的工具输入仍然是人类城市名
- weather plugin 内部会把城市解析成 `adcode`
- 当前生成的 city/adcode 映射来自提供的 AMap Excel 文件
- 生成后的映射位于：
  - `internal/plugins/weather/adcodes_gen.go`

## Dashboard / Frontend

Go 只提供 dashboard API。

Dashboard 的定位必须保持明确：

- Dashboard 是调试与排障控制台
- 它不是面向最终用户的日常操作台
- 它的核心职责是运行态观察、手动调试注入、任务排查、产物 / 触达验证，以及日志辅助诊断

重要路由：

- `GET /api/healthz`
- `GET /api/logs`
- `GET /api/state`
- `GET /api/xiaoai/status`
- `POST /api/assistant/asr`
- `GET /api/tasks/{taskID}/artifacts/{artifactID}/download`
- `GET /api/settings`
- `POST /api/settings/session`
- `POST /api/settings/memory`
- `GET /api/memory/file`
- `POST /api/memory/file`
- `GET /api/memory/logs`
- `POST /api/settings/im-delivery`
- `POST /api/im/wechat/login/start`
- `GET /api/im/wechat/login/status`
- `POST /api/im/wechat/login/confirm`
- `POST /api/im/debug/send-default`
- `POST /api/im/debug/send-image-default`
- `POST /api/im/debug/send-file-default`
- `POST /api/im/targets`
- `POST /api/im/targets/default`
- `POST /api/im/targets/delete`
- `POST /api/im/accounts/delete`
- `POST /api/reset`

重要行为：

- `POST /api/assistant/asr` 是服务端侧的 debug ASR 注入入口
- 它不依赖实时 XiaoAI 设备连接
- 它与真实 XiaoAI 入口共享同一段 `main-voice` 会话上下文
- 它使用一条专用 debug voice channel，但仍遵守 assistant 的 busy gate

前端应保持独立，不要和后端重新耦合。

明确提出过的 UI 决策：

- 会话历史不应在视觉上和任务事件流混在一起
- 任务事件流属于被选中的某个任务
- 会话历史应单独显示
- settings 应放在单独页面
- 后端日志应放在独立页面，不应混入 `/api/state`
- dashboard 应该有设计感，不能像通用后台模板
- dashboard 的文案和信息架构也应强化“调试控制台”定位，而不是做成泛化的管理后台
- dashboard 状态应暴露 assistant 语音通道运行时状态，例如 busy / result-report-ready / has-voice-channel
- dashboard 首页可以提供一个手动 ASR-debug 输入入口，把识别文本注入当前 assistant 流程，使用共享的 `main-voice` 会话上下文和专用 debug voice channel
- dashboard 还可以暴露当前 XiaoAI websocket 连接状态，方便操作者快速确认设备桥是否在线
- dashboard 可以提供单独的长期记忆页面，用来查看 `main-voice` 记忆文件、手动编辑正文和查看更新日志 diff

## 前端 UI 风格

修改 XiaoAiAgent 前端页面时，必须遵守项目 UI 风格指南：

- 参见 `docs/frontend-ui-style-guide.md`
- 保持可爱、明亮、现代 SaaS dashboard 风格
- 保持蓝色 / 紫色 / 薄荷绿的主色方向
- 使用圆角卡片、柔和阴影、可读的中文 UI，以及轻量 mascot 风格装饰
- 除非用户明确要求重新设计，不要引入无关视觉风格

## 测试

推荐的 Go 校验方式：

```sh
GOCACHE=$(pwd)/.gocache go test ./...
```

前端校验：

```sh
npm run build:fe
```

说明：

- `.gocache/` 是刻意忽略的
- 之前有些环境在本地 `httptest` 绑定上出过问题，但当前仓库在迁移后已经验证通过

## 迁移历史

这个项目迁移自：

- `open-xiaoai/examples/go-instruction-server`

它现在是一个独立仓库，应在这里继续开发，而不是回旧的 example 目录。

## 当前 README 方向

README 面向公开读者。

应当聚焦于：

- 这个项目是什么
- 怎么运行
- 它依赖什么
- 当前支持什么
- 未来的高层规划

不要把太重的内部目录解释堆进 README。

## 规划方向

目前已经讨论过的主要架构方向：

- 继续扩展独立的 `IM Gateway`
- 支持微信 / QQ 等渠道
- 让 IM 集成保持在 OpenClaw 之外
- 把 OpenClaw / Claude / 未来执行器都视为可插拔 worker，而不是 channel adapter

理想边界：

- `open-xiaoai` = 设备 / client bridge
- 本仓库 = 服务端编排
- IM gateway = 渠道路由桥
- OpenClaw / Claude = 异步执行器

## 给未来 Agent 的实践备注

- 不要再把这个仓库重新耦合回 `open-xiaoai` 源码树。
- 不要把执行器私有状态存进通用任务表。
- 不要提交 `config.yaml` 或真实数据库凭据。
- 如果有重要架构决策变化，优先更新 `AGENTS.md`。
- 如果某次开发修改了产品行为、面向用户的能力、交互流程或交付能力，必须在同一个改动中同步更新 `docs/` 下相关文档。
- 保持 plugin 代码按 plugin 目录隔离。
- README 保持面向用户；重的开发上下文放在这里。
- 流程图、架构图、时序图等示意图一律使用 Mermaid 语法，不要使用 ASCII art 或静态图片。README 和 docs 中的 mermaid 代码块会被 GitHub 原生渲染。
