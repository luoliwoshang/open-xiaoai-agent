# Open XiaoAI Agent

一个最小但可继续扩展的独立对话 Server，用来接收小爱音箱 Rust Client 发来的最终 ASR 文本，并按两阶段处理：

- `intent` 模型：非流式，优先识别是否命中本地工具
- `reply` 模型：流式，边收增量边按句切段，再调用音箱本地 TTS 播放
- `tasks`：本地 JSON 任务表，承接最小异步任务能力
- `dashboard api`：Go 只提供 `/api/*` 路由
- `web`：React/Vite 前端看板，单独启动

当前完整链路是：

1. 你先说“`小爱同学`”
2. 原生小爱完成唤醒和 ASR
3. Rust Client 监听 `/tmp/mico_aivs_lab/instruction.log`
4. Go Server 收到 `SpeechRecognizer.RecognizeResult`
5. 立即 `abort` 原生小爱
6. 调 `intent` 模型识别是否命中本地工具
7. 如果命中普通工具，执行工具闭包，再交给 `reply` 模型润色后播报
8. 如果命中异步工具，立即返回短回执，并把任务放到后台执行
9. 如果不命中工具，直接调 `reply` 模型流式生成回复
10. 按增量切成适合播报的短句，顺序调用音箱本地 TTS

另外，当前 server 会把**同一音箱连接内首轮对话开始后的 5 分钟**视为一个会话窗口。

- 会话窗口内，后续 `intent` 和 `reply` 请求都会自动带上之前的用户/助手上下文
- 超过 5 分钟后，会自动开启一个新的会话，不再携带旧上下文

## 结构

当前工程只保留几层必要结构：

- `main.go`
  负责启动参数和装配依赖
- `package.json`
  负责同时拉起 Go API 和 React 看板
- `internal/plugins`
  负责插件聚合和内置插件目录拆分；当前天气、股票、能力列表、异步任务类工具都各自独立成包
- `internal/assistant`
  负责主流程编排：`ASR -> intent -> abort -> tool/reply -> speaker`
- `internal/llm`
  负责 OpenAI 兼容协议调用，以及带 tool definitions 的意图识别
- `internal/plugin`
  负责本地工具注册、工具描述导出和命中后的闭包执行
- `internal/tasks`
  负责本地 JSON 任务表、异步任务状态和事件
- `internal/dashboard`
  负责本地任务 API
- `internal/server`
  负责 WebSocket 接入、连接会话、RPC 调用、`abort` 能力
- `internal/speaker`
  负责复用音箱本地 TTS，播放单段文字，以及把流式增量切成可播报短句
- `internal/instruction`
  负责从 `instruction` 事件里提取最终 ASR 文本
- `internal/config`
  负责读取根目录里的 `config.yaml` 和 `SOUL.md`

这样拆分后：

- 入口文件不再堆业务判断
- OpenAI 协议细节不会污染主流程
- 工具注册和工具执行不会污染意图层
- `abort`、单段播放、流式切句播放都是独立能力
- `SOUL.md` 和模型配置都在根目录显式管理
- 后面你要替换意图模型、回复模型或业务规则，不需要回头拆主入口

## 配置

根目录现在有两个配置文件：

- `SOUL.md`
  放人设文本，启动时会直接读成字符串
- `config.yaml`
  放模型和 OpenAI 兼容接口配置
- `config.example.yaml`
  配置样例；首次使用可以复制为 `config.yaml`

当前 `config.yaml` 支持这些字段：

```yaml
openai:
  base_url: https://api.openai.com/v1

amap:
  api_key: ""

intent:
  model: gpt-4.1-mini
  base_url: https://api.openai.com/v1
  api_key: sk-intent-placeholder

reply:
  model: gpt-4.1
  base_url: https://api.openai.com/v1
  api_key: sk-reply-placeholder
```

其中：

- `intent`
  非流式模型，负责工具识别；如果没有命中工具，主流程仍然继续走外部回复，并带上当前会话上下文
- `reply`
  流式模型，负责主回复生成，也负责把普通工具结果整理成自然口语回复
- `amap.api_key`
  高德天气服务 API Key；当前 `ask_weather` 会先把城市名解析成 adcode，再通过高德天气接口查询实时天气
- `openai.base_url`
  通用 OpenAI 协议基址；如果 `intent.base_url` 或 `reply.base_url` 为空，会自动回落到这里

## 运行

先安装 Node 依赖：

```sh
cd /Users/zhangzhiyang/Documents/Code/open-xiaoai-agent
npm install
```

然后一条命令同时启动 Go 和 React：

```sh
npm run dev
```

默认监听：

- WebSocket：`http://127.0.0.1:4399`
- Dashboard API：`http://127.0.0.1:8090`
- React 看板：`http://127.0.0.1:5173`
- 任务文件：`data/tasks.json`

也可以自定义：

如果你只想单独跑后端：

```sh
go run . -addr :4399 -dashboard-addr :8090 -tasks-file data/tasks.json -debug
```

如果你只想单独跑前端：

```sh
npm run dev:web
```

默认会开启并行模式：

- `intent` 工具识别
- `reply` 主回复生成

两者并行启动；如果 `intent` 最终没有命中工具，就直接复用这条提前准备好的 reply，不再重新发一次主回复请求。

如果你不想在拿到最终 ASR 后立刻打断原生小爱，显式关掉即可：

```sh
go run . -abort-after-asr=false
```

如果你想关闭并行模式：

```sh
go run . -parallel-intent-chat=false
```

如果你想控制 `abort` 之后的等待时间：

```sh
go run . -post-abort-delay=0s
go run . -post-abort-delay=500ms
go run . -post-abort-delay=2s
```

