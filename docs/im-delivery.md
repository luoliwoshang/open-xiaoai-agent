# 微信适配层发送说明

这份文档只讲一件事：当前微信适配层本身是怎么发文字、怎么发图片、怎么发文件的。

它不解释 IM 网关、不解释 Dashboard，也不解释主流程什么时候触发发送。这里只看微信协议适配这一层到底做了什么。

## 适配层职责

微信适配层的职责很单纯：

1. 接收一份已经确定好的发送请求。
2. 把这份请求转换成微信 `ilink` HTTP 接口能接受的格式。
3. 发起真正的 HTTP 请求。
4. 把微信返回结果转换成统一的发送结果。

也就是说，这一层不做：

- 默认账号选择
- 默认目标选择
- 配置判断
- 上层业务编排

它只做“把消息按微信协议发出去”。

## 1. 微信适配层的公共请求基础

不管是发文字还是发图片，微信适配层都会复用同一套基础请求能力。

### 1.1 HTTP 客户端

当前适配层内部维护一个 `http.Client`，默认请求超时是 15 秒。

这个超时主要覆盖：

- `sendmessage`
- `getuploadurl`
- 其他微信 bot HTTP 接口

### 1.2 公共请求头

每次请求都会带上微信侧要求的公共头：

- `iLink-App-Id`
- `iLink-App-ClientVersion`
- `X-WECHAT-UIN`

对于 `POST` 请求，还会继续带上：

- `Content-Type: application/json`
- `AuthorizationType: ilink_bot_token`
- `Content-Length`
- `Authorization: Bearer <token>`

其中：

- `X-WECHAT-UIN` 是一个随机 `uint32` 转十进制字符串后再做 base64
- `Authorization` 来自扫码登录拿到的 bot token

### 1.3 公共请求体包装

微信这套接口不是直接收一段裸消息，而是要求统一包在：

- `msg`
- `base_info`

其中 `base_info` 里当前主要带：

- `channel_version`

所以适配层发消息时，最终都会是：

```json
{
  "msg": { ... },
  "base_info": {
    "channel_version": "open-xiaoai-agent"
  }
}
```

## 2. 文字发送怎么做

文字发送是最简单的一条链路。

### 2.1 输入

适配层拿到的输入只有三块：

- 微信账号信息
- 目标用户信息
- 一段文本

文字会先做一次 `trim`，如果内容为空就直接返回错误。

### 2.2 构造消息

适配层会先生成一个随机 `client_id`，然后构造微信消息体：

```json
{
  "msg": {
    "from_user_id": "",
    "to_user_id": "<target user id>",
    "client_id": "<random hex>",
    "message_type": 2,
    "message_state": 2,
    "item_list": [
      {
        "type": 1,
        "text_item": {
          "text": "你好"
        }
      }
    ]
  },
  "base_info": {
    "channel_version": "open-xiaoai-agent"
  }
}
```

这里几个关键字段的含义可以简单理解成：

- `to_user_id`：发给谁
- `client_id`：客户端生成的消息标识
- `message_type = 2`：bot 消息
- `message_state = 2`：完成态消息
- `type = 1`：文本 item

### 2.3 实际发送

消息体组好以后，适配层直接调用：

- `POST /ilink/bot/sendmessage`

如果微信返回 2xx，就认为文本发送成功，并把这次的 `client_id` 作为 `MessageID` 返回给上层。

所以文字发送的本质就是：

1. 组一个只有 `text_item` 的 `item_list`
2. 调 `sendmessage`
3. 返回 `client_id`

## 3. 图片发送怎么做

图片发送比文字复杂很多，因为微信图片不是“直接把二进制塞进 `sendmessage`”。

它实际是两段式：

1. 先把图片上传到微信 CDN
2. 再发一条引用这张图片的消息

### 3.1 输入

图片发送当前拿到的输入是：

- 微信账号信息
- 目标用户信息
- 一张已经存在本地的图片文件
- 可选的 `caption`

