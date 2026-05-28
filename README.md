# Monica Proxy

这是一个代理服务，用于将 Monica 的服务转换为 ChatGPT 兼容的 API 格式。通过提供 Monica Cookie，您可以使用与 ChatGPT API
相同的接口格式来访问 Monica 的服务。

## 功能特点

- 兼容 ChatGPT API 格式
- 使用 Docker 进行简单部署
- Bearer Token 认证保护
- 支持token计数
- 支持流式和非流式响应

## 快速开始

### 部署方式一：使用 Docker 运行

```bash
docker run --pull=always -d \
  --name monica-proxy \
  -p 8080:8080 \
  -e MONICA_COOKIE="你的Monica_Cookie" \
  -e BEARER_TOKEN="你想设置的验证令牌" \
  neccen/monica-proxy:latest
```

### 部署方式二：使用 Docker Compose

1. 创建 `docker-compose.yml` 文件：
   ```yaml
   services:
     monica-proxy:
       image: neccen/monica-proxy:latest
       container_name: monica-proxy
       restart: unless-stopped
       ports:
         - "8080:8080"
       environment:
         - MONICA_COOKIE="MONICA_COOKIE"
         - BEARER_TOKEN="YOUR_BEARER_TOKEN"
   ```

### 命令行参数
| 参数 | 简写 | 默认值 | 说明 |
|------|------|--------|------|
| `--p` | `-p` | `8080` | 服务器监听端口 |
| `--h` | `-h` | `0.0.0.0` | 服务器监听地址 |
| `--c` | `-c` | `""` | Monica Cookie值 (MONICA_COOKIE) |
| `--k` | `-k` | `""` | Bearer Token值 (BEARER_TOKEN) |
| `--i` | `-i` | `true` | 是否启用隐身模式 (IS_INCOGNITO) |

### 启动示例

```bash
# 使用默认配置启动服务
./monica-proxy

# 指定端口和地址
./monica-proxy -p 9000 -h 127.0.0.1

# 设置认证信息并关闭隐身模式
./monica-proxy -c "your-monica-cookie" -k "your-bearer-token" -i=false
```
或者使用环境变量：
- `MONICA_COOKIE`: Monica 的 Cookie `(格式：session_id=eyJ...)`，用于访问 Monica 的 API
- `BEARER_TOKEN`: API 访问令牌，用于保护 API 接口安全

2. 启动服务
   ```bash
   docker-compose up -d
   ```

服务将在 `http://ip:8080` 上运行。

## API 接口说明

兼容 OpenAI/ChatGPT API 格式，所有请求需在 Header 中携带 Bearer Token：

```http
Authorization: Bearer YOUR_BEARER_TOKEN
Content-Type: application/json
```

### 1. 获取模型列表

**GET** `/v1/models`

返回当前支持的模型列表，响应格式与 OpenAI 一致。

**请求示例：**

```bash
curl -X GET "http://ip:8080/v1/models" \
  -H "Authorization: Bearer YOUR_BEARER_TOKEN"
```

**响应示例：**

```json
{
  "object": "list",
  "data": [
    {
      "id": "gpt-4o",
      "object": "model",
      "owned_by": "monica"
    }
  ]
}
```

### 2. Responses API（推荐，多轮会话）

**POST** `/v1/responses`

与 OpenAI Responses API 兼容。代理在首轮生成 `resp_` 开头的 `id` 并缓存 Monica 的 `conversation_id` 与消息链；后续请求传 `previous_response_id` 即可续聊，**无需在 `messages` 里重复携带完整历史**，更接近 Monica 原生协议。

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `model` | string | 是 | 模型 ID |
| `input` | string 或 message 数组 | 是 | 本轮用户输入 |
| `instructions` | string | 否 | 系统指令（仅首轮生效，会拼到首条用户消息） |
| `previous_response_id` | string | 否 | 上一轮响应的 `id`，用于续聊 |
| `stream` | boolean | 否 | 是否流式返回 |

**首轮示例：**

```bash
curl -X POST "http://ip:8080/v1/responses" \
  -H "Authorization: Bearer YOUR_BEARER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5.5",
    "input": "hi",
    "stream": false
  }'
```

**续轮示例（使用上一轮返回的 `id`）：**

```bash
curl -X POST "http://ip:8080/v1/responses" \
  -H "Authorization: Bearer YOUR_BEARER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5.5",
    "input": "哈哈",
    "previous_response_id": "resp_xxxxxxxx",
    "stream": false
  }'
```

