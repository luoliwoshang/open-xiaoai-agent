# AGENTS.md

This file captures the practical development context for `open-xiaoai-agent`, so future work in another workspace does not need to rediscover the current project state.

## Project Identity

- Repository: `https://github.com/luoliwoshang/open-xiaoai-agent`
- Upstream ecosystem reference: [`open-xiaoai`](https://github.com/idootop/open-xiaoai)
- This project is the standalone server side for the `open-xiaoai` device/client flow.
- It is not meant to depend on the `open-xiaoai` repo source tree anymore.
- Current Go module path:
  - `github.com/luoliwoshang/open-xiaoai-agent`

## What This Project Does

This is a standalone Agent Server prototype that sits behind an `open-xiaoai` client.

Current responsibilities:

- receive final ASR text from the XiaoAI Rust client over WebSocket
- optionally abort the original XiaoAI flow after ASR
- run `intent` routing against tools / async tasks / task continuation
- run `reply` generation for normal chat and tool-result summarization
- drive local TTS playback on the device through the existing client protocol
- maintain lightweight async tasks
- expose a React dashboard over API + web frontend

Current non-goals / known missing pieces:

- no real IM gateway yet
- no proper voice interruption detection
- some latency optimizations were intentionally not carried over from earlier experiments
- persistence is JSON-file based, not SQLite / Redis / MQ

## Tech Stack

- Go `1.24.0`
- React `19.x`
- Vite `6.x`
- TypeScript `5.8.x`
- Node.js / npm for the web dashboard and concurrent dev runner

Go dependencies currently declared in `go.mod`:

- `github.com/gorilla/websocket v1.5.3`
- `gopkg.in/yaml.v3 v3.0.1`

Frontend/runtime tooling from `package.json`:

- root workspace name: `open-xiaoai-agent`
- uses `concurrently` to run Go and web together
- frontend workspace lives in `web/`

Important clarification:

- the frontend is `React + Vite`
- it is not `Vue`

## Repository Layout

Only the high-signal parts are listed here.

- `main.go`
  - application entrypoint
  - wiring for config, tasks, plugins, Claude runner, dashboard, assistant
- `internal/assistant`
  - main orchestration flow
  - conversation history
  - speculative reply handling
  - pending task notice delivery
- `internal/llm`
  - OpenAI-compatible client
  - intent recognizer
  - reply generator
- `internal/plugin`
  - tool/async-task registry
- `internal/plugins`
  - actual builtin tools
  - each plugin lives in its own subdirectory
- `internal/tasks`
  - JSON-backed task store and manager
- `internal/server`
  - WebSocket server / session / RPC protocol
- `internal/speaker`
  - device playback wrappers
  - single text playback + streamed chunk playback
- `internal/dashboard`
  - API side for dashboard state
- `web/`
  - React dashboard

## Runtime Commands

Run both backend and frontend:

```sh
npm install
npm run dev
```

Root scripts:

- `npm run dev`
- `npm run dev:go`
- `npm run dev:web`
- `npm run build:web`
- `npm run preview:web`

Run only backend directly:

```sh
go run .
```

Common backend flags from `main.go`:

- `-addr`
  - default `:4399`
- `-dashboard-addr`
  - default `:8090`
- `-tasks-file`
  - default `data/tasks.json`
- `-conversations-file`
  - default `data/conversations.json`
- `-claude-state-file`
  - default `data/plugins/claude_code.json`
- `-claude-cwd`
  - working directory for Claude CLI tasks
- `-debug`
- `-abort-after-asr`
  - default `true`
- `-post-abort-delay`
  - default `0`
- `-parallel-intent-chat`
  - default `true`

## Configuration Files

- `SOUL.md`
  - persona/system flavor for the main reply model
- `config.example.yaml`
  - committed example config
- `config.yaml`
  - local real config
  - ignored by git because it may contain secrets

Config domains currently used:

- `openai.base_url`
- `intent.model / base_url / api_key`
- `reply.model / base_url / api_key`
- `amap.api_key`

Important:

- `config.yaml` is intentionally ignored
- do not commit real API keys

## Persistent State

This project currently uses JSON file persistence.

Files:

- `data/tasks.json`
  - async task records + task events
- `data/conversations.json`
  - 5-minute conversation history windows
- `data/plugins/claude_code.json`
  - Claude plugin private state

These are intentionally ignored in `.gitignore`.

### Meaning of Each Store

`tasks.json`:

- generic task table
- generic task events
- does not store plugin-specific execution internals such as Claude session ids

`conversations.json`:

- persistent conversation windows used by `intent` and `reply`
- conversation windows are still logically 5-minute windows

`data/plugins/claude_code.json`:

- plugin-private storage
- maps the project task id to Claude-specific state such as:
  - Claude `session_id`
  - prompt
  - last summary
  - last assistant text
  - result

This separation is intentional:

- main task storage remains generic
- plugin-specific continuation state stays inside the plugin

## Conversation Model

Conversation history is persisted and reused.

Current rule:

- a conversation window lasts 5 minutes from the start of the window
- it is not a sliding “last activity + 5m” window

What gets written:

- user input
- assistant replies
- async task notices after they are actually spoken

What this means:

- future `intent` calls see recent history
- future `reply` calls see recent history
- pending task notices become part of history after playback succeeds

## Device / XiaoAI Flow

Current assumed flow:

1. user wakes original XiaoAI
2. original XiaoAI does ASR
3. Rust client forwards `SpeechRecognizer.RecognizeResult`
4. backend optionally aborts original XiaoAI
5. backend handles routing + reply/tool/task logic
6. TTS is played back through client shell/RPC mechanisms

The project still fundamentally reuses:

- XiaoAI wakeup + ASR path
- XiaoAI/open-xiaoai client playback path

## Intent / Reply Behavior

Intent stage:

- non-streaming
- tool-aware
- can see recent conversation history
- can see recent completed tasks for `continue_task` style routing

Reply stage:

- streaming
- used for normal chat
- used for summarizing ordinary tool outputs
- used for task pending notices

Speculative behavior:

- `intent` and `reply` can run in parallel
- if no tool is selected, the speculative reply is reused

## Builtin Plugins

Plugins are registered through `internal/plugins/register.go`.

Current builtin tools include:

- `ask_weather`
- `ask_stock`
- `list_tools`
- `complex_task`
- `query_task_progress`
- `cancel_task`
- `continue_task`

Each plugin should stay inside its own subdirectory. This was an explicit design decision.

### Tool Output Modes

The plugin system supports different output modes. The most important practical distinction:

- direct output
- run through reply model
- async task acceptance

Default expectation for ordinary tools:

- tool result is usually fed back through the reply model rather than spoken raw

## Async Task Model

The async task system is deliberately small.

Current task states:

- `accepted`
- `running`
- `completed`
- `failed`
- `canceled`

Task records contain generic fields such as:

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
- timestamps

Important behavioral decisions:

- async task completion does not proactively start a new reply
- instead, it marks `report_pending=true`
- the pending task notice is merged after a later normal reply path
- the pending notice is generated by the reply model from structured task data
- the system should not mechanically read task titles like “xxx 已经完成了”

### Query Task Progress

This area was iterated multiple times.

Current expectation:

- task progress should include:
  - title
  - real task state
  - current summary
- not just summary alone

This was changed because summaries could say something “sounds done” before the actual task state transitioned to completed.

## Claude Code Async Executor

`complex_task` currently uses Claude Code CLI as one async executor.

Typical command shape:

```sh
claude --dangerously-skip-permissions --print --output-format stream-json --verbose "<task>"
```

Continuation shape:

```sh
claude --dangerously-skip-permissions --resume "<session_id>" --print --output-format stream-json --verbose "<follow-up request>"
```

### Claude Runner Behavior

Implemented behavior:

- parses stream-json line by line
- captures `system/init` for `session_id`
- captures assistant text
- captures final result
- stores Claude-private state in `data/plugins/claude_code.json`

Prompt shaping rules currently enforced for new Claude tasks:

- prefix with “执行以下任务：...”
- ask for short progress updates
- avoid weird symbols / markdown noise in progress
- final summary should still be concise and TTS-friendly

### Resume / Continue Task

Task continuation is intentionally plugin-owned, not global.

Meaning:

- the main task table stores `plugin` and generic task info
- when continuing a task:
  - routing finds a completed task
  - the plugin is identified from the task record
  - the plugin looks up its own private state
  - the plugin retrieves its own Claude `session_id`
  - the plugin resumes work

This is important:

- `session_id` is not stored in the main task table
- plugin-private execution state should not be shared across plugins

Current continuation behavior:

- continuing a task creates a new task row
- `parent_task_id` links it back to the original task
- it does not overwrite the old task

## Weather Integration

Weather is backed by Gaode / AMap.

Important details:

- the public tool input remains a human city name
- internally the weather plugin resolves the city to an `adcode`
- the generated city/adcode mapping was derived from the provided AMap Excel file
- the generated mapping lives in:
  - `internal/plugins/weather/adcodes_gen.go`

## Dashboard / Frontend

Go provides the dashboard API only.

Important routes:

- `GET /api/healthz`
- `GET /api/state`

The frontend is separate and should stay that way.

UI decisions that were explicitly requested:

- conversation history should not be visually mixed with task event flow
- task event flow belongs to a selected task
- conversation history is shown separately
- dashboard should feel intentional, not generic admin boilerplate

## Testing

Recommended Go validation:

```sh
GOCACHE=$(pwd)/.gocache go test ./...
```

Frontend validation:

```sh
npm run build:web
```

Notes:

- `.gocache/` is intentionally ignored
- some earlier environments had issues with local `httptest` binding, but the current repo was validated after migration

## Migration History

This project was migrated out of:

- `open-xiaoai/examples/go-instruction-server`

It is now a standalone repository and should be developed there, not in the old example directory.

## Current README Direction

README is meant for public consumption.

Keep it focused on:

- what the project is
- how to run it
- what it depends on
- what it currently supports
- future high-level planning

Avoid dumping deep internal directory explanations into README.

## Planned Direction

The major planned architectural direction already discussed:

- introduce an independent `IM Gateway`
- support channels such as WeChat / QQ
- keep IM integration outside OpenClaw
- treat OpenClaw / Claude / future executors as pluggable workers, not channel adapters

Desired boundary:

- `open-xiaoai` = device/client bridge
- this repo = server-side orchestration
- IM gateway = channel bridge
- OpenClaw / Claude = async executors

## Practical Notes For Future Agents

- Do not re-couple this repo back to the `open-xiaoai` source tree.
- Do not store executor-private state in the generic task table.
- Do not commit `config.yaml` or runtime data JSON files.
- Prefer updating `AGENTS.md` when a meaningful architectural decision changes.
- Keep plugin code isolated by plugin directory.
- Keep README user-facing; put heavy dev context here instead.
