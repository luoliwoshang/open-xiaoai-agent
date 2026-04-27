# Open XiaoAI Agent

`open-xiaoai-agent` 是面向 [`open-xiaoai`](https://github.com/idootop/open-xiaoai) 生态的独立服务端。

它复用设备侧的唤醒、ASR 和播放链路，把对话编排、工具路由、异步任务和看板放到服务端处理。这个仓库现在是独立项目，不再依赖 `open-xiaoai` 的源码目录。

边界上可以简单理解为：

- `open-xiaoai` 负责设备 / client 桥接
- `open-xiaoai-agent` 负责服务端编排

## 当前支持

- 接收 `open-xiaoai` client 通过 WebSocket 转发的最终 ASR 文本
- 使用 `intent` 模型判断普通聊天、工具调用或异步任务
- 使用 `reply` 模型流式生成回复，并通过设备侧 TTS 播放
- 管理轻量异步任务，并在后续对话中补报进度
- 提供独立的 React + Vite dashboard
- 使用本地 JSON 文件保存任务、会话和插件私有状态

当前内置工具：

- `ask_weather`
- `ask_stock`
- `list_tools`
- `complex_task`
- `query_task_progress`
- `cancel_task`
- `continue_task`

其中 `complex_task` / `continue_task` 当前通过 Claude Code CLI 执行；如果要使用这类任务，需要本机可用 `claude` 命令。

## 工作方式

1. 用户唤醒原生小爱并说话。
2. `open-xiaoai` client 把 `SpeechRecognizer.RecognizeResult` 转发到本服务。
3. 服务端执行 `intent` / `reply` / tools / tasks。
4. 回复通过现有 client 播放链路回到设备。

## 依赖

- Go `1.24+`
- Node.js + npm
- 一个可用的 OpenAI 兼容接口
- 高德天气 API Key（仅 `ask_weather` 需要）
- Claude Code CLI（仅 `complex_task` / `continue_task` 需要）

## 快速开始

1. 复制配置文件：

```sh
cp config.example.yaml config.yaml
```

2. 按需修改 `config.yaml`：

```yaml
openai:
  base_url: https://api.openai.com/v1

amap:
  api_key: ""

intent:
  model: qwen-turbo
  base_url: https://dashscope.aliyuncs.com/compatible-mode/v1
  api_key: sk-intent-placeholder

reply:
  model: qwen-turbo
  base_url: https://dashscope.aliyuncs.com/compatible-mode/v1
  api_key: sk-reply-placeholder
```

`SOUL.md` 会在启动时一并读取，用于定义主回复的人设。`config.yaml` 已被忽略，不要提交真实密钥。

3. 安装依赖并启动：

```sh
npm install
npm run dev
```

默认端口：

- WebSocket server: `:4399`
- Dashboard API: `:8090`
- Web dashboard: `http://127.0.0.1:5173`

如果只启动后端：

```sh
go run .
```

如果只启动前端：

```sh
npm run dev:web
```

## 连接设备

在音箱侧把 `open-xiaoai` client 指向这台机器：

```sh
mkdir -p /data/open-xiaoai
echo 'ws://你的电脑局域网IP:4399' > /data/open-xiaoai/server.txt
curl -sSfL https://gitee.com/idootop/artifacts/releases/download/open-xiaoai-client/init.sh | sh
```

例如：

```sh
echo 'ws://192.168.31.227:4399' > /data/open-xiaoai/server.txt
```

## 常用命令

启动前后端：

```sh
npm run dev
```

只启动 Go：

```sh
npm run dev:go
```

只启动前端：

```sh
npm run dev:web
```

构建前端：

```sh
npm run build:web
```

更多后端参数可用 `go run . -h` 查看，常见的有 `-addr`、`-dashboard-addr`、`-abort-after-asr`、`-parallel-intent-chat`。

## Dashboard API

Go 后端只提供 API，前端位于 `web/`。

- `GET /api/healthz`
- `GET /api/state`

## 验证

```sh
GOCACHE=$(pwd)/.gocache go test ./...
npm run build:web
```

## 当前限制

- 还没有独立的 IM Gateway
- 还没有完善的人声打断检测
- 持久化仍然是本地 JSON 文件，不是数据库方案
- 一些更激进的延迟优化还没有迁移到这个独立仓库

## 规划

- 增加独立 IM Gateway，用于接入微信、QQ 等渠道
- 保持 Claude / OpenClaw / 其他执行器可插拔
- 在不改变设备桥接边界的前提下继续增强任务与上下文能力
