# 运行时配置

这份文档只记录当前几个容易混淆、而且会影响本地运行行为的路径配置。

## 1. `soul_path`

- 用途：指定主回复模型的人设文件
- 必填
- 支持相对路径
- 支持完整绝对路径

解析规则：

- 如果没有配置 `soul_path`，启动时会直接报错退出
- 如果配置的是相对路径，就按仓库根目录解析
- 如果配置的是绝对路径，就直接使用那个文件
- 如果路径不存在，启动时也会直接报错退出
- 仓库默认不再提交 `SOUL.md`；如果你习惯把人设文件放在项目根目录，可以把它保留为本地私有文件，并让 `soul_path` 指向它

示例：

```yaml
soul_path: ./SOUL.md
```

```yaml
soul_path: /Users/you/prompts/xiaoai-soul.md
```

## 2. `task.artifact_cache_dir`

- 用途：异步任务产物缓存目录
- 默认值：`<repo>/.cache/task-artifacts`
- 支持相对路径和绝对路径

## 3. `im.media_cache_dir`

- 用途：IM 调试发送图片 / 文件时的媒体缓存目录
- 默认值：`<repo>/.cache/im-media`
- 支持相对路径和绝对路径
