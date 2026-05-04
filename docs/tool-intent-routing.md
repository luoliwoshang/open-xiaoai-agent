# 工具意图识别机制

这份文档只讲一件事：XiaoAiAgent 在意图识别阶段，怎么判断当前一句话应该走普通聊天、取数工具、本机任务，还是命中 `continue_task`。

它不是完整的任务系统设计文档，只解释“给 intent 模型看什么上下文，为什么这么拼”。

## 1. 总体分流

当前意图识别阶段的核心分流是：

- 普通聊天、解释、建议、延伸问答：`continue_chat`
- 明确取数：例如 `ask_weather`、`ask_stock`
- 明确要在本机落地产出物或执行多步骤任务：`complex_task`
- 明确是在补充、修改、继续之前已经完成的任务：`continue_task`
- 明确是在追当前还没做完的任务状态：`query_task_progress`

也就是说，意图识别不是只看“像不像工具调用”，而是要先区分：

- 这是新的任务
- 这是普通对话
- 这是在继续以前做过的那件事
- 这是在追当前任务进度

## 2. 为什么 `continue_task` 不能只看单个 task

`continue_task` 每续做一次，系统都会新建一条新的 task 记录，并通过 `parent_task_id` 串起来。

如果直接把最近完成的 task 行平铺给模型，会有两个问题：

1. 同一条任务链可能出现多次  
   模型容易选到已经过期的旧 `task_id`

2. 只有最新摘要，没有原始输入  
   用户如果按“最开始那件事”来提起任务，模型缺少原始语义锚点

所以 `continue_task` 的候选上下文，不应该按“单条 task”拼，而应该按“任务链快照”拼。

## 3. continue_task 的任务链快照

当前给 intent 模型的 `continue_task` 候选，每条只代表一条任务链当前最新的已完成节点。

每条快照会带这些字段：

- `latest_task_id`
  这条链当前最新的已完成任务 ID
- `plugin`
  这条链应该命中的执行插件
- `root_title`
  根任务标题
- `root_input`
  根任务最开始的用户输入
- `recent_followups`
  最近几次续做任务的输入摘要
- `latest_summary`
  当前最新已完成节点的摘要

这样一条快照同时表达了两层信息：

- 这条链最开始到底是干什么的
- 这条链最近已经改到哪一步

## 4. 为什么这样拼

因为用户继续任务时，提法通常有两种：

### 情况 A：按根任务语义提

例如：

- “那个天气小游戏再加个音效”

这里模型更需要依赖：

- `root_title`
- `root_input`

### 情况 B：按最近续做语义提

例如：

- “刚刚那个再炫酷一点”
- “在上次那个基础上继续改”

这里模型更需要依赖：

- `recent_followups`
- `latest_summary`

所以 `continue_task` 的候选上下文必须同时保留：

- 根任务锚点
- 最近续做轨迹

## 5. 两个具体 case

### Case 1：只有根任务，没有后续续做

用户最开始说：

- “帮我做一个关于天气的小游戏”

任务完成后，候选快照可能是：

```text
- latest_task_id=task_100
  plugin=complex_task
  root_title=天气小游戏
  root_input=帮我做一个关于天气的小游戏
  recent_followups=无
  latest_summary=已经做出一个可玩的天气小游戏网页
```

如果用户后面说：

- “那个天气小游戏再加个音效”

模型就应该命中：

- `continue_task`
- `plugin_name=complex_task`
- `task_id=task_100`
- `request=再加个音效`

### Case 2：已经续做过多次

根任务：

- “帮我做一个关于天气的小游戏”

后续两次续做：

- “加一点动画”
- “再炫酷一点”

这时候选快照应该折叠成一条：

```text
- latest_task_id=task_130
  plugin=complex_task
  root_title=天气小游戏
  root_input=帮我做一个关于天气的小游戏
  recent_followups=加一点动画；再炫酷一点
  latest_summary=当前版本已经加入更强的动画效果和视觉强化
```

如果用户这次说：

- “刚刚那个天气小游戏再加个音效”

模型仍然应该命中：

- `continue_task`
- `plugin_name=complex_task`
- `task_id=task_130`

注意，这里命中的不是根任务 `task_100`，而是这条链当前最新的已完成节点 `task_130`。

## 6. 对 prompt 的影响

优化后，意图识别 prompt 里会明确告诉模型：

- 下面给出的不是普通 task 行，而是“任务链快照”
- 每条快照只代表一条链当前最新的已完成节点
- 判断 `continue_task` 时，要结合：
  - `root_title`
  - `root_input`
  - `recent_followups`
  - `latest_summary`
- 真正调用 `continue_task` 时：
  - `task_id` 必须填写 `latest_task_id`
  - `plugin_name` 必须填写 `plugin`

## 7. 当前刻意不做的事

当前这套机制先不处理：

- 最新节点还在 `running` 时对任务追加新指令
- 一条链同时展示多个历史节点给模型
- 用完整 `result` 全量拼进 prompt

原因很直接：

- `running` 更像进度查询或未来的“运行中追加指令”能力
- 候选里重复展示旧节点会让模型更容易续错
- 全量 `result` 太长，会明显挤占 prompt 预算

所以当前设计收敛为：

- 每条任务链只展示一个快照
- 只命中最新的已完成节点
- 用根输入 + 最近续做 + 最新摘要来平衡语义完整度和 prompt 大小