如果有 `caption`，适配层会先复用文字发送逻辑，先发一条文本，再继续发图。

这意味着“文字说明 + 图片”在微信侧不是一个复合 item，而是两条独立消息。

### 3.2 读取原图并准备加密参数

适配层会先读取本地图片文件，然后生成：

- `filekey`
- 16 字节随机 AES key

接着计算原图：

- 明文大小 `rawsize`
- 明文 MD5 `rawfilemd5`
- AES-128-ECB + PKCS7 加密后的密文
- 密文大小 `filesize`

这里最关键的是：

- 微信 CDN 上传的是加密后的图片
- 后续消息里引用的不是本地文件，而是 CDN 上那份加密资源

### 3.3 获取上传参数

原图准备好后，适配层会先调用：

- `POST /ilink/bot/getuploadurl`

请求体当前已经明确对齐 `openclaw-weixin` 官方实现，只带原图参数：

```json
{
  "filekey": "<filekey>",
  "media_type": 1,
  "to_user_id": "<target user id>",
  "rawsize": 12345,
  "rawfilemd5": "<md5>",
  "filesize": 12352,
  "no_need_thumb": true,
  "aeskey": "<hex aes key>",
  "base_info": {
    "channel_version": "open-xiaoai-agent"
  }
}
```

这里几个关键点是：

- `media_type = 1` 表示图片
- `no_need_thumb = true`
- 当前不走单独的缩略图上传链路
- `aeskey` 这里传的是十六进制字符串

微信返回后，适配层会拿到：

- `upload_full_url`
  或
- `upload_param`

如果只有 `upload_param`，适配层会自己拼 CDN 上传地址。

### 3.4 上传到微信 CDN

拿到上传参数后，适配层会把前面生成好的密文图片上传到微信 CDN。

上传请求是：

- `POST <cdn upload url>`
- `Content-Type: application/octet-stream`
- 请求体直接是加密后的图片字节

上传成功后，CDN 响应头里会返回：

- `x-encrypted-param`

这个值非常关键，它就是后面图片消息里要引用的：

- `encrypt_query_param`

### 3.5 构造图片消息

图片真正发给微信用户时，不再带图片二进制，而是带一份图片引用描述。

当前已经对齐 `openclaw-weixin` 官方实现，最终 `image_item` 只保留最小字段：

```json
{
  "media": {
    "encrypt_query_param": "<cdn encrypted param>",
    "aes_key": "<encoded aes key>",
    "encrypt_type": 1
  },
  "mid_size": 12352
}
```

再把它包进 `item_list`：

```json
{
  "msg": {
    "from_user_id": "",
    "to_user_id": "<target user id>",
    "client_id": "<random hex>",
    "message_type": 2,
    "message_state": 2,
    "item_list": [
      {
        "type": 2,
        "image_item": {
          "media": {
            "encrypt_query_param": "<cdn encrypted param>",
            "aes_key": "<encoded aes key>",
            "encrypt_type": 1
          },
          "mid_size": 12352
        }
      }
    ]
  },
  "base_info": {
    "channel_version": "open-xiaoai-agent"
  }
}
```

然后再调用：

- `POST /ilink/bot/sendmessage`

如果返回 2xx，就认为图片消息已经发送成功。

## 4. 图片发送为什么最后回到官方实现

这部分是当前微信适配层里最重要的实现经验。

之前为了修图片显示异常，曾经尝试过补：

- 缩略图上传参数
- `thumb_media`
- `thumb_size`
- `thumb_width`
- `thumb_height`
- `hd_size`

但真实线上表现说明这条方向不对：

1. 微信 `getuploadurl` 实际并不会稳定返回 `thumb_upload_param`
2. 强行补一套缩略图字段后，客户端表现反而更差

最后真正稳定的做法，是回到 `Tencent/openclaw-weixin` 的实际实现，一比一对齐：