**响应字段（非流式）：** `id`、`output_text`、`output`、`usage` 等，与 OpenAI Responses API 一致。

**流式：** 返回 `text/event-stream`，事件类型含 `response.created`、`response.output_text.delta`、`response.completed`。

会话缓存默认保留 7 天（与 OpenAI `previous_response_id` 有效期类似）。设置 `"store": false` 可不缓存会话。

### 3. 聊天补全（对话）

**POST** `/v1/chat/completions`

> 仍支持，但每次请求会将 `messages` 全量转为 Monica `items`，并新建 `conversation_id`。多轮对话更推荐使用 `/v1/responses`。

**请求体参数（与 OpenAI 兼容）：**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `model` | string | 是 | 模型 ID，见下方「支持的模型」 |
| `messages` | array | 是 | 消息列表，每项含 `role`（user/assistant/system）和 `content` |
| `stream` | boolean | 否 | 是否流式返回，默认 `false` |
| `max_tokens` | number | 否 | 最大生成 token 数 |
| `temperature` | number | 否 | 采样温度 |
| `top_p` | number | 否 | 核采样参数 |

**请求示例：**

```bash
# 流式请求
curl -X POST "http://ip:8080/v1/chat/completions" \
  -H "Authorization: Bearer YOUR_BEARER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [
      {"role": "user", "content": "你好"}
    ],
    "stream": true
  }'

# 非流式请求
curl -X POST "http://ip:8080/v1/chat/completions" \
  -H "Authorization: Bearer YOUR_BEARER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4-6",
    "messages": [
      {"role": "system", "content": "你是一个助手"},
      {"role": "user", "content": "介绍一下自己"}
    ],
    "stream": false
  }'
```

**响应：**

- `stream: false`：返回 JSON 对象，格式同 OpenAI Chat Completions。
- `stream: true`：返回 SSE（Server-Sent Events）流，每行 `data: {...}`，以 `data: [DONE]` 结束。

## 支持的 Monica 模型

请求体中的 `model` 需使用下表中的 **id**。

| 分类 | model id | 说明 |
|------|----------|------|
| **OpenAI** | `gpt-5` | GPT-5 |
| | `gpt-5.1` | GPT-5.1 |
| | `gpt-5.2` | GPT-5.2 |
| | `gpt-5.5` | Monica 主聊天（GPT-5.5） |
| | `gpt-4o` | GPT-4o |
| | `gpt-4.1` | GPT-4.1 |
| | `gpt-4.5-preview` | GPT-4.5 |
| | `gpt-4o-mini` | GPT-4o mini |
| | `dall-e-3` | DALL·E 3（文生图） |
| | `openai-o1` | o1 |
| | `openai-o-3-mini` | o3-mini |
| **Claude** | `claude-3.5-haiku` | Claude 3.5 Haiku |
| | `claude-3.5-sonnet` | Claude 3.5 Sonnet V2 |
| | `claude-3.7-sonnet` | Claude 3.7 Sonnet |
| | `claude-3.7-sonnet-thinking` | Claude 3.7 Sonnet Thinking |
| | `claude-4-sonnet` | Claude 4 Sonnet |
| | `claude-4-opus` | Claude 4 Opus |
| | `claude-sonnet-4-5` | Claude 4.5 Sonnet |
| | `claude-sonnet-4-6` | Claude 4.6 Sonnet |
| | `deepclaude` | DeepClaude |
| **Grok** | `grok-3-beta` | Grok 3 |
| | `grok-4-0709` | Grok 4 |
| **Gemini** | `gemini-2.5-pro` | Gemini 2.5 Pro |
| | `gemini-2.5-flash` | Gemini 2.5 Flash |
| | `gemini-3-pro-preview-thinking` | Gemini 3 Pro（带思考过程） |
| | `gemini-3.5-flash-thinking` | Gemini 3.5 Flash（带思考过程） |
| **DeepSeek** | `deepseek-chat` | DeepSeek V3 |
| | `deepseek-reasoner` | DeepSeek R1 |
| **Llama** | `llama-3.3-70b` | Llama 3.3 70B |
| | `llama-3.1-405b` | Llama 3.1 405B |

## 注意事项

- 请确保 MONICA_COOKIE 和 BEARER_TOKEN 环境变量正确设置
- 确保端口未被占用
- Cookie 和 Bearer Token 应妥善保管，不要泄露给他人
- 所有 API 请求都需要提供有效的 Bearer Token