当前默认值是 `0s`。

看板默认地址：

```txt
http://127.0.0.1:5173/
```

Go 侧只提供：

- `GET /api/healthz`
- `GET /api/state`

## 当前示例行为

当前工程有两种行为：

1. 正常路径

- 收到 ASR 后，默认先立即调用 `AbortXiaoAI()`
- 然后才调用 `intent` 模型
- 默认同时会并行启动主回复模型
- 不再回退给原生小爱继续回复
- 如果 `intent` 返回 tool call：
  - 再执行本地注册工具对应的闭包
  - 普通工具默认先交给主回复模型整理，再播给音箱
  - 异步工具会先返回短回执，把任务放到后台执行
- 如果 `intent` 没有调用工具：
  - 直接复用并行阶段已经启动的 `reply` 结果
  - 通过 `internal/speaker.StreamPlayer` 边收边切句边播

2. 异步任务与补报

- `complex_task` 会创建后台任务，并把状态写入 `data/tasks.json`
- 后台执行过程中会持续写任务事件
- 任务完成、失败或取消后，会标记 `report_pending=true`
- 用户下一次继续对话时，assistant 会在当前回复后补一句任务新进展
- 也可以通过 React 看板直接查看当前任务和事件流

3. 本地演示命令

- 当识别结果等于 `测试播放文字`
- 先打断原生小爱
- 等待 `post-abort-delay`
- 然后调用 `internal/speaker` 里的 `PlayText()` 播放：
  `你好，很高兴认识你！`

- 当识别结果等于 `测试长段播放文字`
- 先打断原生小爱
- 等待 `post-abort-delay`
- 然后调用 `internal/speaker` 里的 `PlayTextStream()`，按多段 chunk 顺序播放一段长回复

## 本地插件工具

当前内置插件已经拆成独立目录：

- `internal/plugins/weather`
- `internal/plugins/stock`
- `internal/plugins/listtools`
- `internal/plugins/complextask`
- `internal/plugins/querytaskprogress`
- `internal/plugins/canceltask`

`main.go` 只负责装配，真正的工具描述、参数 schema 和命中闭包都放在各自插件包里。

目前默认注册了六个工具：

- `ask_weather`
  - 用于天气类问题
  - 当前会先把城市名解析成 adcode，再调用高德天气接口查询实时天气
- `ask_stock`
  - 用于股票类问题
  - 当前默认返回：`股票不错！`
- `list_tools`
  - 用于回答“你能做什么”“你会什么”“能干啥”
  - 当前会返回所有已注册工具的简短简介拼接结果
- `complex_task`
  - 用于受理复杂、耗时较长的任务
  - 当前会异步创建一个最小演示任务
- `query_task_progress`
  - 用于查询最近异步任务的进度摘要
- `cancel_task`
  - 用于取消最近一个还在执行中的异步任务

这层的设计边界是：

- `intent` 阶段只看到工具描述和参数 schema
- 工具真正怎么执行，由 `internal/plugin.Registry` 保存的闭包决定
- 每个工具都必须提供一个 **10 个字以内** 的 `Summary`
- 工具返回值默认不会直接播给用户，而是会先交给主回复模型整理成自然口语化答案再播报
- 只有插件显式声明 `direct` 输出模式时，才会跳过主回复模型直接播报
- 如果插件返回 `async_accept` 模式，则不会直接产出最终结果，而是先受理任务、回执用户，再由后台任务链路异步回流
- `Handler` 会拿到当前 `CallContext`，里面包含：
  - 当前全局 `Registry`
  - 当前命中的 `Tool`
- 你后面新增工具时，只需要继续注册：
  - 工具名
  - 短简介 `Summary`
  - 描述
  - 输入 schema
  - handler 闭包

不需要去改 `intent` 解析器本身

## 让音箱连过来

先在小爱音箱上把 Rust Client 指向你的电脑：

```sh
mkdir -p /data/open-xiaoai
echo 'ws://你的电脑局域网IP:4399' > /data/open-xiaoai/server.txt
curl -sSfL https://gitee.com/idootop/artifacts/releases/download/open-xiaoai-client/init.sh | sh
```

例如：

```sh
echo 'ws://192.168.31.227:4399' > /data/open-xiaoai/server.txt
```

## 预期输出

当你对音箱说：

```txt
小爱同学
今天天气怎么样
```

如果 `intent` 模型判定应该由外部处理，你会在 Go Server 里看到类似输出：

```txt
2026/04/23 20:00:00 client connected: 192.168.31.100:54321
2026/04/23 20:00:08 xiaoai command: 今天天气怎么样
2026/04/23 20:00:08 intent decision: handle=true abort=true reply=true reason=开放式问答
2026/04/23 20:00:10 reply playback completed
```

如果命中异步任务工具，看板里会看到任务和事件：

```txt
2026/04/24 12:00:00 tool invoke: tool=complex_task arguments={"request":"帮我做一个小游戏网页"}
2026/04/24 12:00:00 async task accepted: id=task_xxx title=帮我做一个小游戏网页
2026/04/24 12:00:05 对了，刚刚有任务有新进展：帮我做一个小游戏网页已经完成了。
```

## 后续扩展

你后面如果要接自己的逻辑，直接从这里往下改就行：

- 改 `internal/assistant` 里的接管策略
- 改 `internal/llm` 里的 prompt 和模型路由
- 在 `internal/plugins` 下继续增加新的本地插件目录
- 继续复用 `AbortXiaoAI()`、`PlayText()`、`StreamPlayer`
- 如果你想直接吃原始麦克风流，再补 `BinaryMessage` 的 `Stream` 解析