- `getuploadurl` 只传原图参数
- `no_need_thumb = true`
- 不单独上传缩略图
- `image_item` 只保留官方实际使用的最小字段集合

也就是说，当前图片发送能正常工作，不是因为“字段补得越来越多”，而是因为“停止猜协议，回到官方实现”。

## 4. 文件发送怎么做

文件发送和图片发送非常像，本质上也是两段式：

1. 先把文件上传到微信 CDN
2. 再发一条引用这个文件的消息

### 4.1 输入

文件发送当前拿到的输入是：

- 微信账号信息
- 目标用户信息
- 一个已经存在本地的文件
- 可选的 `caption`

如果有 `caption`，适配层同样会先复用文字发送逻辑，先发一条文本，再继续发文件。

### 4.2 上传阶段

文件上传阶段和图片现在共用同一套适配层公共骨架：

- 读取本地文件
- 生成 `filekey`
- 生成 16 字节随机 AES key
- 计算明文大小、MD5、密文大小
- 调 `POST /ilink/bot/getuploadurl`
- 把 AES-128-ECB + PKCS7 后的密文上传到微信 CDN
- 取回 `x-encrypted-param`

区别主要只有两个：

- `media_type = 3`
- 仍然带 `no_need_thumb = true`

也就是说，文件发送不走缩略图逻辑。

### 4.3 构造文件消息

当前文件消息体同样是按 `openclaw-weixin` 已验证实现来做，核心 `file_item` 是：

```json
{
  "media": {
    "encrypt_query_param": "<cdn encrypted param>",
    "aes_key": "<encoded aes key>",
    "encrypt_type": 1
  },
  "file_name": "story.txt",
  "len": "12345"
}
```

其中：

- `file_name` 是展示给微信用户的文件名
- `len` 是原文件明文字节数，注意这里是字符串

再把它包进 `item_list` 后，仍然调用：

- `POST /ilink/bot/sendmessage`

### 4.4 为什么文件也复用了图片的上传骨架

因为微信侧文件和图片真正共享的是上传协议，而不是消息 item 名称。

它们共同依赖的其实是：

- `getuploadurl`
- AES key 生成
- 原文 MD5
- CDN 密文上传
- `encrypt_query_param`

真正分叉的地方只有：

- `media_type`
- 最终是 `image_item` 还是 `file_item`

所以当前实现里，上传和鉴权复用的是同一套 helper，避免图片和文件各自维护一套几乎一样的协议细节。

## 5. 文字、图片、文件在适配层的根本区别

可以把三者的差别压缩成下面这张表。

| 维度 | 文字发送 | 图片发送 | 文件发送 |
|---|---|---|---|
| 是否直接调用 `sendmessage` | 是 | 否，先上传 CDN 再 `sendmessage` | 否，先上传 CDN 再 `sendmessage` |
| 是否需要读本地文件 | 否 | 是 | 是 |
| 是否需要生成 AES key | 否 | 是 | 是 |
| 是否需要计算 MD5 | 否 | 是 | 是 |
| 是否需要调用 `getuploadurl` | 否 | 是 | 是 |
| 是否需要上传二进制到 CDN | 否 | 是 | 是 |
| `item_list` 类型 | `text_item` | `image_item` | `file_item` |

所以从适配层视角看：

- 文字发送是“直接发消息”
- 图片发送是“先上传资源，再发消息引用”
- 文件发送也是“先上传资源，再发消息引用”

## 6. 当前微信适配层的实现边界

当前这层已经支持：

- 文本发送
- 图片发送
- 文件发送
- 图片附带一条前置 caption 文本
- 文件附带一条前置 caption 文本

但还没有做的是：

- 视频发送
- 图片自动缩略图上传链路
- 统一媒体消息抽象

所以如果后面继续扩展这层，最稳的方向仍然是：

- 先对齐 `openclaw-weixin` 已验证实现
- 再把同样的模式扩到视频、文件等媒体类型
