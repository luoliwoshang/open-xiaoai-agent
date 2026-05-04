# Claude Code 任务产物交付说明

这份文档只讲一件事：Claude Code 作为异步任务执行器时，怎么把本次任务产物交付给任务系统。

它不重复解释整个异步任务总线，也不展开 IM 渠道投递，只关注 Claude adapter 和任务产物接口之间的这条边界。

## 设计目标

这条链路当前要解决的是：

1. Claude 可以在工作目录里自由生成网页、文档、图片等本地产物。
2. 任务系统不应该和 Claude 的本地文件路径直接耦合。
3. 哪些文件属于“这次任务要交付的产物”，需要有稳定、机器可读的声明方式。
4. 最终进入任务系统的仍然应该是产物内容流和元数据，而不是路径协议。
5. 一旦某个产物被导入任务系统，就由任务系统自动为它创建后续交付记录。

## 总体结构

当前采用的是 manifest 索引方案。

整体分成三层：

1. Claude 在工作目录里生成真实文件。
2. Claude 把最终要交付的文件统一放进任务专属产物目录，再写一个 manifest，声明这些文件需要交付。
3. Claude adapter 读取 manifest 和真实文件，再调用任务系统的 `PutArtifact`。

也就是说：

- manifest 只负责“声明”
- adapter 负责“读取”
- 任务系统负责“存储”和“创建交付记录”

## 任务专属产物目录

当前约定的可交付产物目录是：

```text
.open-xiaoai-agent/deliverables/<task_id>/
```

只要某个文件要通过任务系统作为产物交付，它就必须放在这个任务专属目录下。

例如任务 ID 是 `task_123`，则可交付文件应该放在：

```text
.open-xiaoai-agent/deliverables/task_123/
```

这个目录下面可以再按需要分子目录，例如：

```text
.open-xiaoai-agent/deliverables/task_123/rabbit-game/index.html
```

但不能把最终交付文件随手放在工作目录根目录、`outputs/` 的任意旧路径，或者其他不受控位置。

## manifest 位置

当前约定的 manifest 路径是：

```text
.open-xiaoai-agent/artifacts/<task_id>.json
```

这个路径相对于 Claude 的工作目录。

例如任务 ID 是 `task_123`，则 Claude 需要写：

```text
.open-xiaoai-agent/artifacts/task_123.json
```

## manifest 结构

当前 manifest 只负责索引交付产物，不承载文件内容。

结构如下：

```json
{
  "deliver": [
    {
      "path": ".open-xiaoai-agent/deliverables/task_123/rabbit-game/index.html",
      "name": "rabbit-game.html",
      "kind": "file",
      "mime_type": "text/html"
    },
    {
      "path": ".open-xiaoai-agent/deliverables/task_123/rabbit-game/README.txt",
      "name": "README.txt",
      "kind": "file",
      "mime_type": "text/plain"
    }
  ]
}
```

字段语义：

- `path`
  指向真实文件，路径相对 Claude 工作目录
- `name`
  进入任务系统后展示给用户的文件名
- `kind`
  当前通常是 `file`
- `mime_type`
  可选 MIME 类型

关于 `name`，当前有一个很重要的约束：

- 如果填写 `name`，应优先带上和真实文件一致的后缀，比如 `.html`、`.png`、`.txt`
- 如果 Claude 不确定最终展示文件名，就不要硬写一个无后缀名字，直接省略 `name`
- 当 `name` 省略时，adapter 会回退到 `path` 对应的真实文件名
- 如果 `name` 被填写了但漏掉后缀，adapter 会优先用真实文件路径上的后缀做一次补齐

这样做是为了避免：

- Dashboard 里看到的文件名没有后缀
- 移动端下载后无法直接按正确类型打开

## 为什么 manifest 只做索引

manifest 现在故意不承载文件内容，原因很直接：

- JSON 不适合传大文件
- 把内容塞进 manifest 会让 Claude 产物协议变重
- 真正的二进制或文本内容，仍然应该从真实文件读取

所以当前边界是：

- Claude 自己生成真实文件
- Claude 只在 manifest 里声明文件位置
- adapter 再去打开这些文件并导入任务系统

## Claude adapter 的导入流程

