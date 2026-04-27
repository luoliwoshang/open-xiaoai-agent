# Open XiaoAI Agent

这是一个面向 [`open-xiaoai`](https://github.com/idootop/open-xiaoai) 生态的独立服务端原型，用来验证“语音入口 + 外部对话编排 + 异步任务执行器”这条链路。当前实现里，[`open-xiaoai`](https://github.com/idootop/open-xiaoai) client 主要负责把设备侧的 ASR 结果和播放能力桥接出来，主对话编排、工具路由、异步任务、任务补报和前端看板都放在这个项目里。

当前原型主要验证了这几件事：

- `intent` 模型：非流式，优先识别是否命中本地工具或任务接续
- `reply` 模型：流式，边收增量边按句切段，再调用音箱本地 TTS 播放
- `tasks`：本地 JSON 任务表，承接最小异步任务能力、任务查询、任务取消和任务接续
- `dashboard api`：Go 只提供 `/api/*` 路由
- `web`：React/Vite 前端看板，单独启动
- `Agent executor`：当前已接入 Claude Code CLI 作为异步任务执行器之一

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

当前边界也很明确：

- 当前主要复用了小爱的 ASR 和 TTS，没有做完整的人声打断识别
- 一些响应速度相关的优化还没有迁到这个独立 demo
- 当前持久化还是本地 JSON 文件，目的是先把原型链路跑通
- 后续会补独立 `IM Gateway`，把微信 / QQ 等渠道接进来，但不会把这些渠道能力耦合到 OpenClaw 或某个具体执行器里

另外，当前 server 会把**同一音箱连接内首轮对话开始后的 5 分钟**视为一个会话窗口。

- 会话窗口内，后续 `intent` 和 `reply` 请求都会自动带上之前的用户/助手上下文
- 超过 5 分钟后，会自动开启一个新的会话，不再携带旧上下文

## 依赖信息

当前项目是一个 Go 后端加 React 看板的单仓应用，主要依赖如下：

- Go
  - `go 1.24.0`
  - 主要依赖：
    - `github.com/gorilla/websocket v1.5.3`
    - `gopkg.in/yaml.v3 v3.0.1`
- Node.js / npm
  - 用于启动前端看板和根目录并发脚本
- React
  - `react ^19.1.0`
  - `react-dom ^19.1.0`
- Vite
  - `vite ^6.3.5`
- TypeScript
  - `typescript ^5.8.3`
- 其他前端开发依赖
  - `@vitejs/plugin-react ^4.4.1`
  - `concurrently ^9.2.1`

补一句边界：

- 当前前端是 `React + Vite`，不是 `Vue`
- 当前后端是单二进制 Go 服务，没有额外数据库和消息队列依赖
- 当前持久化仍然是本地 JSON 文件，不依赖 SQLite、Redis 或外部任务系统

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

## 后续规划

当前版本主要验证的是：

- `open-xiaoai` 作为设备接入层和语音入口
- 外部对话 Server 承接 ASR、工具路由、异步任务和 TTS 编排
- OpenClaw / Claude Code 这类 Agent 作为可插拔执行器

后续规划里，会补一层独立的 `IM Gateway`，用于把微信、QQ 等 IM 渠道接进来。这层的职责会明确收在“渠道接入与消息投递”，而不是塞进 OpenClaw：

- `IM Gateway`
  - 负责微信 / QQ / 其他 IM 渠道接入
  - 负责账号绑定、渠道标识、消息收发和回调适配
  - 负责把 IM 消息转换成统一的会话输入，再交给当前 Agent Server
- `Agent Server`
  - 继续负责主对话编排、工具路由、异步任务管理、任务补报和上下文
  - 不直接耦合某个具体 IM 平台 SDK
- `OpenClaw`
  - 只作为异步任务执行器之一
  - 负责干活，不负责渠道触达

这层边界确定以后，小爱、微信、QQ 这些入口都只是同一个 Agent Server 的不同渠道，OpenClaw 只是执行器，不再承担任何 IM 侧耦合职责。

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