在 Claude CLI 成功返回最终结果后，runner 会按下面顺序继续处理：

1. 查找当前任务的 manifest。
2. 如果 manifest 不存在，视为本次没有交付产物。
3. 如果 manifest 存在，则解析 `deliver` 列表。
4. 对每个条目：
   - 校验路径非空
   - 把相对路径解析到 Claude 工作目录下
   - 校验最终路径不能逃出工作目录
   - 校验最终路径必须落在任务专属产物目录下
   - 规范化展示文件名，优先保留或补齐真实文件后缀
   - 打开真实文件并读取元数据
   - 调用 `PutArtifact`
5. 全部导入成功后，追加一条“Claude 已登记 N 个交付产物”的任务事件。
6. 最后再把任务标记为完成。

这里有两个重要约束：

- 产物导入必须发生在任务完成之前
- 任务系统会在 `PutArtifact` 时自动创建对应的交付记录

因为当前任务系统已经规定：

- `completed` 状态后不再允许新增产物

## 为什么路径只在 adapter 内部消费

这里的关键边界是：

- Claude adapter 内部可以知道本地路径
- 但任务系统接口不接受路径

Claude adapter 调用任务系统时，传的是：

- 文件名
- 类型
- MIME 类型
- 文件大小
- 文件内容流

这样做的好处是：

- 任务系统不依赖 Claude 的工作目录结构
- 后续换执行器时，不需要把 Claude 的路径协议带到全局层
- 产物缓存最终落在哪里，仍然由任务系统决定

## 路径校验规则

当前 manifest 里的 `path` 必须满足两个条件：

1. 能解析到一个真实存在的文件
2. 最终路径必须仍然落在 Claude 工作目录内
3. 最终路径必须落在 `.open-xiaoai-agent/deliverables/<task_id>/` 下面

这意味着：

- `.open-xiaoai-agent/deliverables/task_123/index.html` 这种路径是允许的
- `.open-xiaoai-agent/deliverables/task_123/game/index.html` 这种路径也是允许的
- `outputs/index.html` 这种旧式随意路径现在不再允许
- `../secret.txt` 这种越界路径会被拒绝

这样做是为了防止 manifest 把任务系统带去读取工作目录外的任意文件，也避免“最终可交付产物落在任意位置”。

## 失败策略

当前失败策略是：

- manifest 缺失：不报错，视为没有交付产物
- manifest 存在但格式错误：任务失败
- manifest 条目越界或文件不存在：任务失败
- `PutArtifact` 失败：任务失败

原因是：

- “有没有交付文件”是可选的
- 但一旦 manifest 已经声明了交付意图，就必须保证这条导入链路是可靠的

## Prompt 约束

为了让 Claude 遵守这套协议，runner 在 prompt 里会额外要求：

1. 如果本次任务有需要交付给系统的文件，必须写 manifest。
2. manifest 只能声明元数据和相对路径。
3. `path` 必须指向这次任务真实生成的文件。
4. 所有要交付的文件都必须放在 `.open-xiaoai-agent/deliverables/<task_id>/` 下。
5. 如果没有交付文件，就不要创建 manifest。
6. 如果填写 `name`，应优先带上和真实文件一致的后缀；如果不确定展示文件名，就直接省略 `name`，让系统回退到 `path` 对应的文件名。
7. 这些路径和 manifest 规则只属于执行器内部协议，不应该出现在面向用户的进度或最终总结里。
8. 面向用户的表达应尽量口语化、易懂，避免过于专业、冗余或工程化。
9. 即使已经产出了文件，最终总结里也不要直接说“保存为 xxx.png / xxx.html / xxx.txt”。

这意味着 Claude 的最终自然语言总结和机器可读的产物声明被拆成了两条：

- 最终总结：给用户看、给 TTS 播报
- manifest：给系统读、给产物导入

## 当前边界

这期实现只解决 Claude 到任务产物系统的接入，不解决更上层的投递问题。

当前不在这份设计里的包括：

- IM 渠道如何把产物发给用户
- 远端 executor 通用协议
- 对象存储
- 多目标投递
- 渠道重新绑定后的自动补发

这一层当前只负责把 Claude 产物稳定地接到任务系统里。
